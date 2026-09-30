// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package editsvc

import (
	"context"
	"strings"
	"testing"

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
