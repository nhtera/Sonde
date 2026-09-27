// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/syntax"
)

func TestInitializeNegotiatesEncoding(t *testing.T) {
	for _, tt := range []struct {
		utf8 bool
		want string
	}{{true, encodingUTF8}, {false, encodingUTF16}} {
		c := newTestClient(t, nil)
		res := c.initialize(tt.utf8, "", nil)
		caps := res.Capabilities
		if caps.PositionEncoding != tt.want {
			t.Errorf("utf8 offered=%v: encoding %q, want %q", tt.utf8, caps.PositionEncoding, tt.want)
		}
		if caps.TextDocumentSync != syncFull || !caps.HoverProvider || !caps.DocumentFormattingProvider || caps.CompletionProvider == nil {
			t.Errorf("capabilities = %+v", caps)
		}
		if res.ServerInfo.Name != "sonde" || res.ServerInfo.Version != "test" {
			t.Errorf("serverInfo = %+v", res.ServerInfo)
		}
		if err := c.shutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}
}

func TestLifecycleErrors(t *testing.T) {
	c := newTestClient(t, nil)
	if m := c.call("textDocument/hover", map[string]any{}); m.Error == nil || m.Error.Code != codeServerNotInit {
		t.Errorf("before initialize: %+v", m.Error)
	}
	c.initialize(false, "", nil)
	if m := c.call("initialize", map[string]any{}); m.Error == nil || m.Error.Code != codeInvalidRequest {
		t.Errorf("second initialize: %+v", m.Error)
	}
	if m := c.call("sonde/nope", map[string]any{}); m.Error == nil || m.Error.Code != codeMethodNotFound {
		t.Errorf("unknown method: %+v", m.Error)
	}
	if m := c.call("textDocument/hover", "not an object"); m.Error == nil || m.Error.Code != codeInvalidParams {
		t.Errorf("bad params: %+v", m.Error)
	}
	c.result("shutdown", nil, nil)
	if m := c.call("textDocument/hover", map[string]any{}); m.Error == nil || m.Error.Code != codeInvalidRequest {
		t.Errorf("after shutdown: %+v", m.Error)
	}
}

func TestExitWithoutShutdown(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(false, "", nil)
	c.notify("exit", nil)
	if err := <-c.done; !errors.Is(err, ErrExitWithoutShutdown) {
		t.Errorf("Run = %v, want ErrExitWithoutShutdown", err)
	}
	c.done <- nil
}

func TestDiagnosticsPerBrokenEntry(t *testing.T) {
	// Each broken entry reports its own error; astral characters before the
	// error move the UTF-16 column by two and the UTF-8 column by four.
	src := "GET http://a/\nHTTP 2x\n\n" +
		"GET http://b/😀\nX: 😀 😀\n[Options]\nnope: 1\n\n" +
		"GET http://c/\nHTTP 200\n[Asserts]\n😀 == 1\n"
	for _, tt := range []struct {
		utf8 bool
		want []Position
	}{
		{false, []Position{{1, 6}, {6, 0}, {11, 0}}},
		{true, []Position{{1, 6}, {6, 0}, {11, 0}}},
	} {
		c := newTestClient(t, nil)
		c.initialize(tt.utf8, "", nil)
		diags := c.open("file:///w/a.hurl", src)
		if len(diags) != 3 {
			t.Fatalf("utf8=%v: %d diagnostics, want 3: %+v", tt.utf8, len(diags), diags)
		}
		for i, d := range diags {
			if d.Range.Start != tt.want[i] || d.Severity != severityError || d.Source != source {
				t.Errorf("utf8=%v: diag %d = %+v, want start %+v", tt.utf8, i, d, tt.want[i])
			}
		}
	}
}

func TestDiagnosticRangeAstral(t *testing.T) {
	src := "GET http://a/\nHTTP 200\n[Asserts]\nheader \"😀\" 😀x\n"
	for _, tt := range []struct {
		utf8       bool
		start, end uint32
	}{{false, 12, 15}, {true, 14, 19}} {
		c := newTestClient(t, nil)
		c.initialize(tt.utf8, "", nil)
		diags := c.open("file:///w/a.hurl", src)
		if len(diags) != 1 {
			t.Fatalf("%+v", diags)
		}
		r := diags[0].Range
		if r.Start != (Position{3, tt.start}) || r.End != (Position{3, tt.end}) {
			t.Errorf("utf8=%v: range %+v, want 3:%d-3:%d", tt.utf8, r, tt.start, tt.end)
		}
	}
}

func TestChangeAndClose(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(false, "", nil)
	uri := "file:///w/a.hurl"
	if d := c.open(uri, "GET http://a/\nHTTP 2x\n"); len(d) != 1 {
		t.Fatalf("open: %+v", d)
	}
	if d := c.change(uri, 2, "GET http://a/\nHTTP 200\n"); len(d) != 0 {
		t.Errorf("after fix: %+v", d)
	}
	c.notify("textDocument/didClose", didCloseParams{TextDocument: textDocumentIdentifier{URI: uri}})
	if d := c.diagnostics(uri); len(d) != 0 {
		t.Errorf("after close: %+v", d)
	}
	if h := c.hover(uri, Position{}); h != nil {
		t.Errorf("hover on closed doc: %+v", h)
	}
}

