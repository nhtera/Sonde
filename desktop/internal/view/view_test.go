// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/enginex"
)

const (
	declared = "declared-sentinel-91" //nolint:gosec // G101: test sentinel
	captured = "captured-sentinel-42" //nolint:gosec // G101: test sentinel
)

// memBodies stores bodies in memory.
type memBodies struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memBodies) Put(data, _ []byte, _ string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := strconv.Itoa(len(b.m))
	b.m[id] = data
	return id
}

// echo answers with the secrets in headers, text, binary and gzip bodies.
func echo(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("X-Token", captured)
			_, _ = io.WriteString(w, `{"token":"`+captured+`"}`)
		case "/text":
			w.Header().Set("X-Echo", r.Header.Get("Authorization"))
			reply := "auth=" + r.Header.Get("Authorization") + " key=" + r.URL.Query().Get("k")
			_, _ = io.WriteString(w, reply) //nolint:gosec // G705: a test echo server
		case "/bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(append([]byte{0, 1, 2, 0xff}, []byte(captured+"\x00"+declared)...))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", "text/plain")
			zw := gzip.NewWriter(w)
			_, _ = io.WriteString(zw, "zipped "+captured)
			_ = zw.Close()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

const src = `GET {{base}}/token
HTTP 200
[Captures]
tok: header "X-Token" redact

GET {{base}}/text?k={{key}}
Authorization: Bearer {{tok}}
HTTP 200

GET {{base}}/bin
HTTP 200

GET {{base}}/gzip
HTTP 200

GET {{base}}/skipped
[Options]
skip: true
HTTP 200
`

// run runs src through converters and returns every DTO.
func run(t *testing.T, bodies BodyStore) []any {
	t.Helper()
	srv := echo(t)
	var (
		mu   sync.Mutex
		dtos []any
	)
	r := engine.NewRunner(engine.Options{
		Variables:    map[string]any{"base": srv.URL},
		Secrets:      map[string]string{"key": declared},
		Verbosity:    engine.Verbose,
		BufferedLogs: true,
	})
	enginex.EnableHostEvents(r)
	var conv *Converter
	r.RunAll(context.Background(), slices.Values([]engine.Job{{Name: filepath.Join(t.TempDir(), "t.hurl"), Source: []byte(src)}}), engine.RunAllOptions{
		Started: func(int, engine.Job) (func(engine.Event), io.Writer) {
			conv = NewConverter("t.hurl", r.Redact, bodies, func(dto any) {
				mu.Lock()
				dtos = append(dtos, dto)
				mu.Unlock()
			})
			return conv.Handle, nil
		},
		Finished: func(_ int, _ engine.Job, res *engine.UnitResult, err error) bool {
			if err != nil || !res.Success {
				t.Errorf("run failed: %v %v", err, res.Errors())
			}
			conv.Flush()
			return true
		},
	})
	return dtos
}

// TestNoSecretInAnyDTO is the sentinel: no DTO and no stored body holds a
// declared secret or a redacted capture, while the run did carry both.
func TestNoSecretInAnyDTO(t *testing.T) {
	bodies := &memBodies{m: map[string][]byte{}}
	dtos := run(t, bodies)
	if len(dtos) == 0 {
		t.Fatal("no events")
	}
	for _, d := range dtos {
		redactcheck.AssertNoSecret(t, "event", d, declared, captured)
	}
	if len(bodies.m) < 3 {
		t.Fatalf("%d bodies stored, want the text, binary and gzip ones", len(bodies.m))
	}
	for id, b := range bodies.m {
		redactcheck.AssertNoSecretBytes(t, "body "+id, b, declared, captured)
	}
}

func TestEventShapes(t *testing.T) {
	bodies := &memBodies{m: map[string][]byte{}}
	dtos := run(t, bodies)
	var (
		started, finished, sent, logs int
		skipped                       []EntrySkipped
		entries                       []Entry
	)
	for _, d := range dtos {
		switch e := d.(type) {
		case EntryStarted:
			started++
		case EntryFinished:
			finished++
			entries = append(entries, e.Entry)
		case RequestSent:
			sent++
			if e.Request.Method != "GET" || !strings.Contains(e.Request.URL, "/") {
				t.Errorf("request sent %+v", e.Request)
			}
		case EntrySkipped:
			skipped = append(skipped, e)
		case Log:
			logs++
			if e.Level == "" {
				t.Errorf("log without level: %+v", e)
			}
		}
	}
	if started != 4 || finished != 4 || sent != 4 || logs == 0 {
		t.Errorf("started %d finished %d sent %d logs %d", started, finished, sent, logs)
	}
	if len(skipped) != 1 || skipped[0].Entry != 5 || skipped[0].Reason != "option" {
		t.Errorf("skipped %+v", skipped)
	}
	gz := entries[3]
	if len(gz.Bodies) != 1 || gz.Bodies[0].ID == "" || gz.Bodies[0].ContentType != "text/plain" {
		t.Fatalf("gzip body %+v", gz.Bodies)
	}
	if got := string(bodies.m[gz.Bodies[0].ID]); got != "zipped ***" {
		t.Errorf("gzip body stored as %q, want it decoded and redacted", got)
	}
	bin := bodies.m[entries[2].Bodies[0].ID]
	if !bytes.HasPrefix(bin, []byte{0, 1, 2, 0xff}) || !bytes.Contains(bin, []byte("***")) {
		t.Errorf("binary body %q", bin)
	}
	if !entries[0].Success || entries[0].Index != 1 || len(entries[0].Captures) != 1 {
		t.Errorf("entry 1 %+v", entries[0])
	}
}

// TestHeldUntilFinished: the attempt with the redact capture is held, so
// its DTOs arrive together with its EntryFinished.
func TestHeldUntilFinished(t *testing.T) {
	dtos := run(t, nil)
	firstFinished, lastEntry1 := -1, -1
	for i, d := range dtos {
		switch e := d.(type) {
		case Log:
			if e.Entry == 1 {
				lastEntry1 = i
			}
		case EntryFinished:
			if firstFinished < 0 {
				firstFinished = i
			}
		}
	}
	if lastEntry1 < 0 || firstFinished < lastEntry1 {
		t.Errorf("entry 1 logs at %d, its finish at %d: held logs must precede the finish", lastEntry1, firstFinished)
	}
}

// TestNoRedactorYetHolds: events before the unit starts wait for its
// redactor.
func TestNoRedactorYetHolds(t *testing.T) {
	var out []any
	c := NewConverter("f", func(s string) string { return strings.ReplaceAll(s, "x", "*") }, nil, func(d any) { out = append(out, d) })
	c.Handle(engine.EntryStarted{Index: 1})
	c.Handle(engine.Log{Text: "x"})
	if len(out) != 0 {
		t.Fatal("converted before the unit started")
	}
	c.Flush()
	if len(out) != 2 || out[1].(Log).Text != "*" {
		t.Fatalf("flush %+v", out)
	}
}
