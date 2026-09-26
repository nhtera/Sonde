// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nhtera/sonde/engine"
)

// WriteJSON appends one Result per result, in order, to the JSON report
// at dir/report.json, saving every call's response body, redacted, under
// dir/store/<id>_response<ext> and referencing it from the corresponding
// Response.Body as "store/<file>" (relative to report.json). ext is
// ".json", ".xml" or ".html" for a response of that content type, and
// omitted otherwise.
//
// Repeated calls with the same dir read the existing report.json (if
// any) and extend its array — the report is cumulative across
// invocations, matching the reference CLI. Existing entries are kept as
// raw JSON and spliced back in unparsed, so a rewrite never loses a big
// integer's precision, an object's member order, or a field this
// package's Result does not know about (fields written by another
// tool, or the reserved "sonde" key) — only the
// entries this call adds are ever actually decoded/re-encoded. The file
// is written atomically (temp file + rename), so a run killed mid-write
// never leaves a truncated report for the next invocation to choke on.
// redact masks secret values everywhere a string (including a response
// body) is written.
func WriteJSON(dir string, results []*engine.UnitResult, redact func(string) string) error {
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o750); err != nil {
		return err
	}

	reportPath := filepath.Join(dir, "report.json")
	existing, err := readJSONReport(reportPath)
	if err != nil {
		return err
	}

	store := func(body []byte, contentType string) (string, error) {
		return saveResponseBody(storeDir, body, contentType, redact)
	}
	for _, res := range results {
		r, err := JSON(res, redact, store)
		if err != nil {
			return err
		}
		line, err := MarshalJSONLine(r)
		if err != nil {
			return err
		}
		existing = append(existing, json.RawMessage(line))
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(existing); err != nil {
		return err
	}
	return atomicWriteFile(reportPath, buf.Bytes(), 0o644) //nolint:gosec // G306: report files are not secret
}

// readJSONReport parses the report.json already at path into its raw
// elements, or returns nil when it does not exist yet. Kept raw (not
// decoded into []Result) so WriteJSON can splice each one back in
// byte-for-byte — see its doc comment for why.
func readJSONReport(path string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is built from a CLI-trusted --report-json flag
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var results []json.RawMessage
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("report: reading JSON report %s: %w", path, err)
	}
	return results, nil
}

// bodyExtension patterns, matching the reference CLI's mimetype module:
// a JSON, XML or HTML response gets that extension so the saved file
// opens sensibly; anything else is saved without one.
var (
	jsonContentType = regexp.MustCompile(`^application/([a-z0-9._-]+[+.])?json`)
	xmlContentType  = regexp.MustCompile(`^(text/xml|application/([a-z0-9._-]+[+.])?xml)`)
)

// bodyExtension returns the file extension (including the leading dot)
// for a response of contentType, or "" when none applies.
func bodyExtension(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	// Strip parameters ("; charset=utf-8"), matching how the reference
	// mimetype matchers only ever look at the type/subtype.
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch {
	case jsonContentType.MatchString(ct):
		return ".json"
	case xmlContentType.MatchString(ct):
		return ".xml"
	case strings.HasPrefix(ct, "text/html"):
		return ".html"
	default:
		return ""
	}
}

// saveResponseBody writes body, redacted, to a new, uniquely named file
// under storeDir and returns the path a JSON report references it by:
// "store/<id>_response<ext>". redact is applied to the raw bytes (through
// a lossless byte<->string round trip, same as internal/redact's own
// RedactBytes) regardless of whether the body is text or binary: it only
// ever replaces exact byte sequences it was told to mask, so it cannot
// corrupt a body it finds nothing to redact in.
func saveResponseBody(storeDir string, body []byte, contentType string, redact func(string) string) (string, error) {
	name := newReportID() + "_response" + bodyExtension(contentType)
	redacted := []byte(redact(string(body)))
	if err := os.WriteFile(filepath.Join(storeDir, name), redacted, 0o600); err != nil {
		return "", err
	}
	return "store/" + name, nil
}

// newReportID returns a random (version 4, RFC 9562) UUID in lowercase
// hyphenated form, used to name files a report saves outside itself
// (response bodies, HTML per-unit pages) so concurrent writers never
// collide.
func newReportID() string {
	var u [16]byte
	_, _ = rand.Read(u[:]) // never fails (crypto/rand panics instead)
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:], u[10:])
	return string(b[:])
}
