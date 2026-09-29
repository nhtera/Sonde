// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package bodies

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/sandbox"
)

func spillRoot(t *testing.T) (*sandbox.Root, string) {
	t.Helper()
	dir := t.TempDir()
	r, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, dir
}

func serve(s *Store, method, id string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, "/x", nil)
	r.URL.Path = id // the asset server strips the route
	s.ServeHTTP(w, r)
	return w
}

func TestServe(t *testing.T) {
	s := New(nil)
	id := s.Put([]byte(`{"a":"***"}`), []byte(`{"a":"secret"}`), "application/json")
	if len(id) != 32 {
		t.Errorf("id %q", id)
	}
	w := serve(s, "GET", id)
	if w.Code != 200 || w.Body.String() != `{"a":"***"}` {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	h := w.Header()
	if h.Get("Content-Type") != "application/json" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(h.Get("Content-Security-Policy"), "sandbox;") || h.Get("Cache-Control") != "no-store" {
		t.Errorf("headers %v", h)
	}
	if w := serve(s, "HEAD", id); w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "11" {
		t.Errorf("HEAD %d %q", w.Code, w.Body.String())
	}
	if w := serve(s, "GET", "nope"); w.Code != 404 {
		t.Errorf("unknown id %d", w.Code)
	}
	if w := serve(s, "POST", id); w.Code != 405 {
		t.Errorf("POST %d", w.Code)
	}
	if raw, _, ok := s.Raw(id); !ok || string(raw) != `{"a":"secret"}` {
		t.Errorf("raw %q %v", raw, ok)
	}
}

func TestSpillKeepsOnlyRedacted(t *testing.T) {
	root, dir := spillRoot(t)
	s := New(root)
	s.memLimit = 20 // one body: 8 redacted + 11 raw bytes
	a := s.Put([]byte("aaaa-***"), []byte("aaaa-SECRET"), "text/plain")
	b := s.Put([]byte("bbbb-***"), []byte("bbbb-SECRET"), "text/plain")
	if _, _, ok := s.Raw(a); ok {
		t.Error("a spilled body kept its raw bytes")
	}
	if _, _, ok := s.Raw(b); !ok {
		t.Error("the newest body lost its raw bytes")
	}
	spilled := filepath.Join(dir, spillDir, a)
	data, err := os.ReadFile(spilled)
	if err != nil || string(data) != "aaaa-***" {
		t.Fatalf("spill file %q %v", data, err)
	}
	if fi, _ := os.Stat(spilled); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("spill mode %v", fi.Mode().Perm())
	}
	if w := serve(s, "GET", a); w.Body.String() != "aaaa-***" {
		t.Errorf("spilled body served %q", w.Body.String())
	}
	s.dskLimit = 1
	s.Put([]byte("cccc-***"), nil, "text/plain")
	if w := serve(s, "GET", a); w.Code != 404 {
		t.Errorf("over the disk limit, the oldest must go: %d", w.Code)
	}
	if _, err := os.Stat(spilled); !os.IsNotExist(err) {
		t.Error("dropped spill file kept")
	}
	// A new store removes earlier spill files.
	s.Put([]byte("dddd-***"), nil, "text/plain")
	New(root)
	if entries, _ := os.ReadDir(filepath.Join(dir, spillDir)); len(entries) != 0 {
		t.Errorf("%d spill files survive a restart", len(entries))
	}
}

func TestCanceledFetchStops(t *testing.T) {
	s := New(nil)
	big := bytes.Repeat([]byte("x"), 4*chunk)
	id := s.Put(big, nil, "text/plain")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, "GET", "/x", nil)
	r.URL.Path = id
	s.ServeHTTP(w, r)
	if w.Body.Len() >= len(big) {
		t.Errorf("a canceled fetch got the whole body (%d bytes)", w.Body.Len())
	}
}

func TestSaveResponse(t *testing.T) {
	s := New(nil)
	id := s.Put([]byte("r ***"), []byte("r SECRET"), "application/json")
	out := filepath.Join(t.TempDir(), "saved.json")
	var suggested string
	d := NewDesktop(s, func(name string) (string, error) { suggested = name; return out, nil })
	name, err := d.SaveResponse(id)
	if err != nil || name != "saved.json" || suggested != "response.json" {
		t.Fatalf("save %q %v (suggested %q)", name, err, suggested)
	}
	if data, _ := os.ReadFile(out); string(data) != "r SECRET" {
		t.Errorf("saved %q: the user's own save is the raw body", data)
	}
	d.pickSave = func(string) (string, error) { return "", nil }
	if name, err := d.SaveResponse(id); name != "" || err != nil {
		t.Errorf("canceled: %q %v", name, err)
	}
	if _, err := d.SaveResponse("nope"); err == nil {
		t.Error("unknown body saved")
	}
}

func TestExtension(t *testing.T) {
	for ct, want := range map[string]string{
		"application/json; charset=utf-8": ".json", "application/problem+json": ".json",
		"text/xml": ".xml", "image/png": ".png", "application/pdf": ".pdf", "text/html": ".html",
		"text/csv": ".txt", "application/octet-stream": ".bin", "": ".bin",
	} {
		if got := extension(ct); got != want {
			t.Errorf("%q: %q, want %q", ct, got, want)
		}
	}
	if allowed[".html"] || allowed[".bin"] {
		t.Error("html and binary must open as text")
	}
}

var _ http.Handler = (*Route)(nil)
