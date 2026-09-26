// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/config"
)

func TestHoverQueryFilterPredicate(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	src := "GET http://a/\nHTTP 200\n[Asserts]\nheader \"X-Id\" count not == 200\n"
	c.open(uri, src)

	// "header" query keyword.
	h := c.hover(uri, Position{3, 1})
	if h == nil || !strings.Contains(h.Contents.Value, "response header") {
		t.Fatalf("header hover = %+v", h)
	}
	if h.Range == nil || h.Range.Start != (Position{3, 0}) {
		t.Errorf("header range = %+v", h.Range)
	}

	// "count" filter keyword.
	h = c.hover(uri, Position{3, uint32(len(`header "X-Id" co`))})
	if h == nil || !strings.Contains(h.Contents.Value, "number of elements") {
		t.Fatalf("count hover = %+v", h)
	}

	// "not".
	h = c.hover(uri, Position{3, uint32(len(`header "X-Id" count n`))})
	if h == nil || !strings.Contains(h.Contents.Value, "Negates") {
		t.Fatalf("not hover = %+v", h)
	}

	// "==" predicate.
	h = c.hover(uri, Position{3, uint32(len(`header "X-Id" count not =`))})
	if h == nil || !strings.Contains(h.Contents.Value, "equals") {
		t.Fatalf("== hover = %+v", h)
	}

	// The whole query token, including its argument, hovers as one unit.
	h = c.hover(uri, Position{3, uint32(len(`header "X-`))})
	if h == nil || !strings.Contains(h.Contents.Value, "response header") {
		t.Errorf("hover over the argument = %+v", h)
	}

	// Nothing on a blank line between entries.
	if h := c.hover(uri, Position{4, 0}); h != nil {
		t.Errorf("hover on a blank line: %+v", h)
	}
}

func TestHoverOption(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\n[Options]\ndigest: true\n")
	h := c.hover(uri, Position{2, 2})
	if h == nil || !strings.Contains(h.Contents.Value, "does not support this yet") {
		t.Fatalf("digest hover = %+v", h)
	}
}

func TestHoverSectionHeader(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\nHTTP 200\n[Asserts]\nstatus == 200\n")
	h := c.hover(uri, Position{2, 3})
	if h == nil || !strings.Contains(h.Contents.Value, "Assertions") {
		t.Fatalf("[Asserts] hover = %+v", h)
	}
	if h.Range == nil || *h.Range != (Range{Start: Position{2, 0}, End: Position{2, 9}}) {
		t.Errorf("range = %+v", h.Range)
	}
}

// posAt returns the utf-8 position of offset in src, avoiding manual line/
// column arithmetic in the tests below.
func posAt(src string, offset int) Position {
	return newLineIndex(src, false).position(offset)
}

func TestHoverRedact(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	src := "GET http://a/\nHTTP 200\n[Captures]\ntoken: jsonpath \"$.token\" redact\n"
	c.open(uri, src)
	h := c.hover(uri, posAt(src, strings.Index(src, "redact")))
	if h == nil || !strings.Contains(h.Contents.Value, "secret") {
		t.Fatalf("redact hover = %+v", h)
	}
}

