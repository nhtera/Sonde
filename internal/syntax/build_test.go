// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

// decodedLiteral concatenates the decoded value of every TemplateString
// element of t; it fails the test if t holds a placeholder, since the
// property test below only ever builds pure literals.
func decodedLiteral(t *testing.T, tpl *Template) string {
	t.Helper()
	var b strings.Builder
	for _, el := range tpl.Elements {
		s, ok := el.(*TemplateString)
		if !ok {
			t.Fatalf("unexpected placeholder in %#v", tpl)
		}
		b.WriteString(s.Value)
	}
	return b.String()
}

// fuzzLiterals are strings crafted to stress every escaping rule: '#', '\',
// braces, quotes, backtick, unicode, tabs, newlines, and leading/trailing
// spaces.
var fuzzLiterals = []string{
	"",
	" ",
	"  ",
	"plain",
	" leading",
	"trailing ",
	" both ",
	"a#b",
	"#comment",
	`a\b`,
	`\`,
	"a{{b",
	"{{",
	"}}",
	"a{{b}}c",
	"{{{{",
	"}}}}",
	`a"b`,
	`"quoted"`,
	"a`b`c",
	"tab\ttab",
	"new\nline",
	"cr\rreturn",
	"emoji \U0001F600 emoji",
	"unicode éè accents",
	"null\x00byte",
	"mixed #\\{{}} \"'` \t\n end  ",
	"   ",
}

func randLiteral(r *rand.Rand) string {
	if r.Intn(3) == 0 {
		return fuzzLiterals[r.Intn(len(fuzzLiterals))]
	}
	const alphabet = "ab#\\{}\"'`\t\n é\U0001F600:;[]@$_-."
	n := r.Intn(12)
	var b strings.Builder
	for range n {
		b.WriteRune([]rune(alphabet)[r.Intn(len([]rune(alphabet)))])
	}
	return b.String()
}

// TestBuildLiteralRoundTrip checks that arbitrary literal strings, placed in
// the URL, a header value, a query value and a JSON string body, decode back
// to the exact original value after Parse.
func TestBuildLiteralRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // G404: deterministic test fuzzing, not security-sensitive
	cases := append([]string{}, fuzzLiterals...)
	for range 200 {
		cases = append(cases, randLiteral(rng))
	}
	for i, lit := range cases {
		body, err := JSONBody(Members{{Key: "v", Value: PlainText(lit)}})
		if err != nil {
			t.Fatalf("case %d: JSONBody: %v", i, err)
		}
		entries := []EntrySpec{{
			Method: "GET",
			URL:    Text{Lit("http://example.com/"), Lit(lit)},
			Headers: []Field{
				{Key: PlainText("X-Lit"), Value: PlainText(lit)},
			},
			Query: []Field{
				{Key: PlainText("q"), Value: PlainText(lit)},
			},
			Body: body,
		}}
		f, err := BuildFile(entries, DialectHurl)
		if err != nil {
			t.Fatalf("case %d (%q): BuildFile: %v", i, lit, err)
		}
		req := f.Entries[0].Request

		gotURL := decodedLiteral(t, req.URL)
		wantURL := "http://example.com/" + lit
		if gotURL != wantURL {
			t.Errorf("case %d: URL decoded = %q, want %q", i, gotURL, wantURL)
		}

		if len(req.Headers) != 1 {
			t.Fatalf("case %d: got %d headers, want 1", i, len(req.Headers))
		}
		if got := decodedLiteral(t, req.Headers[0].Value); got != lit {
			t.Errorf("case %d: header decoded = %q, want %q", i, got, lit)
		}

		if len(req.Sections) != 1 || req.Sections[0].Kind != SectionQueryParams {
			t.Fatalf("case %d: want one Query section, got %#v", i, req.Sections)
		}
		if got := decodedLiteral(t, req.Sections[0].KeyValues[0].Value); got != lit {
			t.Errorf("case %d: query decoded = %q, want %q", i, got, lit)
		}

		jsonObj, ok := req.Body.Value.(*JSONObject)
		if !ok {
			t.Fatalf("case %d: body is %T, want *JSONObject", i, req.Body.Value)
		}
		gotJSON := decodedLiteral(t, jsonObj.Elements[0].Value.(*Template))
		if gotJSON != lit {
			t.Errorf("case %d: JSON string decoded = %q, want %q", i, gotJSON, lit)
		}

		// Format must be idempotent on whatever we just built.
		once := Format(f)
		f2, err := Parse("<fmt>", once, DialectHurl)
		if err != nil {
			t.Fatalf("case %d: Format output failed to parse: %v\n%s", i, err, once)
		}
		twice := Format(f2)
		if string(once) != string(twice) {
			t.Errorf("case %d: Format is not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", i, once, twice)
		}
	}
}

