// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxedit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/nhtera/sonde/internal/syntax"
)

// sample has every kind of row, disabled rows, comments and non-ASCII
// text (emoji, CJK, a combining mark).
const sample = `# Login 🔑
POST https://api.test/こんにちは?x=1
Content-Type: application/json
# X-Debug: 1
[Query]
lang: é
# page: 2
[Options]
insecure: true
{"name": "𝒳"}

HTTP 200
# X-Seen: yes
[Captures]
token: jsonpath "$.token" redact
[Asserts]
jsonpath "$.id" == 1
# jsonpath "$.name" == "x"

# Second
GET https://api.test/users
`

// u16 is the UTF-16 offset of byte offset b of s, computed the slow way.
func u16(s string, b int) int { return len(utf16.Encode([]rune(s[:b]))) }

func TestModel(t *testing.T) {
	ms, err := Model("t.hurl", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 {
		t.Fatalf("%d entries", len(ms))
	}
	m := ms[0]
	if m.Method != "POST" || m.URL != "https://api.test/こんにちは?x=1" || m.Status != "200" || m.Body != `{"name": "𝒳"}` {
		t.Errorf("model %+v", m)
	}
	want := map[Section][]Row{
		Headers:         {{Key: "Content-Type", Value: "application/json"}, {Key: "X-Debug", Value: "1", Disabled: true}},
		Query:           {{Key: "lang", Value: "é"}, {Key: "page", Value: "2", Disabled: true}},
		Options:         {{Key: "insecure", Value: "true"}},
		Captures:        {{Key: "token", Value: `jsonpath "$.token" redact`}},
		Asserts:         {{Value: `jsonpath "$.id" == 1`}, {Value: `jsonpath "$.name" == "x"`, Disabled: true}},
		ResponseHeaders: {{Key: "X-Seen", Value: "yes", Disabled: true}},
	}
	for name, rows := range want {
		got := m.Rows[name]
		if len(got) != len(rows) {
			t.Errorf("%s: %+v", name, got)
			continue
		}
		for i := range rows {
			r := got[i]
			// Ranges are UTF-16 offsets of the row text.
			start := strings.Index(sample, rowText(name, rows[i].Key, rows[i].Value))
			if r.Range != (Range{u16(sample, start), u16(sample, start+len(rowText(name, r.Key, r.Value)))}) {
				t.Errorf("%s row %d range %v", name, i, r.Range)
			}
			r.Range = Range{}
			if r != rows[i] {
				t.Errorf("%s row %d = %+v, want %+v", name, i, r, rows[i])
			}
		}
	}
	urlStart := strings.Index(sample, "https://api.test/こ")
	if m.URLRange.Start != u16(sample, urlStart) {
		t.Errorf("URL range %v", m.URLRange)
	}
	second := strings.Index(sample, "GET https")
	if ms[1].Range.Start != u16(sample, second) || EntryAt(ms, u16(sample, second)+2) != 2 || EntryAt(ms, 0) != 0 || EntryAt(ms, m.URLRange.Start) != 1 {
		t.Errorf("entry ranges %v %v", ms[0].Range, ms[1].Range)
	}
}

// apply runs op and checks the result's invariants: it parses, prints
// back unchanged, and differs from src only inside its edit.
func apply(t *testing.T, src string, op func([]byte) (*Result, error)) string {
	t.Helper()
	res, err := op([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Source)
	f, err := syntax.Parse("t.hurl", res.Source, syntax.DialectHurl)
	if err != nil {
		t.Fatalf("result does not parse: %v\n%s", err, out)
	}
	if string(syntax.Print(f)) != out {
		t.Fatal("result does not print back")
	}
	if len(res.Edits) == 1 {
		e := res.Edits[0]
		o := newOffsets([]byte(src))
		start, _ := o.toByte(e.Range.Start)
		end, _ := o.toByte(e.Range.End)
		if got := src[:start] + e.NewText + src[end:]; got != out {
			t.Fatalf("the edit does not reproduce the result:\n%s", got)
		}
	}
	return out
}

func TestOps(t *testing.T) {
	src := sample
	for _, tc := range []struct {
		name string
		op   func([]byte) (*Result, error)
		want string // a line of the result
		gone string // text no longer in the result
	}{
		{"method", func(b []byte) (*Result, error) { return SetMethod("t.hurl", b, 2, "DELETE") }, "DELETE https://api.test/users", "GET https"},
		{"url", func(b []byte) (*Result, error) { return SetURL("t.hurl", b, 1, "https://api.test/ẞ?q={{q}}") }, "POST https://api.test/ẞ?q={{q}}", "こんにちは"},
		{"status", func(b []byte) (*Result, error) { return SetStatus("t.hurl", b, 1, "201") }, "HTTP 201", "HTTP 200"},
		{"status added", func(b []byte) (*Result, error) { return SetStatus("t.hurl", b, 2, "*") }, "GET https://api.test/users\nHTTP *\n", ""},
		{"set row", func(b []byte) (*Result, error) { return SetRow("t.hurl", b, 1, Query, 0, "lang", "日本") }, "lang: 日本", "lang: é"},
		{"set disabled row", func(b []byte) (*Result, error) { return SetRow("t.hurl", b, 1, Query, 1, "page", "3") }, "# page: 3", "# page: 2"},
		{"add header", func(b []byte) (*Result, error) { return AddRow("t.hurl", b, 2, Headers, "Accept", "*/*") }, "GET https://api.test/users\nAccept: */*\n", ""},
		{"add row", func(b []byte) (*Result, error) { return AddRow("t.hurl", b, 1, Query, "size", "10") }, "# page: 2\nsize: 10\n[Options]", ""},
		{"add section", func(b []byte) (*Result, error) { return AddRow("t.hurl", b, 1, Cookies, "sid", "1") }, "insecure: true\n[Cookies]\nsid: 1\n{", ""},
		{"add capture", func(b []byte) (*Result, error) { return AddCapture("t.hurl", b, 1, "id", `jsonpath "$.id"`) }, "redact\nid: jsonpath \"$.id\"\n[Asserts]", ""},
		{"add assert, new response", func(b []byte) (*Result, error) { return AddAssert("t.hurl", b, 2, "status == 200") }, "users\nHTTP *\n[Asserts]\nstatus == 200\n", ""},
		{"add captures section", func(b []byte) (*Result, error) { return AddCapture("t.hurl", b, 2, "n", "status") }, "users\nHTTP *\n[Captures]\nn: status\n", ""},
		{"remove row", func(b []byte) (*Result, error) { return RemoveRow("t.hurl", b, 1, Headers, 0) }, "POST https://api.test/こんにちは?x=1\n# X-Debug: 1", "Content-Type"},
		{"disable row", func(b []byte) (*Result, error) { return ToggleRow("t.hurl", b, 1, Options, 0) }, "[Options]\n# insecure: true\n", ""},
		{"enable row", func(b []byte) (*Result, error) { return ToggleRow("t.hurl", b, 1, Asserts, 1) }, "\njsonpath \"$.name\" == \"x\"\n", "# jsonpath \"$.name\""},
		{"set body", func(b []byte) (*Result, error) { return SetBody("t.hurl", b, 1, "```\nhello\n```") }, "insecure: true\n```\nhello\n```\n", `{"name"`},
		{"remove body", func(b []byte) (*Result, error) { return SetBody("t.hurl", b, 1, "") }, "insecure: true\n\nHTTP 200", `{"name"`},
		{"add body", func(b []byte) (*Result, error) { return SetBody("t.hurl", b, 2, `{"a": 1}`) }, "users\n{\"a\": 1}\n", ""},
		{"ensure section", func(b []byte) (*Result, error) { return EnsureSection("t.hurl", b, 2, Options) }, "users\n[Options]\n", ""},
		{"remove last entry", func(b []byte) (*Result, error) { return RemoveEntry("t.hurl", b, 2) }, "# jsonpath \"$.name\" == \"x\"\n", "# Second"},
		{"remove first entry", func(b []byte) (*Result, error) { return RemoveEntry("t.hurl", b, 1) }, "# Second\nGET", "Login"},
		{"add entry", func(b []byte) (*Result, error) {
			return AddEntry("t.hurl", b, syntax.EntrySpec{Method: "PUT", URL: syntax.PlainText("https://x/#1"), Headers: []syntax.Field{syntax.KV("A", "b # c")}})
		}, "users\n\nPUT https://x/\\#1\nA: b \\# c\n", ""},
		{"login", func(b []byte) (*Result, error) {
			return AddLoginEntry("t.hurl", b, 2, LoginSpec{
				Request: syntax.EntrySpec{Method: "POST", URL: syntax.PlainText("https://auth.test/token"), Form: []syntax.Field{syntax.KV("grant_type", "client_credentials")}},
				Capture: "access_token", JSONPath: "$.access_token", Redact: true,
			})
		}, "[Captures]\naccess_token: jsonpath \"$.access_token\" redact\n\n# Second\nGET", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := apply(t, src, tc.op)
			if !strings.Contains(out, tc.want) || tc.gone != "" && strings.Contains(out, tc.gone) {
				t.Errorf("result:\n%s", out)
			}
		})
	}
}

func TestRefused(t *testing.T) {
	src := []byte(sample)
	for name, op := range map[string]func() (*Result, error){
		"comment in value":  func() (*Result, error) { return SetRow("t.hurl", src, 1, Query, 0, "lang", "a #b") },
		"newline in value":  func() (*Result, error) { return AddRow("t.hurl", src, 1, Headers, "A", "1\nGET http://evil") },
		"bad method":        func() (*Result, error) { return SetMethod("t.hurl", src, 1, "get x") },
		"assert not assert": func() (*Result, error) { return AddAssert("t.hurl", src, 1, "not an assert") },
		"bad status":        func() (*Result, error) { return SetStatus("t.hurl", src, 1, "20x") },
		"entry range":       func() (*Result, error) { return SetURL("t.hurl", src, 3, "http://x") },
		"row range":         func() (*Result, error) { return RemoveRow("t.hurl", src, 1, Query, 5) },
		"bad capture name":  func() (*Result, error) { return AddLoginEntry("t.hurl", src, 1, LoginSpec{Capture: "a b"}) },
		"two rows":          func() (*Result, error) { return AddRow("t.hurl", src, 1, Options, "retry", "1\nretry: 2") },
	} {
		if res, err := op(); err == nil {
			t.Errorf("%s: accepted:\n%s", name, res.Source)
		}
	}
	if _, err := SetRow("t.hurl", src, 1, Query, 0, "lang", "a #b"); !errors.Is(err, ErrInvalid) {
		t.Errorf("ErrInvalid: %v", err)
	}
}

func TestApplyEdits(t *testing.T) {
	src := sample
	at := u16(src, strings.Index(src, "[Options]"))
	res, err := ApplyEdits("t.hurl", []byte(src), []TextEdit{{Range: Range{at, at}, NewText: "limit: 5\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Source), "# page: 2\nlimit: 5\n[Options]") {
		t.Errorf("result:\n%s", res.Source)
	}
	for name, e := range map[string]TextEdit{
		"two lines":     {Range{at, at}, "a: 1\nb: 2\n"},
		"no newline":    {Range{at, at}, "a: 1"},
		"mid line":      {Range{at + 1, at + 1}, "a: 1\n"},
		"replace":       {Range{at, at + 3}, "a: 1\n"},
		"new entry":     {Range{at, at}, "GET http://evil\n"},
		"comment only":  {Range{at, at}, "# a: 1\n"},
		"section start": {Range{at, at}, "[Cookies]\n"},
	} {
		if _, err := ApplyEdits("t.hurl", []byte(src), []TextEdit{e}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestOffsets(t *testing.T) {
	s := "a😀b́c日"
	o := newOffsets([]byte(s))
	for b := range len(s) + 1 {
		if b < len(s) && !utf8Start(s[b]) {
			continue
		}
		u := o.toU16(b)
		if u != u16(s, b) {
			t.Errorf("toU16(%d) = %d, want %d", b, u, u16(s, b))
		}
		if back, err := o.toByte(u); err != nil || back != b {
			t.Errorf("toByte(%d) = %d, %v, want %d", u, back, err, b)
		}
	}
	if _, err := o.toByte(2); err == nil {
		t.Error("an offset inside a surrogate pair is accepted")
	}
	long := strings.Repeat("é😀", 5000)
	o = newOffsets([]byte(long))
	if got := o.toU16(len(long)); got != u16(long, len(long)) {
		t.Errorf("long: %d", got)
	}
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// TestEditsReparse runs every row operation on every row of the sample:
// each result parses, prints back and changes only its edit.
func TestEditsReparse(t *testing.T) {
	ms, err := Model("t.hurl", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		for name, rows := range m.Rows {
			for i, r := range rows {
				n, sec, i, r := m.Index, name, i, r
				apply(t, sample, func(b []byte) (*Result, error) { return ToggleRow("t.hurl", b, n, sec, i) })
				apply(t, sample, func(b []byte) (*Result, error) { return RemoveRow("t.hurl", b, n, sec, i) })
				apply(t, sample, func(b []byte) (*Result, error) { return SetRow("t.hurl", b, n, sec, i, r.Key, r.Value) })
			}
		}
	}
	if !reflect.DeepEqual(sectionFor(syntax.SectionAsserts), Asserts) {
		t.Error("sectionFor")
	}
}

// FuzzOps runs operations on arbitrary sources: an operation either fails
// or returns a source that parses and prints back.
func FuzzOps(f *testing.F) {
	f.Add(sample, "X-A", "1", 0)
	f.Add("GET http://x\n", "k", "v # c", 1)
	f.Fuzz(func(t *testing.T, src, key, value string, which int) {
		ops := []func() (*Result, error){
			func() (*Result, error) { return AddRow("t.hurl", []byte(src), 1, Headers, key, value) },
			func() (*Result, error) { return AddRow("t.hurl", []byte(src), 1, Query, key, value) },
			func() (*Result, error) { return SetURL("t.hurl", []byte(src), 1, value) },
			func() (*Result, error) { return ToggleRow("t.hurl", []byte(src), 1, Headers, 0) },
			func() (*Result, error) { return SetBody("t.hurl", []byte(src), 1, value) },
			func() (*Result, error) { return AddAssert("t.hurl", []byte(src), 1, value) },
		}
		if which < 0 {
			which = -which
		}
		res, err := ops[which%len(ops)]()
		if err != nil {
			return
		}
		f, err := syntax.Parse("t.hurl", res.Source, syntax.DialectHurl)
		if err != nil {
			t.Fatalf("accepted an edit that does not parse: %v", err)
		}
		if string(syntax.Print(f)) != string(res.Source) {
			t.Fatal("result does not print back")
		}
	})
}
