// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/enginex"
)

// eventPayload is every text an event carries, raw, for sentinel checks.
func eventPayload(ev Event) string {
	var b strings.Builder
	switch e := ev.(type) {
	case Log:
		b.WriteString(e.Text + "\n" + e.Color)
	case EntryFinished:
		for _, c := range e.Result.Calls {
			b.WriteString(c.Request.URL + "\n" + string(c.Request.Body) + "\n")
			for _, h := range c.Request.Headers {
				b.WriteString(h.Name + ": " + h.Value + "\n")
			}
			b.WriteString(string(c.Response.Body) + "\n")
		}
		for _, c := range e.Result.Captures {
			b.WriteString(c.Name + "=" + c.Value.String() + "\n")
		}
		b.WriteString(e.Result.Curl)
	default:
		if _, _, req, ok := enginex.RequestSent(ev); ok {
			b.WriteString(req.URL + "\n" + string(req.Body) + "\n")
			for _, h := range req.Headers {
				b.WriteString(h.Name + ": " + h.Value + "\n")
			}
		}
	}
	return b.String()
}

// TestUnitStartedLiveRedactor runs a file with data rows: every event
// redacted as it arrives with its unit's live redactor holds neither the
// row's secret nor the row's `redact` capture, while the raw events do.
func TestUnitStartedLiveRedactor(t *testing.T) {
	srv := server(t)
	const rowSecret = "row-sentinel-7Q" //nolint:gosec // G101: test sentinel
	const captured = "s3cr3t-token"     // the /json token, a `redact` capture
	src := `GET {{base}}/json?k={{rowkey}}
HTTP 200
[Captures]
tok: jsonpath "$.token" redact

GET {{base}}/echo
Authorization: Bearer {{tok}}
X-Row: {{rowkey}}
HTTP 200
`
	r := NewRunner(Options{Variables: map[string]any{"base": srv.URL}, Verbosity: VeryVerbose, BufferedLogs: true})
	enginex.EnableHostEvents(r)
	type unitLog struct {
		redact        func(string) string
		raw, redacted []string
		unitSaw       bool
	}
	units := map[int]*unitLog{}
	jobs := slices.Values([]Job{
		{Name: filepath.Join(t.TempDir(), "t.hurl"), Source: []byte(src), Row: &Row{Index: 1, Secrets: map[string]string{"rowkey": rowSecret}}},
		{Name: filepath.Join(t.TempDir(), "t.hurl"), Source: []byte(src), Row: &Row{Index: 2, Secrets: map[string]string{"rowkey": rowSecret + "-2"}}},
	})
	var results []*UnitResult
	r.RunAll(context.Background(), jobs, RunAllOptions{
		Parallel: 2,
		Started: func(seq int, _ Job) (func(Event), io.Writer) {
			ul := &unitLog{}
			units[seq] = ul
			return func(ev Event) {
				if redact, ok := enginex.UnitStarted(ev); ok {
					if ul.unitSaw || len(ul.raw) > 0 {
						t.Errorf("unit %d: UnitStarted is not the first event", seq)
					}
					ul.redact, ul.unitSaw = redact, true
					return
				}
				if ul.redact == nil {
					t.Errorf("unit %d: event %T before UnitStarted", seq, ev)
					return
				}
				p := eventPayload(ev)
				ul.raw = append(ul.raw, p)
				ul.redacted = append(ul.redacted, ul.redact(p))
			}, nil
		},
		Finished: func(_ int, _ Job, res *UnitResult, err error) bool {
			if err != nil {
				t.Error(err)
			}
			results = append(results, res)
			return true
		},
	})
	for _, res := range results {
		if !res.Success {
			t.Fatalf("%s: %v", res.Label(), res.Errors())
		}
	}
	for seq, ul := range units {
		raw, red := strings.Join(ul.raw, "\n"), strings.Join(ul.redacted, "\n")
		for _, s := range []string{rowSecret, captured} {
			if !strings.Contains(raw, s) {
				t.Errorf("unit %d: raw events never carry %q: the test proves nothing", seq, s)
			}
			if strings.Contains(red, s) {
				t.Errorf("unit %d: %q in a redacted event:\n%s", seq, s, red)
			}
		}
	}
	if strings.Contains(r.Redact(rowSecret), "***") {
		t.Error("a row secret reached the run's secrets")
	}
}

// TestEntryRedactsHold captures a response header as a secret, which the
// entry's own logs show before the capture runs: redacted as they arrive
// they leak it; held from EntryRedacts to EntryFinished and redacted then,
// they do not. An entry without a `redact` capture is not held.
func TestEntryRedactsHold(t *testing.T) {
	const hdr = "hdr-sentinel-55" //nolint:gosec // G101: test sentinel
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Token", hdr)
	}))
	t.Cleanup(srv.Close)
	src := "GET {{base}}/tok\nHTTP 200\n[Captures]\ntok: header \"X-Token\" redact\n\nGET {{base}}/next\nHTTP 200\n"
	var (
		redact      func(string) string
		live, shown []string
		held        []Event
		holding     bool
		flagged     []int
	)
	r := NewRunner(Options{Variables: map[string]any{"base": srv.URL}, Verbosity: Verbose, BufferedLogs: true,
		OnEvent: func(ev Event) {
			if fn, ok := enginex.UnitStarted(ev); ok {
				redact = fn
				return
			}
			if i, ok := enginex.EntryRedacts(ev); ok {
				flagged = append(flagged, i)
				holding = true
				return
			}
			live = append(live, redact(eventPayload(ev)))
			if holding {
				held = append(held, ev)
				if _, ok := ev.(EntryFinished); !ok {
					return
				}
				for _, h := range held {
					shown = append(shown, redact(eventPayload(h)))
				}
				held, holding = nil, false
				return
			}
			shown = append(shown, redact(eventPayload(ev)))
		}})
	enginex.EnableHostEvents(r)
	res, err := r.RunSource(context.Background(), filepath.Join(t.TempDir(), "t.hurl"), []byte(src))
	if err != nil || !res.Success {
		t.Fatalf("%v %v", err, res.Errors())
	}
	if len(flagged) != 1 || flagged[0] != 1 {
		t.Errorf("flagged entries %v, want [1]", flagged)
	}
	if !strings.Contains(strings.Join(live, "\n"), hdr) {
		t.Error("redacting as events arrive does not leak: the test proves nothing")
	}
	if all := strings.Join(shown, "\n"); strings.Contains(all, hdr) {
		t.Errorf("held events leak the capture:\n%s", all)
	}
}
