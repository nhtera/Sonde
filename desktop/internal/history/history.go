// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package history keeps finished runs in the app's config folder (0600),
// in the CLI's JSON report shape without bodies, redacted: the run's own
// secrets (declared secrets, redact captures), the values of credential
// headers and of every cookie, and captured tokens found by a heuristic
// (a name like token, secret, session…, or a JWT-shaped value). Other
// captures, ids and totals, stay readable.
package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/credential"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/redact"
	"github.com/nhtera/sonde/internal/report"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Dir is the history folder in the app's config folder.
const Dir = "history"

// TopicChanged is sent after a run is added or the history cleared.
const TopicChanged = "history:changed"

// Policy is the history settings.
type Policy struct {
	Enabled bool
	// Retention is how long runs are kept; 0 keeps them forever.
	Retention time.Duration
}

// Record is a stored run.
type Record struct {
	ID      string          `json:"id"`
	Summary *runsvc.Summary `json:"summary"`
	Results []report.Result `json:"results"`
}

// Item is a run in the list.
type Item struct {
	ID        string    `json:"id"`
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Env       string    `json:"env"`
	Files     []string  `json:"files"`
	Outcome   string    `json:"outcome"`
	Requests  int       `json:"requests"`
	Succeeded int       `json:"succeeded"`
	Duration  int64     `json:"durationMs"`
	// Calls are the run's requests, last first (the last maxCalls), as
	// the record keeps them (redacted).
	Calls []Call `json:"calls"`
}

// Call is a request of a run in the history: what was sent, what came
// back, from which file and entry.
type Call struct {
	// Result is the call's unit in the record (a test run's file, a data
	// run's row), to open the right one.
	Result   int    `json:"result"`
	File     string `json:"file"`
	Entry    int    `json:"entry"`
	Method   string `json:"method"`
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Duration int64  `json:"durationMs"`
	// Failed is set when an assert of the entry failed (its status may
	// still be a 2xx).
	Failed bool `json:"failed,omitempty"`
}

// History is the run history of the open project.
type History struct {
	config  *sandbox.Root
	project func() *sandbox.Root
	policy  func() Policy
	emit    func(topic string, data any)
	now     func() time.Time

	mu sync.Mutex
}

// New returns the history kept in config for the project project returns.
func New(config *sandbox.Root, project func() *sandbox.Root, policy func() Policy, emit func(string, any)) *History {
	return &History{config: config, project: project, policy: policy, emit: emit, now: time.Now}
}

func (h *History) dir() (string, bool) {
	root := h.project()
	if root == nil {
		return "", false
	}
	return path.Join(Dir, projectID(root.Dir())), true
}