// TestBuildLiteralKey checks the same property for header/query keys, which
// use a much stricter grammar (no raw space or braces at all).
func TestBuildLiteralKey(t *testing.T) {
	rng := rand.New(rand.NewSource(2)) //nolint:gosec // G404: deterministic test fuzzing, not security-sensitive
	var cases []string
	for _, lit := range fuzzLiterals {
		if lit != "" { // a header/query key can never be empty
			cases = append(cases, lit)
		}
	}
	for range 100 {
		if lit := randLiteral(rng); lit != "" {
			cases = append(cases, lit)
		}
	}
	for i, lit := range cases {
		entries := []EntrySpec{{
			Method:  "GET",
			URL:     PlainText("http://example.com"),
			Headers: []Field{{Key: PlainText(lit), Value: PlainText("v")}},
		}}
		f, err := BuildFile(entries, DialectHurl)
		if err != nil {
			t.Fatalf("case %d (%q): BuildFile: %v", i, lit, err)
		}
		got := decodedLiteral(t, f.Entries[0].Request.Headers[0].Key)
		if got != lit {
			t.Errorf("case %d: key decoded = %q, want %q", i, got, lit)
		}
	}
}

// TestBuildTypicalEntry is a golden-ish test for a realistic entry using
// most of the builder API at once.
func TestBuildTypicalEntry(t *testing.T) {
	body, err := JSONBody(Members{
		{Key: "name", Value: "Alice"},
		{Key: "role", Value: Text{Lit("admin-"), Var("suffix")}},
		{Key: "age", Value: float64(30)},
		{Key: "tags", Value: []any{"a", "b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := []EntrySpec{{
		Comments: []string{"create a user"},
		Method:   "POST",
		URL:      Text{Lit("https://api.example.com/users?since="), Var("since")},
		Headers: []Field{
			{Key: PlainText("Authorization"), Value: Text{Lit("Bearer "), Var("token")}},
			{Key: PlainText("Content-Type"), Value: PlainText("application/json")},
		},
		Options: []OptionField{
			{Name: "retry", RawValue: "3"},
			{Name: "compressed", RawValue: "true"},
		},
		Body: body,
		Response: &ResponseSpec{
			Status: "201",
		},
	}}
	f, err := BuildFile(entries, DialectHurl)
	if err != nil {
		t.Fatalf("BuildFile: %v", err)
	}
	got := string(Format(f))
	want := `# create a user
POST https://api.example.com/users?since={{since}}
Authorization: Bearer {{token}}
Content-Type: application/json
[Options]
retry: 3
compressed: true
{"name": "Alice", "role": "admin-{{suffix}}", "age": 30, "tags": ["a", "b"]}
HTTP 201
`
	if got != want {
		t.Errorf("Format mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// Format(BuildFile(...)) must be idempotent.
	f2, err := Parse("<fmt>", Format(f), DialectHurl)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if string(Format(f)) != string(Format(f2)) {
		t.Errorf("Format is not idempotent")
	}
}

// TestBuildMultipartAndAuth exercises multipart files, basic auth and a
// raw text (graphql) body.
func TestBuildMultipartAndAuth(t *testing.T) {
	entries := []EntrySpec{{
		Method: "POST",
		URL:    PlainText("https://example.com/upload"),
		Multipart: []MultipartField{
			{Key: PlainText("field1"), Value: PlainText("value1")},
			{Key: PlainText("upload"), File: &MultipartFile{
				Name:        PlainText("data.txt"),
				ContentType: PlainText("text/plain"),
			}},
		},
		BasicAuth: &BasicAuth{User: PlainText("bob"), Password: PlainText("secret")},
	}, {
		Method: "POST",
		URL:    PlainText("https://example.com/graphql"),
		Body:   RawTextBody("query { hero { name } }", "graphql"),
	}}
	f, err := BuildFile(entries, DialectHurl)
	if err != nil {
		t.Fatalf("BuildFile: %v", err)
	}
	req0 := f.Entries[0].Request
	if len(req0.Sections) != 2 {
		t.Fatalf("want 2 sections (basic auth, multipart: canonical order), got %d", len(req0.Sections))
	}
	if req0.Sections[0].Kind != SectionBasicAuth {
		t.Fatalf("bad basic auth section: %#v", req0.Sections[0])
	}
	if req0.Sections[1].Kind != SectionMultipart || len(req0.Sections[1].Multipart) != 2 {
		t.Fatalf("bad multipart section: %#v", req0.Sections[1])
	}

	ms, ok := f.Entries[1].Request.Body.Value.(*MultilineString)
	if !ok || ms.Kind != MultilineGraphQL {
		t.Fatalf("want a graphql multiline body, got %#v", f.Entries[1].Request.Body.Value)
	}

	once := Format(f)
	f2, err := Parse("<fmt>", once, DialectHurl)
	if err != nil {
		t.Fatalf("Format output failed to parse: %v\n%s", err, once)
	}
	if string(once) != string(Format(f2)) {
		t.Errorf("Format is not idempotent")
	}
}

// TestRawTextBodyFallback checks the fence and {{ fallbacks.
func TestRawTextBodyFallback(t *testing.T) {
	tests := []struct {
		name string
		text string
		lang string
	}{
		{"fence", "before ``` after", ""},
		{"fence-graphql", "before ``` after", "graphql"},
		{"braces-graphql", "query { hero } {{oops", "graphql"},
		{"variables-line", "query X\nvariables oops", "graphql"},
		{"plain-braces-ok", "a { b } c", ""}, // single braces are fine untemplated
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := RawTextBody(tc.text, tc.lang)
			entries := []EntrySpec{{Method: "POST", URL: PlainText("https://example.com"), Body: body}}
			f, err := BuildFile(entries, DialectHurl)
			if err != nil {
				t.Fatalf("BuildFile: %v", err)
			}
			once := Format(f)
			if _, err := Parse("<fmt>", once, DialectHurl); err != nil {
				t.Fatalf("Format output failed to parse: %v\n%s", err, once)
			}
		})
	}
}

// TestBytesBody checks the base64 body form round-trips arbitrary bytes.
func TestBytesBody(t *testing.T) {
	data := []byte{0, 1, 2, 253, 254, 255, 'h', 'i'}
	entries := []EntrySpec{{Method: "POST", URL: PlainText("https://example.com"), Body: BytesBody(data)}}
	f, err := BuildFile(entries, DialectHurl)
	if err != nil {
		t.Fatalf("BuildFile: %v", err)
	}
	b, ok := f.Entries[0].Request.Body.Value.(*Base64)
	if !ok {
		t.Fatalf("body is %T, want *Base64", f.Entries[0].Request.Body.Value)
	}
	if string(b.Value) != string(data) {
		t.Errorf("decoded bytes = %v, want %v", b.Value, data)
	}
}

// TestFileBody checks the file body form.
func TestFileBody(t *testing.T) {
	entries := []EntrySpec{{
		Method: "POST", URL: PlainText("https://example.com"),
		Body: FileBody(PlainText("data/payload.bin")),
	}}
	f, err := BuildFile(entries, DialectHurl)
	if err != nil {
		t.Fatalf("BuildFile: %v", err)
	}
	fr, ok := f.Entries[0].Request.Body.Value.(*FileRef)
	if !ok {
		t.Fatalf("body is %T, want *FileRef", f.Entries[0].Request.Body.Value)
	}
	if got := decodedLiteral(t, fr.Filename); got != "data/payload.bin" {
		t.Errorf("filename decoded = %q", got)
	}
}

func TestOptionHelpers(t *testing.T) {
	f, err := BuildFile([]EntrySpec{{
		Method: "GET",
		URL:    PlainText("http://localhost/"),
		Options: []OptionField{
			BoolOption("insecure", true),
			IntOption("max-redirs", 5),
			DurationOption("connect-timeout", 5*time.Second),
			DurationOption("max-time", 1500*time.Millisecond),
			StringOption("proxy", PlainText(" host:3128#x")),
			StringOption("user", Text{Var("user"), Lit(":"), Var("password")}),
			FilenameOption("cacert", PlainText("my certs/ca.pem")),
			VariableOption("name", PlainText(`say "hi" {{x}}`)),
			VariableOption("n", PlainText("true")),
		},
	}}, DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	want := "GET http://localhost/\n[Options]\ninsecure: true\nmax-redirs: 5\nconnect-timeout: 5s\nmax-time: 1500ms\n" +
		"proxy: \\u{20}host:3128\\#x\nuser: {{user}}:{{password}}\ncacert: my\\ certs/ca.pem\n" +
		"variable: name=\"say \\\"hi\\\" \\u{7B}{x}}\"\nvariable: n=\"true\"\n"
	if got := string(Format(f)); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestGraphQLBody(t *testing.T) {
	tests := []struct {
		query Text
		vars  any
		want  string
	}{
		{PlainText("{ me { id } }"), nil, "```graphql\n{ me { id } }\n```"},
		{Text{Lit("query($id: ID!) { user(id: $id) { name } }\n")}, Members{{Key: "id", Value: Text{Var("user_id")}}},
			"```graphql\nquery($id: ID!) { user(id: $id) { name } }\nvariables {\"id\": \"{{user_id}}\"}\n```"},
		{Text{Lit("{ a(x: \""), Var("v"), Lit("\") }")}, map[string]any{}, "```graphql\n{ a(x: \"{{v}}\") }\nvariables {}\n```"},
		{PlainText("{ a }\nvariables\n"), nil, "{\"query\": \"{ a }\\nvariables\\n\"}"},
		{PlainText("{ a(x: \"{{\") }"), Members{{Key: "n", Value: float64(1)}}, "{\"query\": \"{ a(x: \\\"\\u007B{\\\") }\", \"variables\": {\"n\": 1}}"},
	}
	for _, tt := range tests {
		body, err := GraphQLBody(tt.query, tt.vars)
		if err != nil {
			t.Fatal(err)
		}
		f, err := BuildFile([]EntrySpec{{Method: "POST", URL: PlainText("http://x/graphql"), Body: body}}, DialectHurl)
		if err != nil {
			t.Fatalf("%v: %v", tt.query, err)
		}
		got := strings.TrimSuffix(strings.TrimPrefix(string(Format(f)), "POST http://x/graphql\n"), "\n")
		if got != tt.want {
			t.Errorf("GraphQLBody(%v):\ngot  %s\nwant %s", tt.query, got, tt.want)
		}
	}
	if _, err := GraphQLBody(PlainText("{a}"), []any{1.0}); err == nil {
		t.Error("array variables: no error")
	}
}

func TestTextBody(t *testing.T) {
	tests := []struct {
		text Text
		lang string
		want string
	}{
		{Text{Lit("<a>"), Var("x"), Lit("</a>\n")}, "xml", "```xml\n<a>{{x}}</a>\n```"},
		{Text{Lit("hello "), Var("name"), Lit("\n")}, "", "```\nhello {{name}}\n```"},
		{Text{Lit("{ a(x: \""), Var("v"), Lit("\") }\n")}, "graphql", "```graphql\n{ a(x: \"{{v}}\") }\n```"},
		{Text{Lit("lit {{ "), Var("x")}, "xml", "`lit \\u{7B}{ {{x}}`"},
		{Text{Lit("{"), Var("x"), Lit("}")}, "", "`\\u{7B}{{x}}}`"},
		{Text{Lit("q\nvariables {}\n")}, "graphql", "`q\\nvariables {}\\n`"},
		{Text{Lit("a```b")}, "json", "`a\\`\\`\\`b`"},
	}
	for _, tt := range tests {
		f, err := BuildFile([]EntrySpec{{Method: "POST", URL: PlainText("http://x/"), Body: TextBody(tt.text, tt.lang)}}, DialectHurl)
		if err != nil {
			t.Fatalf("%v: %v", tt.text, err)
		}
		got := strings.TrimSuffix(strings.TrimPrefix(string(Format(f)), "POST http://x/\n"), "\n")
		if got != tt.want {
			t.Errorf("TextBody(%v, %q):\ngot  %s\nwant %s", tt.text, tt.lang, got, tt.want)
		}
		body := f.Entries[0].Request.Body
		if body == nil {
			t.Fatal("no body")
		}
	}
}

// TestBodyBuildersAdversarial checks that no literal text can break a
// body out of its string or turn into a live placeholder.
func TestBodyBuildersAdversarial(t *testing.T) {
	texts := []Text{
		{Lit("see `x`")},
		{Lit("``"), Lit("`x")},
		{Lit("{"), Lit("{x}}")},
		{Lit("a{"), Var("v"), Lit("`")},
		{Lit("line\r\nvariables {}\n"), Var("v")},
		{Lit("nul\x00byte`")},
		{Lit("```")},
		{Lit("`")},
	}
	for _, text := range texts {
		for _, lang := range []string{"", "json", "xml", "graphql"} {
			bodies := []*BodySpec{TextBody(text, lang)}
			if g, err := GraphQLBody(text, Members{{Key: "a", Value: Text{Var("w")}}}); err == nil {
				bodies = append(bodies, g)
			} else {
				t.Errorf("GraphQLBody(%v): %v", text, err)
			}
			for _, body := range bodies {
				f, err := BuildFile([]EntrySpec{{Method: "POST", URL: PlainText("http://x/"), Body: body}}, DialectHurl)
				if err != nil {
					t.Errorf("%v, %q: %v", text, lang, err)
					continue
				}
				// Only the placeholders of text (and the variables' w)
				// may be live.
				src := string(Format(f))
				for _, name := range []string{"x"} {
					if strings.Contains(src, "{{"+name+"}}") {
						t.Errorf("%v, %q: literal became a live placeholder:\n%s", text, lang, src)
					}
				}
			}
		}
	}
	if b := TextBody(Text{Lit("\xff\xfe")}, ""); !strings.HasPrefix(b.src, "base64,") {
		t.Errorf("invalid UTF-8 body = %s, want base64", b.src)
	}
	if b := RawTextBody("ends with `", "xml"); b.src[0] != '`' {
		t.Errorf("RawTextBody trailing backtick = %s", b.src)
	}
}

func TestBuildFileRejectsBadMethod(t *testing.T) {
	for _, m := range []string{"", "GET x\nGET", "GET x", "PO{ST"} {
		if _, err := BuildFile([]EntrySpec{{Method: m, URL: PlainText("http://x/")}}, DialectHurl); err == nil {
			t.Errorf("method %q: no error", m)
		}
	}
	if _, err := BuildFile([]EntrySpec{{Method: "PROPFIND", URL: PlainText("http://x/")}}, DialectHurl); err != nil {
		t.Errorf("PROPFIND: %v", err)
	}
}