func TestFormatting(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(false, "", nil)
	uri := "file:///w/a.sonde"
	c.open(uri, "GET   http://a/\n  X-A:1   \nHTTP 200\n[Asserts]\nstatus   ==  200")
	var edits []TextEdit
	c.result("textDocument/formatting", formattingParams{TextDocument: textDocumentIdentifier{URI: uri}}, &edits)
	if len(edits) != 1 {
		t.Fatalf("edits = %+v", edits)
	}
	want := "GET http://a/\nX-A: 1\nHTTP 200\n[Asserts]\nstatus == 200\n"
	if edits[0].NewText != want || edits[0].Range != (Range{End: Position{4, 16}}) {
		t.Errorf("edit = %+v", edits[0])
	}
	c.change(uri, 2, want)
	c.result("textDocument/formatting", formattingParams{TextDocument: textDocumentIdentifier{URI: uri}}, &edits)
	if len(edits) != 0 {
		t.Errorf("formatted doc: %+v", edits)
	}
	c.change(uri, 3, "GET http://a/\nHTTP 2x\n")
	c.result("textDocument/formatting", formattingParams{TextDocument: textDocumentIdentifier{URI: uri}}, &edits)
	if len(edits) != 0 {
		t.Errorf("broken doc: %+v", edits)
	}
}

// largeDiagnosticsFile builds a well-formed synthetic file of at least
// 1,000 lines / 40 KB, shared by the correctness test and the benchmark
// below (see docs/benchmarks.md).
func largeDiagnosticsFile() string {
	var b strings.Builder
	for b.Len() < 40_000 || strings.Count(b.String(), "\n") < 1000 {
		b.WriteString("GET http://localhost/{{host}}/items?q=1\nAccept: application/json\nHTTP 200\n[Captures]\nid: jsonpath \"$.id\"\n[Asserts]\nstatus == 200\njsonpath \"$.name\" == \"{{id}}\"\n\n")
	}
	return b.String()
}

func TestDiagnosticsOnLargeFile(t *testing.T) {
	// Correctness only (runs under -short and -race); the timing budget on
	// this same file is BenchmarkDiagnostics, reported in docs/benchmarks.md
	// rather than asserted here, since CI never runs benchmarks under -race.
	d := newDocument("file:///w/big.hurl", 1, largeDiagnosticsFile(), true)
	s, err := NewServer(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if diags := s.diagnostics(d); len(diags) != 0 {
		t.Errorf("well-formed 1,000-line file: %d diagnostics, want 0: %+v", len(diags), diags)
	}
}

func BenchmarkDiagnostics(b *testing.B) {
	d := newDocument("file:///w/big.hurl", 1, largeDiagnosticsFile(), true)
	s, err := NewServer(Options{})
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		d.setText(d.text, true)
		s.diagnostics(d)
	}
}

func TestLongLineManyWarningsBudget(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing budget; skipped in -short and -race runs")
	}
	// One 200 KB line of placeholders, none defined: every warning's range
	// is converted on the same line, which must stay linear in UTF-16.
	src := "POST http://x/\n```\n" + strings.Repeat("😀{{a}}", 20_000) + "\n```\n"
	d := newDocument("file:///w/long.hurl", 1, src, true)
	s, err := NewServer(Options{})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	s.diagnostics(d)
	if el := time.Since(start); el > 250*time.Millisecond {
		t.Errorf("diagnostics on one long line took %v", el)
	}
}

func TestRangedChangeIgnored(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(false, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\n")
	c.notify("textDocument/didChange", didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []contentChange{{Range: &Range{}, Text: "HTTP 2x"}},
	})
	var edits []TextEdit
	c.result("textDocument/formatting", formattingParams{TextDocument: textDocumentIdentifier{URI: uri}}, &edits)
	if len(edits) != 0 {
		t.Errorf("ranged change replaced the text: %+v", edits)
	}
}

func TestCaptureVisibleToEarlierAsserts(t *testing.T) {
	src := "GET http://a/\nHTTP 200\n[Asserts]\nbody == \"{{id}}\"\n[Captures]\nid: body\n"
	f, errs := syntax.ParseAll("a.hurl", []byte(src), syntax.DialectHurl, 50)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	defs := definitions(f)
	use := strings.Index(src, "id}}")
	if _, ok := visibleAt(defs, use)["id"]; !ok {
		t.Error("capture not visible to an assert written before [Captures]")
	}
	if _, ok := visibleAt(defs, strings.Index(src, "http"))["id"]; ok {
		t.Error("capture visible in its own request")
	}
}
