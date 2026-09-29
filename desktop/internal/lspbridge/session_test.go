// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lspbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/lsp"
)

// client is a test client: it collects server messages by id and method.
type client struct {
	t    *testing.T
	s    *Session
	msgs chan map[string]json.RawMessage
}

func open(ctx context.Context, t *testing.T) *client {
	t.Helper()
	c := &client{t: t, msgs: make(chan map[string]json.RawMessage, 64)}
	s, err := Start(ctx, lsp.Options{Version: "test"}, func(msg []byte) {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(msg, &m); err != nil {
			t.Errorf("server sent bad JSON: %v", err)
			return
		}
		c.msgs <- m
	})
	if err != nil {
		t.Fatal(err)
	}
	c.s = s
	return c
}

func (c *client) send(v string) {
	c.t.Helper()
	if err := c.s.Send([]byte(v)); err != nil {
		c.t.Fatal(err)
	}
}

// await returns the first message matching id (a response) or method (a
// notification), skipping others.
func (c *client) await(id, method string) map[string]json.RawMessage {
	c.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-c.msgs:
			if id != "" && string(m["id"]) == id {
				return m
			}
			if method != "" && string(m["method"]) == `"`+method+`"` {
				return m
			}
		case <-timeout:
			c.t.Fatalf("no message id=%s method=%s", id, method)
		}
	}
}

func initialize(dir string) string {
	u := (&url.URL{Scheme: "file", Path: filepath.ToSlash(dir)}).String()
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":%q,"capabilities":{}}}`, u)
}

// TestSessionPerClient: a reload opens a second session, whose initialize
// succeeds; initialize twice on one session is refused.
func TestSessionPerClient(t *testing.T) {
	dir := t.TempDir()
	first := open(t.Context(), t)
	first.send(initialize(dir))
	if r := first.await("1", ""); r["error"] != nil {
		t.Fatalf("initialize: %s", r["error"])
	}
	first.send(strings.Replace(initialize(dir), `"id":1`, `"id":2`, 1))
	if r := first.await("2", ""); !strings.Contains(string(r["error"]), "initialize sent twice") {
		t.Fatalf("second initialize on one session: %s", r["error"])
	}
	if err := first.s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.s.Send([]byte(`{}`)); !errors.Is(err, ErrClosed) {
		t.Errorf("send after close: %v", err)
	}

	reload := open(t.Context(), t)
	defer reload.s.Close()
	reload.send(initialize(dir))
	if r := reload.await("1", ""); r["error"] != nil {
		t.Fatalf("initialize after reload: %s", r["error"])
	}
}

// TestSessionEndsWithContext: canceling the context (the page went away)
// stops the server and closes the pipes.
func TestSessionEndsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c := open(ctx, t)
	c.send(initialize(t.TempDir()))
	c.await("1", "")
	cancel()
	select {
	case <-c.s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not end")
	}
	if err := c.s.Send([]byte(`{}`)); !errors.Is(err, ErrClosed) {
		t.Errorf("send after cancel: %v", err)
	}
}

// TestSessionExit: shutdown and exit end the session cleanly.
func TestSessionExit(t *testing.T) {
	c := open(t.Context(), t)
	c.send(initialize(t.TempDir()))
	c.await("1", "")
	c.send(`{"jsonrpc":"2.0","id":9,"method":"shutdown"}`)
	c.await("9", "")
	c.send(`{"jsonrpc":"2.0","method":"exit"}`)
	select {
	case <-c.s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not end on exit")
	}
	if err := c.s.Err(); err != nil {
		t.Errorf("clean exit: %v", err)
	}
}

// TestDiagnosticsLatency measures didOpen to publishDiagnostics (budget
// 150 ms).
func TestDiagnosticsLatency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.hurl")
	src := "GET https://example.org\nHTTP 200\n[Asserts]\nstatus ==\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	c := open(t.Context(), t)
	defer c.s.Close()
	c.send(initialize(dir))
	c.await("1", "")
	c.send(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	text, _ := json.Marshal(src)
	var worst time.Duration
	for i := range 5 {
		start := time.Now()
		c.send(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"hurl","version":%d,"text":%s}}}`, uri, i+1, text))
		m := c.await("", "textDocument/publishDiagnostics")
		worst = max(worst, time.Since(start))
		if !strings.Contains(string(m["params"]), "message") {
			t.Fatalf("no diagnostic for a broken file: %s", m["params"])
		}
		c.send(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didClose","params":{"textDocument":{"uri":%q}}}`, uri))
		c.await("", "textDocument/publishDiagnostics") // cleared on close
	}
	t.Logf("didOpen to diagnostics, worst of 5: %v", worst)
	if worst > 150*time.Millisecond {
		t.Errorf("diagnostics took %v, budget 150ms", worst)
	}
}

func TestReadFrame(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		err      bool
	}{
		{"Content-Length: 2\r\n\r\n{}", "{}", false},
		{"content-length: 2\r\nContent-Type: x\r\n\r\n{}", "{}", false},
		{"Content-Type: x\r\n\r\n{}", "", true},
		{"Content-Length: -1\r\n\r\n", "", true},
		{"Content-Length: 99999999999\r\n\r\n", "", true},
		{"Content-Length: 5\r\n\r\n{}", "", true},
	} {
		got, err := readFrame(bufio.NewReader(strings.NewReader(tc.in)))
		if (err != nil) != tc.err || string(got) != tc.want {
			t.Errorf("%q: %q %v", tc.in, got, err)
		}
	}
}

// TestCloseUnderBackpressure: Close returns while the consumer is stalled
// and a Send is blocked on the server.
func TestCloseUnderBackpressure(t *testing.T) {
	release := make(chan struct{})
	s, err := Start(t.Context(), lsp.Options{}, func([]byte) { <-release })
	if err != nil {
		t.Fatal(err)
	}
	defer close(release)
	_ = s.Send([]byte(initialize(t.TempDir()))) // its reply stalls the consumer
	go func() {
		for range 64 { // queue behind the stalled consumer until Close breaks the pipe
			if s.Send([]byte(`{"jsonrpc":"2.0","id":7,"method":"textDocument/hover","params":{}}`)) != nil {
				return
			}
		}
	}()
	time.Sleep(100 * time.Millisecond)
	closed := make(chan struct{})
	go func() {
		_ = s.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close blocked behind a stalled consumer")
	}
	if err := s.Send([]byte(`{}`)); !errors.Is(err, ErrClosed) {
		t.Errorf("send after close: %v", err)
	}
}
