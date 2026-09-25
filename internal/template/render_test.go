// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// urlTemplate parses `GET <url>` and returns the URL template.
func urlTemplate(t *testing.T, url string) *syntax.Template {
	t.Helper()
	f, err := syntax.Parse("t.hurl", []byte("GET "+url+"\n"), syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	return f.Entries[0].Request.URL
}

func testEnv() *Env {
	vars := Vars{}
	vars.Set("host", value.String("example.org"))
	vars.Set("n", value.Int(3))
	vars.Set("f", value.Float(2))
	vars.Set("b", value.Bool(true))
	vars.Set("z", value.Null{})
	vars.Set("list", value.List{value.Int(1)})
	vars.Set("bytes", value.Bytes{1})
	vars.SetSecret("token", "s3cret")
	return &Env{
		Vars: vars,
		Now:  func() time.Time { return time.Date(2026, 9, 26, 1, 2, 3, 456_789_000, time.FixedZone("x", 3600)) },
		UUID: func() string { return "00000000-0000-4000-8000-000000000000" },
	}
}

func TestRender(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://{{host}}/a", "http://example.org/a"},
		{"http://a/{{n}}/{{f}}/{{b}}/{{z}}", "http://a/3/2.0/true/null"},
		{"http://a/{{ token }}", "http://a/s3cret"},
		{"http://a/{{newDate}}", "http://a/2026-09-26T00:02:03.456789Z"},
		{"http://a/{{newUuid}}", "http://a/00000000-0000-4000-8000-000000000000"},
		{"http://a/b", "http://a/b"},
		{`http://a/\u{41}`, "http://a/A"},
	}
	env := testEnv()
	for _, tt := range tests {
		got, err := env.Render(urlTemplate(t, tt.in))
		if err != nil || got != tt.want {
			t.Errorf("Render(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

// Text after the expression inside a placeholder is ignored.
func TestRenderIgnoresPlaceholderTrailingText(t *testing.T) {
	f, err := syntax.Parse("t.hurl", []byte("GET http://a\nHTTP 200\n[Asserts]\nheader \"{{host b}}\" exists\n"), syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := f.Entries[0].Response.Sections[0].Asserts[0].Query.Arg.(*syntax.Template)
	got, err := testEnv().Render(tmpl)
	if err != nil || got != "example.org" {
		t.Errorf("Render = %q, %v", got, err)
	}
}

func TestRenderErrors(t *testing.T) {
	env := testEnv()
	tests := []struct {
		in, kind, msg string
		col           int
	}{
		{"http://{{missing}}", "Undefined variable", "you must set the variable missing", 14},
		{"http://a/{{list}}", "Unrenderable expression", "expression with value [1] can not be rendered", 16},
		{"http://a/{{bytes}}", "Unrenderable expression", "expression with value 01 can not be rendered", 16},
	}
	for _, tt := range tests {
		_, err := env.Render(urlTemplate(t, tt.in))
		var re *runerr.Error
		if !errors.As(err, &re) {
			t.Fatalf("%s: error %v", tt.in, err)
		}
		if re.Description() != tt.kind || re.Message() != tt.msg || re.Span.Start.Col != tt.col || re.Assert {
			t.Errorf("%s: %s / %s at col %d", tt.in, re.Description(), re.Message(), re.Span.Start.Col)
		}
	}
}

func TestDefaultFunctions(t *testing.T) {
	env := &Env{Vars: Vars{}}
	v, err := env.Eval(syntax.Expr{Kind: syntax.ExprFunction, Name: "newUuid"})
	if err != nil {
		t.Fatal(err)
	}
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if s, ok := v.(value.String); !ok || !uuid.MatchString(string(s)) {
		t.Errorf("newUuid = %#v", v)
	}
	if v2, _ := env.Eval(syntax.Expr{Kind: syntax.ExprFunction, Name: "newUuid"}); v2 == v {
		t.Error("newUuid repeated a value")
	}
	before := time.Now().UTC()
	v, _ = env.Eval(syntax.Expr{Kind: syntax.ExprFunction, Name: "newDate"})
	d, ok := v.(value.Date)
	if !ok || d.UTC().Before(before.Add(-time.Second)) || time.Time(d).Location() != time.UTC {
		t.Errorf("newDate = %#v", v)
	}
}

func TestVars(t *testing.T) {
	vars := Vars{}
	vars.SetSecret("s", "x")
	if v, ok := vars.Get("s"); !ok || v != value.String("x") || !vars["s"].Secret {
		t.Errorf("secret = %#v", vars["s"])
	}
	vars.Set("s", value.Int(1))
	if vars["s"].Secret {
		t.Error("Set kept the secret flag")
	}
	if _, ok := vars.Get("nope"); ok {
		t.Error("Get(nope) found")
	}
}

// requestBody parses a request with the given body and returns it.
func requestBody(t *testing.T, body string) syntax.Bytes {
	t.Helper()
	f, err := syntax.Parse("t.hurl", []byte("POST http://a\n"+body+"\n"), syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	return f.Entries[0].Request.Body.Value
}

func TestRenderJSON(t *testing.T) {
	env := testEnv()
	env.Vars.Set("q", value.String("a\"b\\c\nd\te\r"))
	tests := []struct{ body, keep, compact string }{
		{`{ "a" : [ 1 , {{n}}, "x{{q}}" ] , "b": {"c": null, "d": true, "e": {{f}} } }`,
			`{ "a" : [ 1 , 3, "xa\"b\\c\nd\te\r" ] , "b": {"c": null, "d": true, "e": 2.0 } }`,
			`{"a":[1,3,"xa\"b\\c\nd\te\r"],"b":{"c":null,"d":true,"e":2.0}}`},
		{`[ ]`, `[ ]`, `[]`},
		{`{"{{host}}": "\u00e9"}`, `{"example.org": "\u00e9"}`, `{"{{host}}":"\u00e9"}`},
	}
	for _, tt := range tests {
		v := requestBody(t, tt.body).(syntax.JSONValue)
		for keep, want := range map[bool]string{true: tt.keep, false: tt.compact} {
			got, err := env.RenderJSON(v, keep)
			if err != nil || got != want {
				t.Errorf("RenderJSON(%s, %v) = %s, %v; want %s", tt.body, keep, got, err, want)
			}
		}
	}
	for body, msg := range map[string]string{
		`[{{host}}]`:           "Invalid JSON: actual value is <example.org>",
		`[{{missing}}]`:        "Undefined variable: you must set the variable missing",
		`["{{missing}}"]`:      "Undefined variable: you must set the variable missing",
		`{"{{missing}}": 1}`:   "Undefined variable: you must set the variable missing",
		`{"a": [{{missing}}]}`: "Undefined variable: you must set the variable missing",
	} {
		if _, err := env.RenderJSON(requestBody(t, body).(syntax.JSONValue), true); err == nil || err.Error() != msg {
			t.Errorf("%s: %v", body, err)
		}
	}
	for _, s := range []string{"-1", "1abc", "true", "falsey", "null"} {
		if !isJSONScalarPrefix(s) {
			t.Errorf("isJSONScalarPrefix(%q) = false", s)
		}
	}
	for _, s := range []string{"", "-", "-x", ".5", "nul", "x"} {
		if isJSONScalarPrefix(s) {
			t.Errorf("isJSONScalarPrefix(%q) = true", s)
		}
	}
}

func TestRenderMultiline(t *testing.T) {
	env := testEnv()
	tests := []struct{ body, want string }{
		{"```\nhost {{host}}\n```", "host example.org\n"},
		{"```json\n{\"n\": {{n}}}\n```", "{\"n\": 3}\n"},
		{"```xml\n<a>{{n}}</a>\n```", "<a>3</a>\n"},
		{"```raw\n{{host}} \\n\n```", "{{host}} \\n\n"},
		{"```graphql\nquery { a(x: \"{{host}}\") }\n```", `{"query":"query { a(x: \"example.org\") }"}`},
		{"```graphql\n{ a }\n\nvariables {\"n\": {{n}}, \"s\": \"x\"}\n```", `{"query":"{ a }","variables":{"n":3,"s":"x"}}`},
	}
	for _, tt := range tests {
		got, err := env.RenderMultiline(requestBody(t, tt.body).(*syntax.MultilineString))
		if err != nil || got != tt.want {
			t.Errorf("%s:\n%q, %v\nwant %q", tt.body, got, err, tt.want)
		}
	}
	for _, body := range []string{"```graphql\n{{missing}}\n```", "```graphql\n{ a }\nvariables {\"n\": {{missing}}}\n```"} {
		if _, err := env.RenderMultiline(requestBody(t, body).(*syntax.MultilineString)); err == nil {
			t.Errorf("%s: no error", body)
		}
	}
	if got := jsonString("\x01\b\f\n\r\t\"\\é"); got != `"\u0001\b\f\n\r\t\"\\é"` {
		t.Errorf("jsonString = %s", got)
	}
	tmpl := &syntax.Template{Elements: []syntax.TemplateElement{
		&syntax.TemplateString{Value: "a", Source: `\u{61}`},
		&syntax.Placeholder{Space0: syntax.Whitespace{Value: " "}, Expr: syntax.Expr{Name: "x"}, Trailing: " y"},
	}}
	if got := templateSource(tmpl); got != `\u{61}{{ x y}}` {
		t.Errorf("templateSource = %q", got)
	}
}

func TestRegexArg(t *testing.T) {
	env := testEnv()
	span := syntax.Span{Start: syntax.Pos{Line: 1, Col: 2}}
	for _, n := range []syntax.Node{
		&syntax.Regex{Pattern: `\d+`},
		&syntax.Template{Elements: []syntax.TemplateElement{&syntax.TemplateString{Value: `\d+`}}},
	} {
		re, err := env.Regex(n, span)
		if err != nil || !re.Re.MatchString("١") {
			t.Errorf("Regex(%T) = %v, %v", n, re, err)
		}
	}
	for _, n := range []syntax.Node{
		&syntax.Regex{Pattern: `(`},
		&syntax.Template{Elements: []syntax.TemplateElement{&syntax.TemplateString{Value: `(`}}},
		&syntax.Template{Elements: []syntax.TemplateElement{&syntax.Placeholder{Expr: syntax.Expr{Name: "missing"}}}},
		&syntax.Null{},
	} {
		if _, err := env.Regex(n, span); err == nil {
			t.Errorf("Regex(%T) succeeded", n)
		}
	}
}

func TestFile(t *testing.T) {
	name := urlTemplate(t, "data.bin")
	env := testEnv()
	env.ReadFile = func(n string) ([]byte, error) {
		switch n {
		case "data.bin":
			return []byte{1}, nil
		case "x":
			return nil, errors.New("boom")
		}
		return nil, ErrFileAccessDenied
	}
	if b, err := env.File(name); err != nil || len(b) != 1 {
		t.Errorf("File = %v, %v", b, err)
	}
	for url, msg := range map[string]string{
		"x":           "File read access: file x can not be read",
		"../y":        "Unauthorized file access: unauthorized access to file ../y, check --file-root option",
		"{{missing}}": "Undefined variable: you must set the variable missing",
	} {
		if _, err := env.File(urlTemplate(t, url)); err == nil || err.Error() != msg {
			t.Errorf("File(%s) = %v", url, err)
		}
	}
	env.ReadFile = nil
	if _, err := env.File(name); err == nil || err.Error() != "File read access: file data.bin can not be read" {
		t.Errorf("no reader: %v", err)
	}
}
