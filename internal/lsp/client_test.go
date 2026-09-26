// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/config"
)

// testClient drives a Server over in-memory pipes, the way an editor does.
type testClient struct {
	t      *testing.T
	conn   *conn
	nextID int
	// replies and notes receive what the server sends.
	replies chan *message
	notes   chan *message
	done    chan error
}

// newTestClient starts a server with environ (nil for none); it is shut
// down when the test ends.
func newTestClient(t *testing.T, environ config.Env) *testClient {
	t.Helper()
	s, err := NewServer(Options{Version: "test", Environ: environ})
	if err != nil {
		t.Fatal(err)
	}
	clientIn, serverOut := io.Pipe()
	serverIn, clientOut := io.Pipe()
	c := &testClient{
		t: t, conn: newConn(clientIn, clientOut),
		replies: make(chan *message, 64), notes: make(chan *message, 256), done: make(chan error, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c.done <- s.Run(ctx, serverIn, serverOut)
		_ = serverOut.Close()
	}()
	go func() {
		for {
			m, err := c.conn.read()
			if err != nil {
				close(c.notes)
				return
			}
			if m.Method != "" {
				c.notes <- m
			} else {
				c.replies <- m
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = clientOut.Close()
		<-c.done
	})
	return c
}

// call sends a request and returns its reply.
func (c *testClient) call(method string, params any) *message {
	c.t.Helper()
	c.nextID++
	if err := c.conn.call(c.nextID, method, params); err != nil {
		c.t.Fatal(err)
	}
	select {
	case m := <-c.replies:
		if string(*m.ID) != strconv.Itoa(c.nextID) {
			c.t.Fatalf("reply id %s, want %d", *m.ID, c.nextID)
		}
		return m
	case <-time.After(10 * time.Second):
		c.t.Fatalf("no reply to %s", method)
		return nil
	}
}

// result sends a request and decodes its successful result into v.
func (c *testClient) result(method string, params, v any) {
	c.t.Helper()
	m := c.call(method, params)
	if m.Error != nil {
		c.t.Fatalf("%s: %v", method, m.Error)
	}
	if v != nil {
		if err := json.Unmarshal(m.Result, v); err != nil {
			c.t.Fatalf("%s: decoding %s: %v", method, m.Result, err)
		}
	}
}

func (c *testClient) notify(method string, params any) {
	c.t.Helper()
	if err := c.conn.notify(method, params); err != nil {
		c.t.Fatal(err)
	}
}

// initialize runs the handshake. utf8 offers the utf-8 position encoding;
// folder (may be "") is the single workspace folder; initOpts is sent as
// initializationOptions when not nil.
func (c *testClient) initialize(utf8 bool, folder string, initOpts any) initializeResult {
	c.t.Helper()
	params := map[string]any{
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"completion": map[string]any{"completionItem": map[string]any{"snippetSupport": true}},
				"hover":      map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
			},
			"workspace": map[string]any{"didChangeWatchedFiles": map[string]any{"dynamicRegistration": true}},
		},
	}
	if utf8 {
		params["capabilities"].(map[string]any)["general"] = map[string]any{"positionEncodings": []string{"utf-8", "utf-16"}}
	}
	if folder != "" {
		params["workspaceFolders"] = []workspaceFolder{{URI: pathToURI(folder), Name: "ws"}}
	}
	if initOpts != nil {
		params["initializationOptions"] = initOpts
	}
	var res initializeResult
	c.result("initialize", params, &res)
	c.notify("initialized", map[string]any{})
	return res
}

// open sends didOpen and returns the diagnostics published for it.
func (c *testClient) open(uri, text string) []Diagnostic {
	c.t.Helper()
	c.notify("textDocument/didOpen", didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "hurl", Version: 1, Text: text}})
	return c.diagnostics(uri)
}

// change replaces a document's text and returns its new diagnostics.
func (c *testClient) change(uri string, version int32, text string) []Diagnostic {
	c.t.Helper()
	c.notify("textDocument/didChange", didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: []contentChange{{Text: text}},
	})
	return c.diagnostics(uri)
}

// diagnostics waits for the next publishDiagnostics for uri, skipping other
// notifications (e.g. client/registerCapability).
func (c *testClient) diagnostics(uri string) []Diagnostic {
	c.t.Helper()
	m := c.next("textDocument/publishDiagnostics", func(m *message) bool {
		var p publishDiagnosticsParams
		return json.Unmarshal(m.Params, &p) == nil && p.URI == uri
	})
	var p publishDiagnosticsParams
	_ = json.Unmarshal(m.Params, &p)
	return p.Diagnostics
}

// next waits for the next server message with method that satisfies ok.
func (c *testClient) next(method string, ok func(*message) bool) *message {
	c.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case m, open := <-c.notes:
			if !open {
				c.t.Fatalf("connection closed waiting for %s", method)
			}
			if m.Method == method && (ok == nil || ok(m)) {
				return m
			}
		case <-timeout:
			c.t.Fatalf("no %s", method)
		}
	}
}

// completion returns the labels proposed at pos.
func (c *testClient) completion(uri string, pos Position) []CompletionItem {
	c.t.Helper()
	var list completionList
	c.result("textDocument/completion", textDocumentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: pos}, &list)
	return list.Items
}

// hover returns the hover at pos, or nil.
func (c *testClient) hover(uri string, pos Position) *Hover {
	c.t.Helper()
	var h *Hover
	c.result("textDocument/hover", textDocumentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: pos}, &h)
	return h
}

// shutdown sends shutdown and exit and returns Run's error.
func (c *testClient) shutdown() error {
	c.t.Helper()
	c.result("shutdown", nil, nil)
	c.notify("exit", nil)
	select {
	case err := <-c.done:
		c.done <- err // for Cleanup
		return err
	case <-time.After(10 * time.Second):
		return errors.New("server did not exit")
	}
}

// pathToURI returns the file URI of an absolute local path.
func pathToURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