func TestHoverVariable(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	src := "GET http://a/\n" +
		"HTTP 200\n" +
		"[Captures]\n" +
		"id: jsonpath \"$.id\"\n" +
		"\n" +
		"GET http://b/{{id}}\n" +
		"[Options]\n" +
		"variable: token=supersecretvalue\n" +
		"\n" +
		"GET http://c/{{token}}{{missing}}{{newUuid}}\n"
	c.open(uri, src)

	idPos := posAt(src, strings.Index(src, "{{id}}")+2)
	h := c.hover(uri, idPos)
	if h == nil || !strings.Contains(h.Contents.Value, "Capture at line 4") {
		t.Fatalf("{{id}} hover = %+v", h)
	}

	tokenPos := posAt(src, strings.Index(src, "{{token}}")+2)
	h = c.hover(uri, tokenPos)
	if h == nil || !strings.Contains(h.Contents.Value, "[Options] variable at line 8") {
		t.Fatalf("{{token}} hover = %+v", h)
	}

	h = c.hover(uri, posAt(src, strings.Index(src, "{{missing}}")+2))
	if h == nil || !strings.Contains(h.Contents.Value, "undefined") {
		t.Fatalf("{{missing}} hover = %+v", h)
	}

	h = c.hover(uri, posAt(src, strings.Index(src, "{{newUuid}}")+2))
	if h == nil || !strings.Contains(h.Contents.Value, "UUID") {
		t.Fatalf("{{newUuid}} hover = %+v", h)
	}

	b, err := json.Marshal([]*Hover{c.hover(uri, idPos), c.hover(uri, tokenPos)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "supersecretvalue") {
		t.Error("a variable value leaked into hover")
	}
}

func TestHoverImporterComments(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	src := "# prerequest script (never executed):\n" +
		"# console.log(1)\n" +
		"# opencollection assertion: res.status eq 200\n" +
		"# just a note\n" +
		"GET http://a/\n"
	c.open(uri, src)

	h := c.hover(uri, Position{0, 3})
	if h == nil || !strings.Contains(h.Contents.Value, "does not run pre-request or test scripts") {
		t.Fatalf("prerequest comment hover = %+v", h)
	}
	h = c.hover(uri, Position{2, 3})
	if h == nil || !strings.Contains(h.Contents.Value, "could not map this OpenCollection assertion") {
		t.Fatalf("opencollection assertion hover = %+v", h)
	}
	if h := c.hover(uri, Position{3, 3}); h != nil {
		t.Errorf("plain comment should not hover: %+v", h)
	}
}

func TestProcessEnvironmentSecret(t *testing.T) {
	c := newTestClient(t, config.Env{"SONDE_SECRET_tok": "supersecretvalue"})
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	src := "GET http://a/{{tok}}\n"
	c.open(uri, src)

	h := c.hover(uri, posAt(src, strings.Index(src, "{{tok}}")+2))
	if h == nil || !strings.Contains(h.Contents.Value, "Process environment secret (value hidden)") {
		t.Fatalf("{{tok}} hover = %+v", h)
	}

	items := c.completion(uri, posAt(src, strings.Index(src, "{{tok}}")+4))
	tok := item(t, items, "tok")
	if !strings.Contains(tok.Detail, "Process environment secret (value hidden)") {
		t.Errorf("tok completion detail = %q", tok.Detail)
	}

	b, err := json.Marshal(struct {
		Hover *Hover
		Items []CompletionItem
	}{h, items})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "supersecretvalue") {
		t.Error("a SONDE_SECRET_* value leaked into hover or completion")
	}
}

func TestHoverPlaintextNoBackticks(t *testing.T) {
	c := newTestClient(t, nil)
	params := map[string]any{
		"capabilities": map[string]any{
			"textDocument": map[string]any{"hover": map[string]any{"contentFormat": []string{"plaintext"}}},
		},
	}
	var res initializeResult
	c.result("initialize", params, &res)
	c.notify("initialized", map[string]any{})
	if res.Capabilities.HoverProvider != true {
		t.Fatalf("initialize result: %+v", res)
	}

	uri := "file:///w/a.hurl"
	src := "GET http://a/\nHTTP 200\n[Captures]\ntoken: jsonpath \"$.token\" redact\n\nGET http://b/{{token}}{{missing}}\n"
	c.open(uri, src)

	h := c.hover(uri, posAt(src, strings.Index(src, "{{token}}")+2))
	if h == nil || h.Contents.Kind != markupPlainText || strings.Contains(h.Contents.Value, "`") {
		t.Fatalf("redact hover for a plaintext client = %+v", h)
	}
	h = c.hover(uri, posAt(src, strings.Index(src, "{{missing}}")+2))
	if h == nil || h.Contents.Kind != markupPlainText || strings.Contains(h.Contents.Value, "`") {
		t.Fatalf("undefined-variable hover for a plaintext client = %+v", h)
	}
}

func TestHoverAstral(t *testing.T) {
	src := "GET http://a/😀/\nHTTP 200\n[Asserts]\nstatus == 200\n"
	for _, utf8 := range []bool{true, false} {
		c := newTestClient(t, nil)
		c.initialize(utf8, "", nil)
		uri := "file:///w/a.hurl"
		c.open(uri, src)
		pos := newLineIndex(src, !utf8).position(strings.Index(src, "status") + 2)
		h := c.hover(uri, pos)
		if h == nil || !strings.Contains(h.Contents.Value, "status code") {
			t.Fatalf("utf8=%v: status hover = %+v", utf8, h)
		}
	}
}
