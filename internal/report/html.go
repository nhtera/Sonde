// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// htmlBodyMaxProcess bounds the response body a page will even attempt to
// pretty-print, decode-check and redact; a larger one is summarized by
// its size instead, so one huge response cannot blow up report
// generation time or memory.
const htmlBodyMaxProcess = 4 << 20 // 4 MiB

// htmlBodyPreviewCap is the displayed length of a body preview, applied
// after redaction (never before it: truncating first could cut a secret
// in half and leave an unredacted fragment visible up to the cut).
const htmlBodyPreviewCap = 64 << 10 // 64 KiB

//go:embed templates/index.html.tmpl templates/unit.html.tmpl
var htmlTemplateFS embed.FS

var htmlTemplates = template.Must(template.ParseFS(htmlTemplateFS, "templates/*.tmpl"))

// htmlManifestEntry is one run recorded in a report's manifest, the
// bookkeeping file that lets WriteHTML rebuild index.html across
// invocations without re-reading every store/<id>.html.
type htmlManifestEntry struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Success   bool      `json:"success"`
	TimeMS    int64     `json:"time_ms"`
	Timestamp time.Time `json:"timestamp"`
}

// htmlIndexEntry is one row of index.html.
type htmlIndexEntry struct {
	ID       string
	Filename string
	Success  bool
	TimeMS   int64
	RunAt    string
}

// htmlIndexData is index.html's template data.
type htmlIndexData struct {
	Entries   []htmlIndexEntry
	Total     int
	Succeeded int
	Failed    int
}

// htmlCall is one HTTP exchange shown on a unit's page: the shared JSON
// model's Call (request/response/timings — already redacted) plus a
// rendered preview of the response body, which the JSON model does not
// carry (it only ever references a body by path, for --report-json).
type htmlCall struct {
	Request     Request
	Response    Response
	Timings     Timings
	BodyPreview string
}

// htmlEntry is one attempt shown on a unit's page: the shared JSON model's
// Entry (asserts, captures, curl command — already redacted) plus its
// calls with a body preview added, and the attempt's non-assert runtime
// errors, which the JSON schema does not carry.
type htmlEntry struct {
	Index         int
	Line          int
	CurlCmd       string
	Calls         []htmlCall
	Asserts       []Assert
	Captures      []Capture
	RuntimeErrors []string
	Stream        *htmlStream
}

// htmlStream is a stream transcript, cut to its first messages.
type htmlStream struct {
	Stream
	Omitted int // messages not shown
}

// Transcript limits of the HTML report.
const (
	htmlStreamMaxMessages = 100
	htmlStreamMaxData     = 2 << 10
)

// newHTMLStream truncates a stream (already redacted) for display.
func newHTMLStream(s *Stream) *htmlStream {
	hs := &htmlStream{Stream: *s}
	if len(s.Messages) > htmlStreamMaxMessages {
		hs.Messages = s.Messages[:htmlStreamMaxMessages]
		hs.Omitted = len(s.Messages) - htmlStreamMaxMessages
	}
	hs.Messages = append([]StreamMessage(nil), hs.Messages...)
	for i, m := range hs.Messages {
		if len(m.Data) > htmlStreamMaxData {
			hs.Messages[i].Data = truncateUTF8(m.Data, htmlStreamMaxData) + fmt.Sprintf(" ... (%d bytes)", len(m.Data))
		}
	}
	return hs
}

// htmlUnitData is a unit page's (store/<id>.html) template data.
type htmlUnitData struct {
	Filename   string
	Success    bool
	TimeMS     int64
	RunAt      string
	ParseError string
	Source     string
	Entries    []htmlEntry
}

