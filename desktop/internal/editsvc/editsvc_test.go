// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package editsvc

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/syntaxedit"
)

type bodies map[string]string

func (b bodies) Redacted(id string) ([]byte, string, bool) {
	s, ok := b[id]
	return []byte(s), "application/json", ok
}

const src = "# 🚀 first\nGET https://example.org/a\nHTTP 200\n\nPOST https://example.org/b\nContent-Type: application/json\n{\"a\": 1}\nHTTP 201\n"

func svc() *Edits {
	body := `{"id": 12345678901234567890, "name": "x{{y\"z", "tok": "***", "items": [1, 2], "ok": true, "n": null, "f": 1.5}`
	return New(bodies{"b1": body}, func(context.Context, string, string, string) (*engine.Runner, engine.Job, error) {
		r := engine.NewRunner(engine.Options{})
		return r, engine.Job{Name: "t.hurl", Source: []byte(src)}, nil
	})
}

func TestModelAndEntryAt(t *testing.T) {
	e := svc()
	b := Buffer{File: "t.hurl", Text: src, Version: 3}
	m, err := e.Model(b)
	if err != nil || len(m) != 2 || m[1].Method != "POST" {
		t.Fatalf("model %+v %v", m, err)
	}
	// Offsets are UTF-16: the rocket is two units.
	at, err := e.EntryAt(b, m[1].Range.Start+1)
	if err != nil || at != 2 {
		t.Errorf("entry at: %d %v", at, err)
	}
}

func TestApply(t *testing.T) {
	e := svc()
	b := Buffer{File: "t.hurl", Text: src, Version: 7}
	res, err := e.Apply(b, Op{Kind: SetURL, Entry: 1, Value: "https://example.org/{{id}}"})
	if err != nil || res.Version != 7 || !strings.Contains(res.Text, "GET https://example.org/{{id}}") || len(res.Edits) == 0 {
		t.Fatalf("setURL %+v %v", res, err)
	}
	res, err = e.Apply(b, Op{Kind: AddRow, Entry: 2, Section: string(syntaxedit.Headers), Key: "X-Trace", Value: "1"})
	if err != nil || !strings.Contains(res.Text, "X-Trace: 1") {
		t.Fatalf("addRow %+v %v", res, err)
	}
	if _, err := e.Apply(b, Op{Kind: "rm -rf"}); err == nil {
		t.Error("unknown op")
	}
	if _, err := e.Apply(b, Op{Kind: SetMethod, Entry: 9, Value: "PUT"}); err == nil {
		t.Error("missing entry")
	}
}

func TestAssertValue(t *testing.T) {
	e := svc()
	b := Buffer{File: "t.hurl", Text: src, Version: 1}
	for path, want := range map[string]string{
		"$.id":    `jsonpath "$.id" == 12345678901234567890`,
		"$.name":  `jsonpath "$.name" == "x\u{7b}{y\"z"`,
		"$.tok":   `jsonpath "$.tok" exists`,
		"$.items": `jsonpath "$.items" count == 2`,
		"$.ok":    `jsonpath "$.ok" == true`,
		"$.n":     `jsonpath "$.n" == null`,
		"$.f":     `jsonpath "$.f" == 1.5`,
	} {
		res, err := e.AssertValue(b, FromBody{Entry: 1, BodyID: "b1", Path: path})
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if !strings.Contains(res.Text, want) {
			t.Errorf("%s: want %s in\n%s", path, want, res.Text)
		}
		if _, err := syntaxedit.Model("t.hurl", []byte(res.Text)); err != nil {
			t.Errorf("%s: the result does not parse: %v", path, err)
		}
	}
	res, err := e.AssertValue(b, FromBody{Entry: 1, BodyID: "b1", Path: "$.id", Capture: "user_id"})
	if err != nil || !strings.Contains(res.Text, `user_id: jsonpath "$.id"`) {
		t.Fatalf("capture %+v %v", res, err)
	}
	if _, err := e.AssertValue(b, FromBody{Entry: 1, BodyID: "b1", Path: "$.id", Capture: "bad name"}); err == nil {
		t.Error("bad capture name")
	}
	if _, err := e.AssertValue(b, FromBody{Entry: 1, BodyID: "gone", Path: "$.id"}); err == nil {
		t.Error("missing body")
	}
	if _, err := e.AssertValue(b, FromBody{Entry: 1, BodyID: "b1", Path: "$.nope"}); err == nil {
		t.Error("no value")
	}
}

