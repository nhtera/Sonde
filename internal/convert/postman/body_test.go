// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

func TestParseJSONLoose(t *testing.T) {
	if _, ok := parseJSONLoose(`{"a": 1}`); !ok {
		t.Error("valid JSON should parse")
	}
	if _, ok := parseJSONLoose(`{"a": {{n}}}`); ok {
		t.Error("a placeholder in a non-string position is not valid JSON")
	}
	if _, ok := parseJSONLoose(`{"a": 1} garbage`); ok {
		t.Error("trailing garbage should not parse")
	}
}

func TestGraphQLVariablesText(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`"{\"id\": 1}"`, `{"id": 1}`},
		{`"{}"`, ""},
		{`""`, ""},
		{`null`, ""},
		{`{"id": 1}`, `{"id":1}`},
	} {
		if got := graphQLVariablesText([]byte(tc.raw)); got != tc.want {
			t.Errorf("graphQLVariablesText(%s) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestGraphQLBody checks a query keeps its {{variable}} placeholders
// working, a JSON-object "variables" string becomes a real variables
// object, and a non-object "variables" value is dropped with a warning
// instead of guessed at.
func TestGraphQLBody(t *testing.T) {
	col := `{"info":{"name":"x"},"item":[
		{"name":"with vars","request":{"method":"POST","url":"http://x","body":{"mode":"graphql",
			"graphql":{"query":"query Q($id: ID!) { widget(id: $id) }","variables":"{\"id\": \"{{id}}\"}"}}}},
		{"name":"bad vars","request":{"method":"POST","url":"http://x","body":{"mode":"graphql",
			"graphql":{"query":"query Q { widgets }","variables":"[1,2,3]"}}}}
	]}`
	out, err := Import([]byte(col), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(out.Files))
	}
	withVars := string(syntax.Format(out.Files[0].File))
	for _, want := range []string{"```graphql", `variables {"id": "{{id}}"}`} {
		if !strings.Contains(withVars, want) {
			t.Errorf("output missing %q:\n%s", want, withVars)
		}
	}
	badVars := string(syntax.Format(out.Files[1].File))
	if strings.Contains(badVars, "variables") {
		t.Errorf("a non-object variables value should be dropped:\n%s", badVars)
	}
	found := false
	for _, w := range out.Warnings {
		if w.Kind == "unsupported-body" && strings.Contains(w.Message, "not a JSON object") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning for the non-object variables value: %v", out.Warnings)
	}
}

// TestRawJSONBodyFallback checks that a raw JSON body invalid because of a
// placeholder in a non-string position falls back to templated text (with
// a warning that it is no longer structured JSON) instead of failing the
// whole request — but the placeholder itself, unlike a plain RawTextBody
// fallback, still substitutes, exactly like the placeholder inside a
// string value that round-trips through JSONBody as a working variable.
func TestRawJSONBodyFallback(t *testing.T) {
	col := `{"info":{"name":"x"},"item":[
		{"name":"bad","request":{"method":"POST","url":"http://x","body":{"mode":"raw","raw":"{\"n\": {{n}}}","options":{"raw":{"language":"json"}}}}},
		{"name":"good","request":{"method":"POST","url":"http://x","body":{"mode":"raw","raw":"{\"n\": \"{{n}}\"}","options":{"raw":{"language":"json"}}}}}
	]}`
	out, err := Import([]byte(col), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(out.Files))
	}
	// The invalid JSON falls back to a templated body: {{n}} keeps working
	// as a real variable even though the surrounding text is no longer
	// rebuilt as structured JSON.
	bad := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(bad, `"n": {{n}}`) {
		t.Errorf("bad body should keep {{n}} as a working placeholder:\n%s", bad)
	}
	found := false
	for _, w := range out.Warnings {
		if w.Kind == "unsupported-body" {
			found = true
		}
	}
	if !found {
		t.Error("expected an unsupported-body warning for the invalid JSON fallback")
	}
	good := string(syntax.Format(out.Files[1].File))
	if !strings.Contains(good, `"n": "{{n}}"`) {
		t.Errorf("good body should keep the working {{n}} placeholder:\n%s", good)
	}
}

// TestRawBodyPlaceholdersStaySubstituted checks that xml, text, html and
// javascript raw bodies keep a {{variable}} live (via TextBody), unlike a
// plain RawTextBody, which would make it literal.
func TestRawBodyPlaceholdersStaySubstituted(t *testing.T) {
	for _, tc := range []struct {
		lang, raw, wantFence, wantCT string
	}{
		{"xml", "<a>{{id}}</a>", "```xml", "application/xml"},
		{"text", "hello {{name}}", "```\n", "text/plain"},
		{"html", "<p>{{name}}</p>", "```\n", "text/html"},
		{"javascript", "var x = {{name}};", "```\n", "application/javascript"},
	} {
		col := fmt.Sprintf(`{"info":{"name":"x"},"item":[{"name":"r","request":{"method":"POST","url":"http://x",
			"body":{"mode":"raw","raw":%s,"options":{"raw":{"language":%q}}}}}]}`, jsonString(t, tc.raw), tc.lang)
		out, err := Import([]byte(col), Options{Dialect: syntax.DialectHurl})
		if err != nil {
			t.Fatalf("%s: %v", tc.lang, err)
		}
		src := string(syntax.Format(out.Files[0].File))
		if !strings.Contains(src, tc.wantFence) {
			t.Errorf("%s: output missing fence %q:\n%s", tc.lang, tc.wantFence, src)
		}
		if !strings.Contains(src, "Content-Type: "+tc.wantCT) {
			t.Errorf("%s: output missing inferred Content-Type %q:\n%s", tc.lang, tc.wantCT, src)
		}
		if !strings.Contains(src, tc.raw) {
			t.Errorf("%s: placeholder should stay live, exactly as written:\n%s", tc.lang, src)
		}
	}
}

// TestFormDataMultipleFilesWarns checks that a formdata file field with
// several "src" entries keeps only the first, with a warning, rather than
// silently dropping the rest.
func TestFormDataMultipleFilesWarns(t *testing.T) {
	col := `{"info":{"name":"x"},"item":[
		{"name":"r","request":{"method":"POST","url":"http://x","body":{"mode":"formdata",
			"formdata":[{"key":"files","type":"file","src":["a.png","b.png"]}]}}}
	]}`
	out, err := Import([]byte(col), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	src := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(src, "file,a.png;") || strings.Contains(src, "b.png") {
		t.Errorf("only the first file should be kept:\n%s", src)
	}
	found := false
	for _, w := range out.Warnings {
		if w.Kind == "unsupported-body" && strings.Contains(w.Message, "2 files") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning naming the dropped files: %v", out.Warnings)
	}
}

// TestTolerantShapes exercises the tolerant parsing this package is meant
// for: a url written as an object without "raw", a header value written as
// a number, and a description written as an object.
func TestTolerantShapes(t *testing.T) {
	col := `{"info":{"name":"x"},"item":[
		{"name":"obj-url","description":{"content":"desc line","type":"text/plain"},
		 "request":{"method":"GET",
		   "url":{"protocol":"https","host":["api","example","test"],"path":["v1","widgets"],"query":[{"key":"limit","value":"10"}]},
		   "header":[{"key":"X-Count","value":5}]}}
	]}`
	out, err := Import([]byte(col), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(out.Files))
	}
	src := string(syntax.Format(out.Files[0].File))
	for _, want := range []string{"desc line", "https://api.example.test/v1/widgets?limit=10", "X-Count: 5"} {
		if !strings.Contains(src, want) {
			t.Errorf("output missing %q:\n%s", want, src)
		}
	}
}

func TestRejectV1Collection(t *testing.T) {
	if _, err := Import(readFixture(t, "v1-legacy.json"), Options{}); err == nil {
		t.Fatal("expected a v1 collection to be rejected")
	} else if !strings.Contains(err.Error(), "v1") {
		t.Errorf("error should mention v1: %v", err)
	}
}

func TestBadGroupOption(t *testing.T) {
	if _, err := Import([]byte(`{"info":{"name":"x"},"item":[]}`), Options{Group: "bogus"}); err == nil {
		t.Fatal("expected an error for an invalid --group value")
	}
}

func TestNeverPanicsOnMalformedJSON(_ *testing.T) {
	for _, in := range []string{
		``, `null`, `{`, `[]`, `{"info":null}`, `{"info":{},"item":"not-an-array"}`,
		`{"info":{"name":"x"},"item":[{"request":{"url":123}}]}`,
		`{"info":{"name":"x"},"item":[{"request":{"body":{"mode":"graphql","graphql":123}}}]}`,
	} {
		if _, err := Import([]byte(in), Options{}); err != nil {
			// An error is fine; a panic is not.
			continue
		}
	}
}
