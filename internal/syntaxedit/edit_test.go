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
	res := apply(t, src, func(b []byte) (*Result, error) {
		return ApplyEdits("t.hurl", b, []RowInsert{
			{Entry: 1, Section: Query, Key: "limit", Value: "5"},
			{Entry: 2, Section: Asserts, Value: "status == 200"},
		})
	})
	if !strings.Contains(res, "# page: 2\nlimit: 5\n[Options]") || !strings.Contains(res, "users\nHTTP *\n[Asserts]\nstatus == 200\n") {
		t.Errorf("result:\n%s", res)
	}
	for name, r := range map[string]RowInsert{
		"options":    {Entry: 1, Section: Options, Key: "output", Value: "/etc/x"},
		"multipart":  {Entry: 1, Section: Multipart, Key: "f", Value: "file,/etc/passwd;"},
		"basic auth": {Entry: 1, Section: BasicAuth, Key: "u", Value: "p"},
		"two lines":  {Entry: 1, Section: Headers, Key: "A", Value: "1\nB: 2"},
		"comment":    {Entry: 1, Section: Headers, Key: "A", Value: "1 # x"},
		"new entry":  {Entry: 1, Section: Asserts, Value: "status == 200\nGET http://evil"},
	} {
		if _, err := ApplyEdits("t.hurl", []byte(src), []RowInsert{r}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestCommentsAreNotRows checks comments that only look like rows: after
// a blank line, above the next entry, inside a multi-line value.
func TestCommentsAreNotRows(t *testing.T) {
	src := "GET http://x\nA: 1\n\n# Note: next call\n# TODO: remove\nGET http://y\nHTTP 200\n[Asserts]\nbody == ```\n# status == 200\n```\n"
	ms, err := Model("t.hurl", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if rows := ms[0].Rows[Headers]; len(rows) != 1 {
		t.Errorf("headers of entry 1: %+v", rows)
	}
	if rows := ms[1].Rows[Asserts]; len(rows) != 1 {
		t.Errorf("asserts of entry 2: %+v", rows)
	}
	if rows := ms[1].Rows[Headers]; len(rows) != 0 {
		t.Errorf("headers of entry 2: %+v", rows)
	}
}

// TestCRLF inserts lines with the source's line endings.
func TestCRLF(t *testing.T) {
	src := strings.ReplaceAll("GET http://x\nA: 1\n", "\n", "\r\n")
	out := apply(t, src, func(b []byte) (*Result, error) { return AddRow("t.hurl", b, 1, Query, "q", "1") })
	if out != "GET http://x\r\nA: 1\r\n[Query]\r\nq: 1\r\n" {
		t.Errorf("result %q", out)
	}
	out = apply(t, out, func(b []byte) (*Result, error) { return SetBody("t.hurl", b, 1, "```\na\n```") })
	if strings.Count(out, "\n") != strings.Count(out, "\r\n") {
		t.Errorf("mixed line endings %q", out)
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
		res, err := ops[uint(which)%uint(len(ops))]()
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

func TestAddLoginEntryGolden(t *testing.T) {
	src := "# Users\nGET https://api.test/users\nAuthorization: Bearer {{token}}\n"
	out := apply(t, src, func(b []byte) (*Result, error) {
		return AddLoginEntry("t.hurl", b, 1, LoginSpec{
			Request: syntax.EntrySpec{Method: "POST", URL: syntax.PlainText("https://auth.test/token"),
				Form: []syntax.Field{syntax.KV("grant_type", "client_credentials"), {Key: syntax.PlainText("client_id"), Value: syntax.Text{syntax.Var("client_id")}}}},
			Capture: "token", JSONPath: `$["access token"]`, Redact: true,
		})
	})
	want := "POST https://auth.test/token\n[Form]\ngrant_type: client_credentials\nclient_id: {{client_id}}\nHTTP 200\n" +
		"[Captures]\ntoken: jsonpath \"$[\\\"access token\\\"]\" redact\n\n# Users\nGET https://api.test/users\nAuthorization: Bearer {{token}}\n"
	if out != want {
		t.Errorf("result:\n%s\nwant:\n%s", out, want)
	}
}

func TestRemoveSection(t *testing.T) {
	src := "# upload\nPOST https://api.test/files\n[Multipart]\nname: a\n# note: b\nfile: file,a.txt;\n[Options]\nretry: 1\nHTTP 201\n"
	out := apply(t, src, func(b []byte) (*Result, error) { return RemoveSection("t.hurl", b, 1, Multipart) })
	if want := "# upload\nPOST https://api.test/files\n[Options]\nretry: 1\nHTTP 201\n"; out != want {
		t.Errorf("result:\n%s\nwant:\n%s", out, want)
	}
	// A missing section: nothing changes; a section without a header: refused.
	if res, err := RemoveSection("t.hurl", []byte(src), 1, Form); err != nil || string(res.Source) != src {
		t.Errorf("missing section: %v", err)
	}
	if _, err := RemoveSection("t.hurl", []byte(src), 1, Headers); err == nil {
		t.Error("headers have no section to remove")
	}
}

// TestRemoveSectionWithDisabledRows: disabled rows in a section are removed too.
func TestRemoveSectionWithDisabledRows(t *testing.T) {
	src := "POST https://api.test/files\n[Query]\na: 1\n# b: 2\nc: 3\nHTTP 200\n"
	out := apply(t, src, func(b []byte) (*Result, error) { return RemoveSection("t.hurl", b, 1, Query) })
	if !strings.Contains(out, "POST https://api.test/files\nHTTP 200\n") || strings.Contains(out, "[Query]") || strings.Contains(out, "a: 1") || strings.Contains(out, "# b: 2") {
		t.Errorf("disabled rows not removed:\n%s", out)
	}
}

// TestRemoveSectionBeforeBody: removing the last request section before the body.
func TestRemoveSectionBeforeBody(t *testing.T) {
	src := "POST https://api.test/files\n[Query]\nq: 1\n{\"data\": 1}\n"
	out := apply(t, src, func(b []byte) (*Result, error) { return RemoveSection("t.hurl", b, 1, Query) })
	if want := "POST https://api.test/files\n{\"data\": 1}\n"; out != want {
		t.Errorf("result:\n%s\nwant:\n%s", out, want)
	}
}

// TestRemoveSectionResponseSections: removing [Captures] before [Asserts].
func TestRemoveSectionResponseSections(t *testing.T) {
	src := "GET https://api.test/x\nHTTP 200\n[Captures]\nid: jsonpath \"$.id\"\n[Asserts]\nstatus == 200\n"
	out := apply(t, src, func(b []byte) (*Result, error) { return RemoveSection("t.hurl", b, 1, Captures) })
	if want := "GET https://api.test/x\nHTTP 200\n[Asserts]\nstatus == 200\n"; out != want {
		t.Errorf("result:\n%s\nwant:\n%s", out, want)
	}
}

// TestRemoveSectionWithCRLF: CRLF line endings are preserved.
func TestRemoveSectionWithCRLF(t *testing.T) {
	src := strings.ReplaceAll("POST https://api.test/files\n[Query]\nq: 1\nHTTP 200\n", "\n", "\r\n")
	out := apply(t, src, func(b []byte) (*Result, error) { return RemoveSection("t.hurl", b, 1, Query) })
	want := strings.ReplaceAll("POST https://api.test/files\nHTTP 200\n", "\n", "\r\n")
	if out != want {
		t.Errorf("result %q", out)
	}
}

// TestSetRowAsWritten: a row set to its own key and value is left as
// written; an empty value has no trailing space.
func TestSetRowAsWritten(t *testing.T) {
	src := "GET https://h\nX-Empty:\nX-Spaced:    a\n"
	for i, kv := range [][2]string{{"X-Empty", ""}, {"X-Spaced", "a"}} {
		res, err := SetRow("t.hurl", []byte(src), 1, Headers, i, kv[0], kv[1])
		if err != nil || string(res.Source) != src {
			t.Errorf("row %d: %q %v", i, res.Source, err)
		}
	}
	out := apply(t, src, func(b []byte) (*Result, error) { return SetRow("t.hurl", b, 1, Headers, 1, "X-Spaced", "") })
	if out != "GET https://h\nX-Empty:\nX-Spaced:\n" {
		t.Errorf("empty value: %q", out)
	}
}

// TestSetRowEmptyValueBehavior: setting a row to empty value preserves structure.
func TestSetRowEmptyValueBehavior(t *testing.T) {
	src := "GET https://h\nX-A: value\nX-B: another\n"
	out := apply(t, src, func(b []byte) (*Result, error) { return SetRow("t.hurl", b, 1, Headers, 0, "X-A", "") })
	if !strings.Contains(out, "X-A:\n") || !strings.Contains(out, "X-B: another") {
		t.Errorf("empty value result:\n%s", out)
	}
}

// TestSetRowWithNonASCII: SetRow with non-ASCII text reparses correctly.
func TestSetRowWithNonASCII(t *testing.T) {
	src := "GET https://api.test/🚀\nContent-Language: en\n"
	out := apply(t, src, func(b []byte) (*Result, error) {
		return SetRow("t.hurl", b, 1, Headers, 0, "Content-Language", "日本語")
	})
	if !strings.Contains(out, "Content-Language: 日本語\n") {
		t.Errorf("non-ASCII not set:\n%s", out)
	}
}

// TestGrpcRows: [SondeGrpc] rows read and edit like any keyed section.
func TestGrpcRows(t *testing.T) {
	src := "POST http://h:50051/inv.v1.Inventory/GetStock\n[SondeGrpc]\nproto: protos/inv.proto\n{\"sku\": \"a\"}\nHTTP 200\n"
	m, err := Model("t.sonde", []byte(src))
	if err != nil || len(m) != 1 || len(m[0].Rows[Grpc]) != 1 || m[0].Rows[Grpc][0].Value != "protos/inv.proto" {
		t.Fatalf("model %+v %v", m, err)
	}
	res, err := AddRow("t.sonde", []byte(src), 1, Grpc, "import-path", "protos")
	if err != nil {
		t.Fatal(err)
	}
	if want := "POST http://h:50051/inv.v1.Inventory/GetStock\n[SondeGrpc]\nproto: protos/inv.proto\nimport-path: protos\n{\"sku\": \"a\"}\nHTTP 200\n"; string(res.Source) != want {
		t.Errorf("add row:\n%s", res.Source)
	}
}

// TestGrpcAddRowCreatesSection: AddRow to a .sonde entry without [SondeGrpc] creates it.
func TestGrpcAddRowCreatesSection(t *testing.T) {
	src := "POST http://h:50051/inv.v1.Inventory/GetStock\n{\"sku\": \"a\"}\nHTTP 200\n"
	res, err := AddRow("t.sonde", []byte(src), 1, Grpc, "proto", "inv.proto")
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Source)
	if !strings.Contains(out, "[SondeGrpc]\nproto: inv.proto\n") {
		t.Errorf("section not created:\n%s", out)
	}
	m, _ := Model("t.sonde", res.Source)
	if len(m[0].Rows[Grpc]) != 1 {
		t.Errorf("grpc rows: %+v", m[0].Rows[Grpc])
	}
}

// TestGrpcToggleRow: ToggleRow on grpc rows disables and enables them.
func TestGrpcToggleRow(t *testing.T) {
	src := "POST http://h:50051/inv.v1.Inventory/GetStock\n[SondeGrpc]\nproto: inv.proto\nimport-path: protos\nHTTP 200\n"
	res, err := ToggleRow("t.sonde", []byte(src), 1, Grpc, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Source)
	if !strings.Contains(out, "# proto: inv.proto\n") {
		t.Errorf("row not disabled:\n%s", out)
	}
	res, err = ToggleRow("t.sonde", res.Source, 1, Grpc, 0)
	if err != nil {
		t.Fatal(err)
	}
	out = string(res.Source)
	if !strings.Contains(out, "proto: inv.proto\n") || strings.Contains(out, "# proto") {
		t.Errorf("row not re-enabled:\n%s", out)
	}
}
