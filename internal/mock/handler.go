// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package mock serves an OpenAPI mock over HTTP (sonde mock): the
// net/http side of openapi.Mock, which chooses and renders responses.
package mock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nhtera/sonde/internal/openapi"
)

// maxBodyBytes caps a request body: a larger one is answered 413.
const maxBodyBytes = 16 << 20

// Options configure the handler.
type Options struct {
	// ValidateRequests answers a request that does not match its
	// operation with a 415 or 422 problem instead of a response.
	ValidateRequests bool
	// CORS answers preflight requests and allows any origin.
	CORS bool
	// Log receives one line per request and generator warnings (nil:
	// none). Bodies are never logged.
	Log io.Writer
}

// Handler returns the mock's HTTP handler.
func Handler(m *openapi.Mock, opt Options) http.Handler {
	return &handler{m: m, opt: opt}
}

type handler struct {
	m      *openapi.Mock
	opt    Options
	logMu  sync.Mutex
	warned sync.Map // warnings already logged
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		h.logf("%s %s %d %s\n", r.Method, r.URL.EscapedPath(), rec.status, time.Since(start).Round(time.Microsecond))
	}()
	if h.opt.CORS && h.cors(rec, r) {
		return
	}
	op, p := h.m.Match(r.Method, r.URL.EscapedPath())
	if p != nil {
		writeProblem(rec, p)
		return
	}
	// The body is read once routed: an unknown path never buffers it.
	body, err := io.ReadAll(http.MaxBytesReader(rec, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(rec, &openapi.Problem{Status: http.StatusRequestEntityTooLarge, Detail: fmt.Sprintf("the request body is larger than %d bytes", maxBodyBytes)})
			return
		}
		writeProblem(rec, &openapi.Problem{Status: http.StatusBadRequest, Detail: "the request body can not be read: " + err.Error()})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	if h.opt.ValidateRequests {
		if p := h.m.ValidateRequest(r.Context(), op, r); p != nil {
			writeProblem(rec, p)
			return
		}
	}
	code, example := parsePrefer(r.Header.Values("Prefer"))
	resp, p := h.m.Respond(op, openapi.Selection{Status: code, Example: example, Accept: strings.Join(r.Header.Values("Accept"), ",")})
	if p != nil {
		writeProblem(rec, p)
		return
	}
	if resp.Warning != "" {
		if _, seen := h.warned.LoadOrStore(resp.Warning, true); !seen {
			h.logf("warning: %s\n", resp.Warning)
		}
	}
	for k, vs := range resp.Header {
		rec.Header()[k] = vs
	}
	rec.Header().Set("Content-Length", strconv.Itoa(len(resp.Body)))
	rec.WriteHeader(resp.Status)
	if r.Method != http.MethodHead {
		_, _ = rec.Write(resp.Body)
	}
}

// cors sets the CORS headers of a response to any origin, and answers a
// preflight request itself (true). Credentials are never allowed.
func (h *handler) cors(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	hd := w.Header()
	if origin == "" {
		hd.Set("Access-Control-Allow-Origin", "*")
	} else {
		hd.Set("Access-Control-Allow-Origin", origin)
		hd.Add("Vary", "Origin")
	}
	method := r.Header.Get("Access-Control-Request-Method")
	if r.Method != http.MethodOptions || origin == "" || method == "" {
		hd.Set("Access-Control-Expose-Headers", "*")
		return false
	}
	hd.Set("Access-Control-Allow-Methods", method)
	if hs := r.Header.Get("Access-Control-Request-Headers"); hs != "" {
		hd.Set("Access-Control-Allow-Headers", hs)
	}
	hd.Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
	return true
}

func (h *handler) logf(format string, args ...any) {
	if h.opt.Log == nil {
		return
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	_, _ = fmt.Fprintf(h.opt.Log, format, args...)
}

// parsePrefer returns the code and example preferences (RFC 7240) of
// the Prefer headers: "Prefer: code=404, example=missing". The first
// value of a preference wins; a quoted value may hold commas.
func parsePrefer(values []string) (code, example string) {
	var seenCode, seenExample bool
	for _, v := range values {
		for _, pref := range splitUnquoted(v, ',') {
			// Parameters of a preference (";...") are ignored.
			pref = splitUnquoted(pref, ';')[0]
			name, value, ok := strings.Cut(strings.TrimSpace(pref), "=")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
			}
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "code":
				if !seenCode {
					code, seenCode = value, true
				}
			case "example":
				if !seenExample {
					example, seenExample = value, true
				}
			}
		}
	}
	return code, example
}

// splitUnquoted splits s at each sep outside double quotes.
func splitUnquoted(s string, sep byte) []string {
	var out []string
	quoted, last := false, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case sep:
			if !quoted {
				out = append(out, s[last:i])
				last = i + 1
			}
		}
	}
	return append(out, s[last:])
}

// problem is an RFC 9457 problem document.
type problem struct {
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	Status     int      `json:"status"`
	Detail     string   `json:"detail"`
	Violations []string `json:"violations,omitempty"`
}

func writeProblem(w http.ResponseWriter, p *openapi.Problem) {
	body, _ := json.Marshal(problem{Type: "about:blank", Title: http.StatusText(p.Status), Status: p.Status, Detail: p.Detail, Violations: p.Violations})
	if len(p.Allow) > 0 {
		w.Header().Set("Allow", strings.Join(p.Allow, ", "))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(p.Status)
	_, _ = w.Write(body)
}

// recorder remembers the status written, for the access log.
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
