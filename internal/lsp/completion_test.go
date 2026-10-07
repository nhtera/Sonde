// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// item returns the completion item labeled label, or fails the test.
func item(t *testing.T, items []CompletionItem, label string) CompletionItem {
	t.Helper()
	for _, it := range items {
		if it.Label == label {
			return it
		}
	}
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.Label
	}
	t.Fatalf("no item %q among %v", label, labels)
	return CompletionItem{}
}

func hasLabel(items []CompletionItem, label string) bool {
	return slices.ContainsFunc(items, func(it CompletionItem) bool { return it.Label == label })
}

// TestCompletionNeverNull decodes the raw JSON of a completion reply for a
// context with nothing to propose (a header's value: free text, not a
// query/filter/predicate/option position). LSP requires
// CompletionList.items to be an array, never null.
func TestCompletionNeverNull(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\nContent-Type: application/json\n")
	pos := uint32(len("Content-Type: appl"))
	m := c.call("textDocument/completion", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri}, Position: Position{1, pos},
	})
	if m.Error != nil {
		t.Fatalf("completion: %v", m.Error)
	}
	var decoded struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(m.Result, &decoded); err != nil {
		t.Fatalf("decoding %s: %v", m.Result, err)
	}
	if string(decoded.Items) != "[]" {
		t.Errorf(`raw "items" = %s, want "[]" (LSP forbids null)`, decoded.Items)
	}
}

func TestCompletionMethods(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "")
	items := c.completion(uri, Position{0, 0})
	for _, m := range []string{"GET", "POST", "DELETE"} {
		got := item(t, items, m)
		if got.Kind != kindMethod || got.TextEdit.NewText != m {
			t.Errorf("%s: %+v", m, got)
		}
	}
	if hasLabel(items, "Content-Type") || hasLabel(items, "Query") {
		t.Error("no header/section proposals before any method is typed")
	}
}

func TestCompletionHeaderAndHTTPLine(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\n")
	items := c.completion(uri, Position{1, 0})
	ct := item(t, items, "Content-Type")
	if ct.TextEdit.NewText != "Content-Type: " {
		t.Errorf("Content-Type insert = %q", ct.TextEdit.NewText)
	}
	h := item(t, items, "HTTP")
	if h.TextEdit.NewText == "" {
		t.Errorf("HTTP item: %+v", h)
	}
	if hasLabel(items, "GET") {
		t.Error("no more methods once the request line exists")
	}
}

func TestCompletionHeaderAfterBOM(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "\xEF\xBB\xBFGET http://a/\n")
	items := c.completion(uri, Position{1, 0})
	item(t, items, "Content-Type")
	item(t, items, "HTTP")
	if hasLabel(items, "GET") {
		t.Error("a BOM before the request line should not hide it from the line-start heuristic")
	}
}

func TestCompletionHeaderAfterStatusLine(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\nHTTP 200\n")
	items := c.completion(uri, Position{2, 0})
	item(t, items, "Set-Cookie")
	if hasLabel(items, "HTTP") {
		t.Error("no second HTTP proposal once the response has started")
	}
}

func TestCompletionSectionNames(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"

	c.open(uri, "GET http://a/\n[")
	req := c.completion(uri, Position{1, 1})
	for _, n := range requestSectionNames {
		item(t, req, n)
	}
	if hasLabel(req, "Captures") || hasLabel(req, "Asserts") {
		t.Error("no response sections in the request part")
	}
	q := item(t, req, "Query")
	if q.TextEdit.NewText != "Query]" {
		t.Errorf("Query insert = %q, want closing bracket added", q.TextEdit.NewText)
	}

	c.open(uri, "GET http://a/\nHTTP 200\n[")
	resp := c.completion(uri, Position{2, 1})
	for _, n := range responseSectionNames {
		item(t, resp, n)
	}
	if hasLabel(resp, "Query") || hasLabel(resp, "Options") {
		t.Error("no request sections in the response part")
	}
}

func TestCompletionSectionNameKeepsExistingBracket(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\n[]")
	items := c.completion(uri, Position{1, 1})
	q := item(t, items, "Query")
	if q.TextEdit.NewText != "Query" {
		t.Errorf("insert = %q, want no extra bracket", q.TextEdit.NewText)
	}
}

func TestCompletionAssertQueryThenFilterPredicate(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"

	c.open(uri, "GET http://a/\nHTTP 200\n[Asserts]\n")
	atQuery := c.completion(uri, Position{3, 0})
	item(t, atQuery, "status")
	item(t, atQuery, "jsonpath")
	if hasLabel(atQuery, "contains") || hasLabel(atQuery, "not") {
		t.Error("only queries at the start of an assert line")
	}

	c.open(uri, "GET http://a/\nHTTP 200\n[Asserts]\nstatus ")
	atRest := c.completion(uri, Position{3, uint32(len("status "))})
	item(t, atRest, "contains")
	item(t, atRest, "not")
	pred := item(t, atRest, "==")
	if pred.Kind != kindOperator {
		t.Errorf("== kind = %d", pred.Kind)
	}
	if hasLabel(atRest, "redact") {
		t.Error("no redact in [Asserts]")
	}
	if hasLabel(atRest, "status") {
		t.Error("no query proposals once the query is done")
	}
}

