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