// WriteHTML appends one page per result, in order, to the HTML report at
// dir: a page per file under dir/store/<id>.html — one directory level
// under dir, a flat file rather than a nested index.html, so a shell
// glob like "dir/**/*.html" (which without bash's globstar only ever
// descends one extra level) still finds it (source and calls are
// escaped through html/template, so a hostile header, URL or body value
// can never break out of its context) — and a dir/index.html listing
// every run recorded so far, linking to its page.
//
// Repeated calls with the same dir read the report's manifest and grow
// it — the report is cumulative across invocations, matching the other
// report formats. redact masks secret values everywhere a string is
// written.
func WriteHTML(dir string, results []*engine.UnitResult, redact func(string) string) error {
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o750); err != nil {
		return err
	}

	manifestPath := filepath.Join(dir, ".manifest.json")
	manifest, err := readHTMLManifest(manifestPath)
	if err != nil {
		return err
	}
	// Keyed by the lowercased id: many filesystems sonde runs on (notably
	// APFS and NTFS by default) are case-insensitive, so "A.hurl" and
	// "a.hurl" must not both claim a page differing only in case.
	used := make(map[string]bool, len(manifest))
	for _, m := range manifest {
		used[strings.ToLower(m.ID)] = true
	}

	for _, res := range results {
		rd := forResult(res, redact)
		id := uniqueHTMLID(used, res.Label())
		if err := writeHTMLUnitPage(storeDir, id, res, rd); err != nil {
			return err
		}
		manifest = append(manifest, htmlManifestEntry{
			ID:        id,
			Filename:  rd(res.Label()),
			Success:   res.Success,
			TimeMS:    res.Duration.Milliseconds(),
			Timestamp: res.Timestamp,
		})
	}

	if err := writeHTMLManifest(manifestPath, manifest); err != nil {
		return err
	}
	return writeHTMLIndex(dir, manifest)
}

func writeHTMLUnitPage(storeDir, id string, res *engine.UnitResult, redact func(string) string) error {
	jr, err := JSON(res, redact, nil)
	if err != nil {
		return err
	}
	entries := make([]htmlEntry, len(jr.Entries))
	for i, je := range jr.Entries {
		he := htmlEntry{
			Index: je.Index, Line: je.Line, CurlCmd: je.CurlCmd,
			Asserts: je.Asserts, Captures: je.Captures,
		}
		for j, jc := range je.Calls {
			hc := htmlCall{Request: jc.Request, Response: jc.Response, Timings: jc.Timings}
			if i < len(res.Entries) && j < len(res.Entries[i].Calls) {
				hc.BodyPreview = htmlBodyPreview(res.Entries[i].Calls[j].Response, redact)
			}
			he.Calls = append(he.Calls, hc)
		}
		if i < len(res.Entries) {
			for _, e := range res.Entries[i].Errors {
				if !e.Assert() {
					he.RuntimeErrors = append(he.RuntimeErrors, redact(e.Render()))
				}
			}
		}
		if je.Sonde != nil && je.Sonde.Stream != nil {
			he.Stream = newHTMLStream(je.Sonde.Stream)
		}
		entries[i] = he
	}

	data := htmlUnitData{
		Filename: redact(res.Label()),
		Success:  res.Success,
		TimeMS:   res.Duration.Milliseconds(),
		RunAt:    formatRunAt(res.Timestamp),
		Source:   redact(string(res.Source)),
		Entries:  entries,
	}
	if res.ParseError != nil {
		data.ParseError = redact(res.ParseError.Render())
	}

	var buf bytes.Buffer
	if err := htmlTemplates.ExecuteTemplate(&buf, "unit.html.tmpl", data); err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(storeDir, id+".html"), buf.Bytes(), 0o644) //nolint:gosec // G306: report files are not secret
}

func writeHTMLIndex(dir string, manifest []htmlManifestEntry) error {
	data := htmlIndexData{Total: len(manifest)}
	for _, m := range manifest {
		if m.Success {
			data.Succeeded++
		} else {
			data.Failed++
		}
		data.Entries = append(data.Entries, htmlIndexEntry{
			ID: m.ID, Filename: m.Filename, Success: m.Success,
			TimeMS: m.TimeMS, RunAt: formatRunAt(m.Timestamp),
		})
	}

	var buf bytes.Buffer
	if err := htmlTemplates.ExecuteTemplate(&buf, "index.html.tmpl", data); err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, "index.html"), buf.Bytes(), 0o644) //nolint:gosec // G306: report files are not secret
}

