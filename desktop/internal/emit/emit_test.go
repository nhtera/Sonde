// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package emit

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecorder(t *testing.T) {
	var r Recorder
	r.Emit("a", 1)
	r.Emit("b", map[string]string{"k": "v"})
	if got := r.Events(); len(got) != 2 || got[0].Topic != "a" || got[1].Topic != "b" {
		t.Fatalf("events %+v", got)
	}
}

func TestStreamDeliversInOrder(t *testing.T) {
	s := NewStream()
	srv := httptest.NewServer(s)
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("Content-Type %q", ct)
	}
	waitClients(t, s, 1)
	for i := range 3 {
		s.Emit("run:1", map[string]int{"seq": i})
	}
	lines := bufio.NewScanner(res.Body)
	for i := range 3 {
		if !lines.Scan() {
			t.Fatalf("stream ended at %d: %v", i, lines.Err())
		}
		var ev struct {
			Topic string
			Data  struct{ Seq int }
		}
		if err := json.Unmarshal(lines.Bytes(), &ev); err != nil || ev.Topic != "run:1" || ev.Data.Seq != i {
			t.Fatalf("line %d: %s (%v)", i, lines.Bytes(), err)
		}
	}
}

func TestStreamDropsSlowClient(t *testing.T) {
	s := NewStream()
	c := &client{ch: make(chan []byte, 1)}
	s.clients[c] = struct{}{}
	s.Emit("x", 1)
	s.Emit("x", 2) // buffer full: dropped
	if s.Clients() != 0 {
		t.Fatal("a client that cannot keep up must be dropped")
	}
	if _, ok := <-c.ch; !ok {
		t.Fatal("the buffered event is lost")
	}
	if _, ok := <-c.ch; ok {
		t.Fatal("channel not closed")
	}
	// Too many bytes behind, even with room in the buffer.
	big := &client{ch: make(chan []byte, clientBuffer)}
	s.clients[big] = struct{}{}
	s.Emit("x", strings.Repeat("a", clientBytes/2))
	s.Emit("x", strings.Repeat("a", clientBytes/2))
	if s.Clients() != 0 {
		t.Error("a client 32 MiB behind must be dropped")
	}
}

func TestStreamClientLeaves(t *testing.T) {
	s := NewStream()
	srv := httptest.NewServer(s)
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	waitClients(t, s, 1)
	_ = res.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for s.Clients() != 0 && time.Now().Before(deadline) {
		s.Emit("x", 1) // a write notices the closed connection
		time.Sleep(10 * time.Millisecond)
	}
	if s.Clients() != 0 {
		t.Fatal("client kept after it left")
	}
}

func TestStreamGETOnly(t *testing.T) {
	w := httptest.NewRecorder()
	NewStream().ServeHTTP(w, httptest.NewRequest(http.MethodPost, StreamPath, nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
}

func waitClients(t *testing.T, s *Stream, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.Clients() != n {
		if time.Now().After(deadline) {
			t.Fatalf("%d clients, want %d", s.Clients(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