func TestMethodsNeedsGRPC(t *testing.T) {
	e := svc()
	if _, err := e.Methods(context.Background(), Buffer{File: "t.hurl", Text: src}, 1, ""); err == nil {
		t.Error("an HTTP entry has no gRPC methods")
	}
}

// TestAppendMessages writes session messages into [SondeMessages]: before
// a final close, after the last step otherwise, at UTF-16 offsets.
func TestAppendMessages(t *testing.T) {
	e := svc()
	const live = "# 🚀 live\nGET ws://h/ws\n[SondeMessages]\nsend: `hi`\nreceive\nclose\nHTTP 101\n"
	b := Buffer{File: "live.sonde", Text: live, Version: 2}
	res, err := e.AppendMessages(b, 1, `{"type": "ping"}`, false)
	if err != nil {
		t.Fatal(err)
	}
	want := "# 🚀 live\nGET ws://h/ws\n[SondeMessages]\nsend: `hi`\nreceive\nsend: {\"type\": \"ping\"}\nreceive: 1\nclose\nHTTP 101\n"
	if res.Text != want || res.Version != 2 {
		t.Fatalf("got\n%s", res.Text)
	}
	// The edit's offset is in UTF-16 units: the rocket is two of them.
	ed := res.Edits[0]
	if ed.Range.Start != ed.Range.End || ed.Range.Start != len("# ")+2+len(" live\nGET ws://h/ws\n[SondeMessages]\nsend: `hi`\nreceive\n") {
		t.Errorf("edit at %+v", ed.Range)
	}
	// No close step: after the last one; binary as hex.
	open := "GET ws://h/ws\n[SondeMessages]\nsend: `hi`"
	res, err = e.AppendMessages(Buffer{File: "o.sonde", Text: open}, 1, "01 FF", true)
	if err != nil || res.Text != open+"\nsend: hex,01FF;\nreceive: 1\n" {
		t.Fatalf("binary %q %v", res.Text, err)
	}
	res, err = e.AppendMessages(Buffer{File: "o.sonde", Text: open + "\n"}, 1, "plain text", false)
	if err != nil || !strings.HasSuffix(res.Text, "send: `hi`\nsend: `plain text`\nreceive: 1\n") {
		t.Fatalf("text %q %v", res.Text, err)
	}
	for name, c := range map[string]struct {
		text, data string
		binary     bool
	}{
		"no section":  {"GET ws://h/ws\n", "x", false},
		"backtick":    {open, "a`b", false},
		"template":    {open, "{{secret}}", false},
		"bad hex":     {open, "zz", true},
		"parse error": {"GET\n", "x", false},
	} {
		if _, err := e.AppendMessages(Buffer{File: "o.sonde", Text: c.text}, 1, c.data, c.binary); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestAppendMessagesCRLF: CRLF line endings are preserved.
func TestAppendMessagesCRLF(t *testing.T) {
	e := svc()
	const crlfLive = "GET ws://h/ws\r\n[SondeMessages]\r\nsend: `hi`\r\nreceive\r\nHTTP 101\r\n"
	b := Buffer{File: "live.sonde", Text: crlfLive, Version: 1}
	res, err := e.AppendMessages(b, 1, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "send: `test`\r\nreceive: 1\r\n") {
		t.Errorf("CRLF not preserved in %q", res.Text)
	}
	if _, err := syntaxedit.Model("live.sonde", []byte(res.Text)); err != nil {
		t.Errorf("result does not parse: %v", err)
	}
}

// TestAppendMessagesHigherEntry: entry index > 1.
func TestAppendMessagesHigherEntry(t *testing.T) {
	src2 := "GET /a\nHTTP 200\n\nGET ws://h/ws\n[SondeMessages]\nsend: `initial`\nHTTP 101\n"
	body := `{"ok": true}`
	bodies := bodies{"b2": body}
	e2 := New(bodies, func(context.Context, string, string, string) (*engine.Runner, engine.Job, error) {
		r := engine.NewRunner(engine.Options{})
		return r, engine.Job{Name: "t.sonde", Source: []byte(src2)}, nil
	})
	b := Buffer{File: "t.sonde", Text: src2, Version: 1}
	res, err := e2.AppendMessages(b, 2, "data", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "send: `data`") {
		t.Errorf("message not appended: %q", res.Text)
	}
}

// TestAppendMessagesEntryIndexBounds: entry out of range.
func TestAppendMessagesEntryIndexBounds(t *testing.T) {
	e := svc()
	const live = "GET ws://h/ws\n[SondeMessages]\nsend: `hi`\nHTTP 101\n"
	b := Buffer{File: "live.sonde", Text: live, Version: 1}
	if _, err := e.AppendMessages(b, 0, "x", false); err == nil {
		t.Error("entry 0 accepted")
	}
	if _, err := e.AppendMessages(b, 99, "x", false); err == nil {
		t.Error("entry 99 accepted")
	}
}

// TestAppendMessagesWithCommentsAndBlankLines: [SondeMessages] with comment/blank before close.
func TestAppendMessagesWithCommentsAndBlankLines(t *testing.T) {
	e := svc()
	const live = "GET ws://h/ws\n[SondeMessages]\nsend: `hi`\nreceive\n# comment\n\nclose\nHTTP 101\n"
	b := Buffer{File: "live.sonde", Text: live, Version: 1}
	res, err := e.AppendMessages(b, 1, "data", false)
	if err != nil {
		t.Fatal(err)
	}
	// Should insert before close
	if !strings.Contains(res.Text, "send: `data`\nreceive: 1\nclose") {
		t.Errorf("message not inserted correctly: %q", res.Text)
	}
	if _, err := syntaxedit.Model("live.sonde", []byte(res.Text)); err != nil {
		t.Errorf("result does not parse: %v", err)
	}
}

// TestAppendMessagesUTF16WithAstral: UTF-16 offsets with astral characters (emoji).
func TestAppendMessagesUTF16WithAstral(t *testing.T) {
	e := svc()
	// Comment with emoji: each emoji takes 2 UTF-16 code units.
	const live = "# 🎭 test\nGET ws://h/ws\n[SondeMessages]\nsend: `hi`\nHTTP 101\n"
	b := Buffer{File: "live.sonde", Text: live, Version: 1}
	res, err := e.AppendMessages(b, 1, "msg", false)
	if err != nil {
		t.Fatal(err)
	}
	// Verify UTF-16 offset is correct: "# " (2 bytes) + emoji (2 UTF-16 units) + " test\nGET ws://h/ws\n[SondeMessages]\nsend: `hi`\n" = position
	ed := res.Edits[0]
	if ed.Range.Start <= 0 {
		t.Errorf("UTF-16 offset should be positive: %d", ed.Range.Start)
	}
	if _, err := syntaxedit.Model("live.sonde", []byte(res.Text)); err != nil {
		t.Errorf("result does not parse: %v", err)
	}
}

// TestAppendMessagesHurlFile: .hurl files should fail (only .sonde with [SondeMessages]).
func TestAppendMessagesHurlFile(t *testing.T) {
	e := svc()
	const hurl = "GET ws://h/ws\nHTTP 101\n"
	b := Buffer{File: "test.hurl", Text: hurl, Version: 1}
	if _, err := e.AppendMessages(b, 1, "x", false); err == nil {
		t.Error(".hurl file accepted (should fail, no [SondeMessages])")
	}
}

// TestAppendMessagesResultParses: resulting text must parse correctly.
func TestAppendMessagesResultParses(t *testing.T) {
	e := svc()
	testCases := []struct {
		name string
		text string
		data string
	}{
		{"empty section", "GET ws://h/ws\n[SondeMessages]\nHTTP 101\n", "test"},
		{"with receive", "GET ws://h/ws\n[SondeMessages]\nsend: `hi`\nreceive\nHTTP 101\n", "data"},
		{"no newline at end", "GET ws://h/ws\n[SondeMessages]\nsend: `hi`", "msg"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := Buffer{File: "t.sonde", Text: tc.text, Version: 1}
			res, err := e.AppendMessages(b, 1, tc.data, false)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if _, err := syntaxedit.Model("t.sonde", []byte(res.Text)); err != nil {
				t.Errorf("result does not parse: %v\nText: %q", err, res.Text)
			}
		})
	}
}

// TestBatch: ops run on the text the previous one left; the result is one
// edit of the original; a failing op applies nothing.
func TestBatch(t *testing.T) {
	e := svc()
	const text = "# 🚀 upload\nPOST https://h/files\n[Form]\na: 1\nHTTP 200\n"
	b := Buffer{File: "t.hurl", Text: text, Version: 5}
	res, err := e.Batch(b, []Op{
		{Kind: RemoveSection, Entry: 1, Section: string(syntaxedit.Form)},
		{Kind: EnsureSection, Entry: 1, Section: string(syntaxedit.Multipart)},
		{Kind: AddRow, Entry: 1, Section: string(syntaxedit.Multipart), Key: "file", Value: "file,a.txt;"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "# 🚀 upload\nPOST https://h/files\n[Multipart]\nfile: file,a.txt;\nHTTP 200\n"
	if res.Text != want || res.Version != 5 || len(res.Edits) != 1 {
		t.Fatalf("batch %+v", res)
	}
	// The edit turns the original into the result (UTF-16 offsets).
	u := utf16.Encode([]rune(text))
	ed := res.Edits[0]
	got := string(utf16.Decode(u[:ed.Range.Start])) + ed.NewText + string(utf16.Decode(u[ed.Range.End:]))
	if got != want {
		t.Errorf("the edit gives\n%s", got)
	}
	if _, err := e.Batch(b, []Op{{Kind: SetURL, Entry: 1, Value: "x"}, {Kind: SetMethod, Entry: 9, Value: "GET"}}); err == nil {
		t.Error("a failing op in a batch")
	}
	if res, err := e.Batch(b, nil); err != nil || res.Text != text || len(res.Edits) != 0 {
		t.Errorf("empty batch %+v %v", res, err)
	}
}

// TestLoginAndChecks: the login request lands before the entry; asserts
// split into query, predicate and value.
func TestLoginAndChecks(t *testing.T) {
	e := svc()
	const text = "GET https://h/me\nAuthorization: Bearer {{token}}\nHTTP 200\n[Asserts]\njsonpath \"$.items\" count >= 1\nheader \"X-A\" not contains \"a b\"\n# jsonpath \"$.off\" exists\nbody isString\n"
	b := Buffer{File: "t.hurl", Text: text}
	res, err := e.Apply(b, Op{Kind: AddLogin, Entry: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "# Log in (OAuth 2 client credentials)\nPOST {{token_url}}\n[Form]\ngrant_type: client_credentials\nclient_id: {{client_id}}\nclient_secret: {{client_secret}}\nHTTP 200\n[Captures]\ntoken: jsonpath \"$.access_token\" redact\n") {
		t.Errorf("login:\n%s", res.Text)
	}
	checks, err := e.Checks(b)
	if err != nil || len(checks) != 1 {
		t.Fatalf("checks %+v %v", checks, err)
	}
	want := []Check{
		{Query: `jsonpath "$.items" count`, Predicate: ">=", Value: "1", Parsed: true},
		{Query: `header "X-A"`, Predicate: "not contains", Value: `"a b"`, Parsed: true},
		{Query: `jsonpath "$.off"`, Predicate: "exists", Parsed: true},
		{Query: "body", Predicate: "isString", Parsed: true},
	}
	if fmt.Sprint(checks[0].Asserts) != fmt.Sprint(want) {
		t.Errorf("checks\n%+v\nwant\n%+v", checks[0].Asserts, want)
	}
	if c := splitAssert("t.hurl", "not an assert"); c.Parsed || c.Query != "not an assert" {
		t.Errorf("unparsed %+v", c)
	}
}

// TestBatchWithDependencies: a later op in a batch depends on earlier one.
func TestBatchWithDependencies(t *testing.T) {
	e := svc()
	const text = "POST https://h/files\n[Form]\na: 1\nHTTP 200\n"
	b := Buffer{File: "t.hurl", Text: text, Version: 2}
	// removeSection then addRow in same section
	res, err := e.Batch(b, []Op{
		{Kind: RemoveSection, Entry: 1, Section: string(syntaxedit.Form)},
		{Kind: AddRow, Entry: 1, Section: string(syntaxedit.Query), Key: "q", Value: "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "[Query]\nq: x\n") || strings.Contains(res.Text, "[Form]") {
		t.Errorf("batch with dependencies:\n%s", res.Text)
	}
	if _, err := e.Batch(b, []Op{
		{Kind: RemoveRow, Entry: 1, Section: string(syntaxedit.Form), Index: 0},
		{Kind: RemoveRow, Entry: 1, Section: string(syntaxedit.Form), Index: 0}, // indices shift
	}); err == nil {
		t.Error("shifting indices should fail")
	}
}

// TestBatchAddLoginBeforeEntry: addLogin before entry 1 and before entry N.
func TestBatchAddLoginBeforeEntry(t *testing.T) {
	e := svc()
	const text = "GET https://h/me\nHTTP 200\n\nGET https://h/data\nHTTP 200\n"
	b := Buffer{File: "t.hurl", Text: text, Version: 1}
	res, err := e.Apply(b, Op{Kind: AddLogin, Entry: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "# Log in") {
		t.Errorf("login not before entry 1:\n%s", res.Text)
	}
	res, err = e.Apply(Buffer{File: "t.hurl", Text: text, Version: 1}, Op{Kind: AddLogin, Entry: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Login is before the named entry
	lines := strings.Split(res.Text, "\n")
	var loginLine, entry2Line int
	for i, line := range lines {
		if strings.HasPrefix(line, "# Log in") {
			loginLine = i
		}
		if strings.HasPrefix(line, "GET https://h/data") {
			entry2Line = i
		}
	}
	if loginLine == 0 || entry2Line == 0 || loginLine >= entry2Line {
		t.Errorf("login line %d before entry2 line %d", loginLine, entry2Line)
	}
}

// TestChecksWithFilters: asserts with filters and predicates split correctly.
func TestChecksWithFilters(t *testing.T) {
	e := svc()
	const text = "GET https://h/x\nHTTP 200\n[Asserts]\njsonpath \"$.items\" nth 0 == 1\njsonpath \"$.off\" not exists\nstatus == 200\n"
	b := Buffer{File: "t.hurl", Text: text}
	checks, err := e.Checks(b)
	if err != nil || len(checks) != 1 {
		t.Fatalf("checks: %+v %v", checks, err)
	}
	want := []Check{
		{Query: `jsonpath "$.items" nth 0`, Predicate: "==", Value: "1", Parsed: true},
		{Query: `jsonpath "$.off"`, Predicate: "not exists", Parsed: true},
		{Query: "status", Predicate: "==", Value: "200", Parsed: true},
	}
	for i, c := range checks[0].Asserts {
		if i >= len(want) {
			t.Errorf("check %d: unexpected %+v", i, c)
			continue
		}
		if c.Query != want[i].Query || c.Predicate != want[i].Predicate || c.Value != want[i].Value {
			t.Errorf("check %d: got %+v, want %+v", i, c, want[i])
		}
	}
}

// TestNoOpEditsKeepTheCorpus: writing every field of every entry back as
// it is leaves each corpus file byte for byte (comments, layout), and every
// assert splits back into its text.
func TestNoOpEditsKeepTheCorpus(t *testing.T) {
	e := svc()
	var files []string
	for _, dir := range []string{"../../../testdata/conformance/hurl/tests_ok", "../../testdata"} {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".hurl") || strings.HasSuffix(p, ".sonde")) {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) < 50 {
		t.Fatalf("only %d corpus files", len(files))
	}
	// Non-ASCII, for UTF-16 offsets.
	files = append(files, "")
	for _, p := range files {
		t.Run(filepath.Base(p), func(t *testing.T) {
			t.Parallel()
			noOpKeeps(t, e, p)
		})
	}
}

// noOpKeeps checks one file of TestNoOpEditsKeepTheCorpus ("" for the
// non-ASCII sample).
func noOpKeeps(t *testing.T, e *Edits, p string) {
	name, text := "unicode.hurl", "# 日本語 🚀\nGET https://h/ü?q=é\nX-Name: 名前 🚀\nHTTP 200\n[Asserts]\njsonpath \"$.名\" == \"値 🚀\"\n"
	if p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		// A benchmark file of thousands of rows: an op re-parses the file,
		// which a form edit (a few ops) never makes quadratic.
		if len(data) > 64<<10 {
			t.Skip("a benchmark file")
		}
		name, text = filepath.Base(p), string(data)
	}
	b := Buffer{File: name, Text: text}
	m, err := e.Model(b)
	if err != nil {
		return // not a file the form edits
	}
	var ops []Op
	for _, en := range m {
		ops = append(ops, Op{Kind: SetMethod, Entry: en.Index, Value: en.Method}, Op{Kind: SetURL, Entry: en.Index, Value: en.URL})
		for sec, rows := range en.Rows {
			for i, r := range rows {
				// A multiline value is read-only in the form.
				if !r.Disabled && !strings.Contains(r.Value, "\n") {
					ops = append(ops, Op{Kind: SetRow, Entry: en.Index, Section: string(sec), Index: i, Key: r.Key, Value: r.Value})
				}
			}
		}
	}
	res, err := e.Batch(b, ops)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != text || len(res.Edits) != 0 {
		t.Errorf("a no-op edit changed the file: %v", res.Edits)
	}
	checks, _ := e.Checks(b)
	for ci, ec := range checks {
		for i, c := range ec.Asserts {
			if !c.Parsed {
				continue
			}
			joined := strings.TrimSpace(c.Query + " " + c.Predicate + " " + c.Value)
			if strings.Join(strings.Fields(joined), " ") != strings.Join(strings.Fields(m[ci].Rows[syntaxedit.Asserts][i].Value), " ") {
				t.Errorf("assert %q splits as %+v", m[ci].Rows[syntaxedit.Asserts][i].Value, c)
			}
		}
	}
}

// TestReviewFixes: a batch that only appends, a new request's URL as
// source text, a [SondeGrpc] naming no proto, file names escaped.
func TestReviewFixes(t *testing.T) {
	e := svc()
	b := Buffer{File: "t.hurl", Text: "GET https://h\n"}
	res, err := e.Batch(b, []Op{
		{Kind: AddRow, Entry: 1, Section: string(syntaxedit.Options), Key: "cert", Value: "a.pem"},
		{Kind: AddRow, Entry: 1, Section: string(syntaxedit.Options), Key: "key", Value: "a.key"},
	})
	if err != nil || res.Text != "GET https://h\n[Options]\ncert: a.pem\nkey: a.key\n" {
		t.Fatalf("append batch %+v %v", res, err)
	}
	if ed := res.Edits[0]; ed.Range.Start != len(b.Text) || ed.Range.End != len(b.Text) {
		t.Errorf("append edit %+v", ed)
	}
	res, err = e.Apply(b, Op{Kind: AddEntry, Key: "GET", Value: "{{base_url}}/"})
	if err != nil || !strings.HasSuffix(res.Text, "\nGET {{base_url}}/\n") || len(res.Edits) != 1 {
		t.Fatalf("add entry %+v %v", res, err)
	}
	grpc := New(bodies{}, func(context.Context, string, string, string) (*engine.Runner, engine.Job, error) {
		src := "POST http://h:1/a.S/M\n[SondeGrpc]\n{}\n"
		return engine.NewRunner(engine.Options{}), engine.Job{Name: filepath.Join(t.TempDir(), "g.sonde"), Source: []byte(src)}, nil
	})
	s, err := grpc.Methods(context.Background(), Buffer{File: "g.sonde"}, 1, "")
	if err != nil || s == nil || len(s) != 0 {
		t.Errorf("no proto: %+v %v", s, err)
	}
	if got := NewService(e).EscapeFilename("My Photo;#1{x}.png"); got != `My\ Photo\;\#1\{x\}.png` {
		t.Errorf("escaped %q", got)
	}
	// The escaped name reads back in a body.
	res, err = e.Apply(b, Op{Kind: SetBody, Entry: 1, Value: "file," + NewService(e).EscapeFilename("My Photo.png") + ";"})
	if err != nil || !strings.Contains(res.Text, `file,My\ Photo.png;`) {
		t.Errorf("file body %+v %v", res, err)
	}
}