// htmlBodyPreview renders resp's body for a unit page: content-decoded,
// pretty-printed when it is JSON (stdlib encoding/json, no new
// dependency), shown as text when it decodes to valid UTF-8, or
// summarized by size otherwise ("binary, N bytes") — always redacted
// before the display cap is applied, and never fully materialized (or
// redacted) when it is implausibly large.
func htmlBodyPreview(resp *exchange.Response, redact func(string) string) string {
	if resp == nil || len(resp.Body) == 0 {
		return ""
	}
	body, err := resp.DecodedBody()
	if err != nil {
		body = resp.Body // shown as received; still worth a look even undecoded
	}
	if len(body) == 0 {
		return ""
	}
	if len(body) > htmlBodyMaxProcess {
		return fmt.Sprintf("body too large to preview, %d bytes", len(body))
	}

	ct, _ := resp.ContentType()
	if bodyExtension(ct) == ".json" {
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err == nil {
			body = buf.Bytes()
		}
		// Content-Type said JSON but the body is not valid JSON: fall
		// through and show it as text/binary below, exactly as received.
	}
	if !utf8.Valid(body) {
		return fmt.Sprintf("binary, %d bytes", len(body))
	}

	text := redact(string(body))
	if len(text) > htmlBodyPreviewCap {
		total := len(text)
		text = truncateUTF8(text, htmlBodyPreviewCap)
		text += fmt.Sprintf("\n... truncated (showing %d of %d bytes)", len(text), total)
	}
	return text
}

// truncateUTF8 returns the first n bytes of s, pulled back to the start
// of a rune if n would otherwise split one.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func formatRunAt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

// readHTMLManifest returns the manifest already at path, or an empty one
// when it does not exist yet — including when dir already holds an
// index.html but no manifest (its own history, and the manifest, were
// deleted, or a directory built some other way is being reused as a
// report). WriteHTML does not try to recover that history by scraping the
// existing index.html: it just restarts the report from this call's
// results, the same warn-free way it behaves on an entirely empty dir.
func readHTMLManifest(path string) ([]htmlManifestEntry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is built from a CLI-trusted --report-html flag
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var manifest []htmlManifestEntry
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("report: reading HTML report manifest %s: %w", path, err)
	}
	return manifest, nil
}

func writeHTMLManifest(path string, manifest []htmlManifestEntry) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644) //nolint:gosec // G306: report files are not secret
}

// htmlUnsafeID matches everything not safe to use unquoted in a
// filesystem path component.
var htmlUnsafeID = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// sanitizeHTMLID turns name (typically a file path) into a filesystem-safe
// directory name: unsafe characters become "_", and leading dots (which
// would otherwise create a hidden directory, or "." / ".." themselves)
// are stripped.
func sanitizeHTMLID(name string) string {
	id := htmlUnsafeID.ReplaceAllString(name, "_")
	id = strings.TrimLeft(id, "._")
	if id == "" {
		id = "unit"
	}
	const maxLen = 120
	if len(id) > maxLen {
		id = id[:maxLen]
	}
	return id
}

// uniqueHTMLID returns a sanitized id for name that is not already in
// used (compared case-insensitively — see the comment where WriteHTML
// builds used), marking it used. Matches architecture.md's "per-unit
// files uniquely named" rule for repeated runs of the same file.
func uniqueHTMLID(used map[string]bool, name string) string {
	base := sanitizeHTMLID(name)
	id := base
	for n := 2; used[strings.ToLower(id)]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	used[strings.ToLower(id)] = true
	return id
}