// Add stores a finished run (runsvc's Record hook).
func (h *History) Add(s *runsvc.Summary, results []*engine.UnitResult) {
	p := h.policy()
	dir, ok := h.dir()
	if !p.Enabled || !ok || s == nil || s.Error != "" {
		return
	}
	rec := Record{ID: s.StartedAt.UTC().Format("20060102T150405.000000000Z") + "-" + safe(s.RunID), Summary: s, Results: []report.Result{}}
	for _, res := range results {
		rec.Results = append(rec.Results, redacted(res))
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	h.mu.Lock()
	_ = h.config.MkdirAll(dir, 0o700)
	err = h.config.WriteFileAtomic(path.Join(dir, rec.ID+".json"), data, 0o600)
	h.prune(dir, p)
	h.mu.Unlock()
	if err == nil && h.emit != nil {
		h.emit(TopicChanged, rec.ID)
	}
}

// List returns the project's runs, newest first.
func (h *History) List() ([]Item, error) {
	dir, ok := h.dir()
	if !ok {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prune(dir, h.policy())
	entries, err := h.config.ReadDir(dir)
	if err != nil {
		return []Item{}, nil //nolint:nilerr // no history yet
	}
	out := []Item{}
	for _, e := range entries {
		rec, err := h.read(path.Join(dir, e.Name()))
		if err != nil || rec.Summary == nil {
			continue
		}
		s := rec.Summary
		it := Item{ID: rec.ID, At: s.StartedAt, Kind: s.Kind, Env: s.Env, Outcome: s.Outcome,
			Requests: s.Requests, Succeeded: s.Succeeded, Duration: s.Duration, Files: []string{}}
		for _, u := range s.Units {
			if !slices.Contains(it.Files, u.File) {
				it.Files = append(it.Files, u.File)
			}
		}
		it.Calls = calls(rec)
		out = append(out, it)
	}
	slices.SortFunc(out, func(a, b Item) int { return b.At.Compare(a.At) })
	return out, nil
}

// Get returns run id.
func (h *History) Get(id string) (*Record, error) {
	dir, ok := h.dir()
	if !ok {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	if id != safe(id) {
		return nil, apperr.New(apperr.Invalid, "not a history id")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	rec, err := h.read(path.Join(dir, id+".json"))
	if err != nil {
		return nil, apperr.New(apperr.NotFound, "that run is no longer in the history")
	}
	return rec, nil
}

// Clear deletes the project's history.
func (h *History) Clear() error {
	dir, ok := h.dir()
	if !ok {
		return apperr.New(apperr.NotFound, "no project is open")
	}
	h.mu.Lock()
	entries, _ := h.config.ReadDir(dir)
	for _, e := range entries {
		_ = h.config.Remove(path.Join(dir, e.Name()))
	}
	h.mu.Unlock()
	if h.emit != nil {
		h.emit(TopicChanged, "")
	}
	return nil
}

func (h *History) read(name string) (*Record, error) {
	data, err := h.config.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// prune deletes runs older than the retention.
func (h *History) prune(dir string, p Policy) {
	if p.Retention <= 0 {
		return
	}
	cutoff := h.now().Add(-p.Retention).UTC().Format("20060102T150405")
	entries, _ := h.config.ReadDir(dir)
	for _, e := range entries {
		if e.Name() < cutoff {
			_ = h.config.Remove(path.Join(dir, e.Name()))
		}
	}
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func safe(s string) string { return unsafe.ReplaceAllString(s, "_") }

// credentialHeaders have their values masked.
var credentialHeaders = []string{"authorization", "proxy-authorization", "cookie", "set-cookie"}

// redacted converts res for the history. Captured tokens (by name or
// shape) join the run's secrets for every text of the result, then
// credential headers and every cookie value are masked.
func redacted(res *engine.UnitResult) report.Result {
	tokens := redact.New()
	// A short cookie value would mask every text containing it: the cookie
	// headers and lists mask it where it is.
	add := func(name, value string) {
		if credential.Maskable(value) {
			tokens.Add(name, value)
		}
	}
	// Cookie values are masked wherever they appear (a capture, an assert
	// message), not only in the cookie lists.
	for _, c := range res.Cookies {
		add(c.Name, c.Value)
	}
	for _, e := range res.Entries {
		for _, call := range e.Calls {
			if v, ok := call.Request.Headers.Get("Cookie"); ok {
				for _, part := range strings.Split(v, ";") {
					if _, val, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
						add("cookie", val)
					}
				}
			}
			if call.Response != nil {
				for _, c := range call.Response.Cookies() {
					add(c.Name, c.Value)
				}
			}
		}
	}
	for _, e := range res.Entries {
		for _, c := range e.Captures {
			text, ok := c.Value.Text()
			if !ok {
				continue
			}
			if credential.Likely(c.Name, text) {
				tokens.Add(c.Name, text)
			}
		}
	}
	mask := func(s string) string { return tokens.Redact(res.Redact(s)) }
	r, err := report.JSON(res, mask, nil)
	if err != nil {
		return report.Result{Filename: res.File}
	}
	for i := range r.Cookies {
		r.Cookies[i].Value = redact.Mask
	}
	for i := range r.Entries {
		for j := range r.Entries[i].Calls {
			c := &r.Entries[i].Calls[j]
			maskHeaders(c.Request.Headers)
			maskHeaders(c.Response.Headers)
			for k := range c.Request.Cookies {
				c.Request.Cookies[k].Value = redact.Mask
			}
			for k := range c.Response.Cookies {
				c.Response.Cookies[k].Value = redact.Mask
			}
		}
		// A curl command repeats header values from the file itself
		// (a static Authorization, …): it is not kept.
		r.Entries[i].CurlCmd = ""
		for j := range r.Entries[i].Captures {
			if credential.Likely(r.Entries[i].Captures[j].Name, "") {
				r.Entries[i].Captures[j].Value = redact.Mask
			}
		}
	}
	return r
}

func maskHeaders(hs []report.NameValue) {
	for i := range hs {
		if slices.Contains(credentialHeaders, strings.ToLower(hs[i].Name)) {
			hs[i].Value = redact.Mask
		}
	}
}

// projectID names a project's folder in the history.
func projectID(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return hex.EncodeToString(sum[:8])
}

// Service is the history bindings.
type Service struct{ h *History }

// NewService returns the bindings over h.
func NewService(h *History) *Service { return &Service{h: h} }

// List returns the project's runs, newest first.
func (s *Service) List() ([]Item, error) { return s.h.List() }

// Get returns a stored run.
func (s *Service) Get(id string) (*Record, error) { return s.h.Get(id) }

// Clear deletes the project's history.
func (s *Service) Clear() error { return s.h.Clear() }

// calls lists a record's requests, last first: each entry's last call
// (the attempt that counted), in the file of its unit.
func calls(rec *Record) []Call {
	out := []Call{}
	for i, res := range rec.Results {
		file := path.Base(filepath.ToSlash(res.Filename))
		if i < len(rec.Summary.Units) {
			file = rec.Summary.Units[i].File
		}
		for _, e := range res.Entries {
			if len(e.Calls) == 0 {
				continue
			}
			c := e.Calls[len(e.Calls)-1]
			failed := slices.ContainsFunc(e.Asserts, func(a report.Assert) bool { return !a.Success })
			out = append(out, Call{Result: i, File: file, Entry: e.Index, Method: c.Request.Method, URL: c.Request.URL, Status: c.Response.Status, Duration: e.Time, Failed: failed})
		}
	}
	slices.Reverse(out)
	// The list shows the last ones; the record keeps them all.
	return out[:min(len(out), maxCalls)]
}

// maxCalls bounds the requests a history item lists.
const maxCalls = 50