func TestCompletionAssertNoDoubleNot(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\nHTTP 200\n[Asserts]\nstatus not ")
	items := c.completion(uri, Position{3, uint32(len("status not "))})
	if hasLabel(items, "not") {
		t.Error("no second not right after one")
	}
	item(t, items, "==")
}

func TestCompletionCapture(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"

	c.open(uri, "GET http://a/\nHTTP 200\n[Captures]\nid")
	noColon := c.completion(uri, Position{3, 2})
	if len(noColon) != 0 {
		t.Errorf("no completion while typing the capture name: %+v", noColon)
	}

	c.open(uri, "GET http://a/\nHTTP 200\n[Captures]\nid: ")
	atQuery := c.completion(uri, Position{3, uint32(len("id: "))})
	item(t, atQuery, "jsonpath")
	if hasLabel(atQuery, "redact") {
		t.Error("no redact right at the query position")
	}

	c.open(uri, "GET http://a/\nHTTP 200\n[Captures]\nid: header \"X-Id\" ")
	atRest := c.completion(uri, Position{3, uint32(len(`id: header "X-Id" `))})
	item(t, atRest, "redact")
	item(t, atRest, "count")
	if hasLabel(atRest, "not") || hasLabel(atRest, "==") {
		t.Error("no predicates in [Captures]")
	}
}

func TestCompletionOptionNameAndValue(t *testing.T) {
	c := newTestClientTable(t, nil, unsupportedTable(t))
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"

	c.open(uri, "GET http://a/\n[Options]\n")
	names := c.completion(uri, Position{2, 0})
	insecure := item(t, names, "insecure")
	if insecure.TextEdit.NewText != "insecure: " {
		t.Errorf("insecure insert = %q", insecure.TextEdit.NewText)
	}
	digest := item(t, names, "http3")
	if !strings.Contains(digest.Detail, "not supported yet") {
		t.Errorf("http3 detail = %q", digest.Detail)
	}

	c.open(uri, "GET http://a/\n[Options]\ninsecure: ")
	bools := c.completion(uri, Position{2, uint32(len("insecure: "))})
	item(t, bools, "true")
	item(t, bools, "false")

	c.open(uri, "GET http://a/\n[Options]\ndelay: ")
	durations := c.completion(uri, Position{2, uint32(len("delay: "))})
	if len(durations) == 0 {
		t.Error("no duration hints for delay")
	}
	for _, d := range durations {
		if !strings.HasSuffix(d.Label, "ms") && !strings.HasSuffix(d.Label, "s") && !strings.HasSuffix(d.Label, "m") {
			t.Errorf("duration hint %q looks wrong", d.Label)
		}
	}

	c.open(uri, "GET http://a/\n[Options]\nverbosity: ")
	verbosity := c.completion(uri, Position{2, uint32(len("verbosity: "))})
	item(t, verbosity, "brief")
	item(t, verbosity, "debug")
}

func TestCompletionVariableAndFunctions(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	src := "GET http://a/\n" +
		"HTTP 200\n" +
		"[Captures]\n" +
		"id: jsonpath \"$.id\"\n" +
		"secret: jsonpath \"$.token\" redact\n" +
		"\n" +
		"GET http://b/{{id}}?t={{"
	c.open(uri, src)
	pos := newLineIndex(src, false).position(len(src))
	items := c.completion(uri, pos)

	id := item(t, items, "id")
	if id.Kind != kindVariable || !strings.Contains(id.Detail, "Capture at line 4") {
		t.Errorf("id = %+v", id)
	}
	secret := item(t, items, "secret")
	if !strings.Contains(secret.Detail, "value hidden") {
		t.Errorf("secret detail = %q, want value hidden", secret.Detail)
	}
	item(t, items, "newUuid")
	item(t, items, "newDate")

	b, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "supersecretvalue") {
		t.Error("a variable value leaked into completion")
	}
}

func TestCompletionDeprecatedSortsLast(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	uri := "file:///w/a.hurl"
	c.open(uri, "GET http://a/\nHTTP 200\n[Asserts]\nstatus ")
	items := c.completion(uri, Position{3, uint32(len("status "))})
	includes := item(t, items, "includes")
	if !slices.Contains(includes.Tags, tagDeprecated) {
		t.Errorf("includes tags = %v", includes.Tags)
	}
	contains := item(t, items, "contains")
	if includes.SortText <= contains.SortText {
		t.Errorf("deprecated sort text %q should sort after %q", includes.SortText, contains.SortText)
	}
}

func TestCompletionVariableAstral(t *testing.T) {
	src := "GET http://a/😀/\nHTTP 200\n[Captures]\nid: jsonpath \"$.id\"\n\nGET http://b/\nX: {{"
	for _, utf8 := range []bool{true, false} {
		c := newTestClient(t, nil)
		c.initialize(utf8, "", nil)
		uri := "file:///w/a.hurl"
		c.open(uri, src)
		pos := newLineIndex(src, !utf8).position(len(src))
		items := c.completion(uri, pos)
		item(t, items, "id")
	}
}
