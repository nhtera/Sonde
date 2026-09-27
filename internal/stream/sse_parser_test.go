// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"reflect"
	"testing"
)

func intp(n int) *int { return &n }

func TestParseSSE(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     []Event
	}{
		{"single", "data: hello\n\n", []Event{{Type: "message", Data: "hello"}}},
		{"multi-line data", "data: a\ndata:b\ndata:  c\n\n", []Event{{Type: "message", Data: "a\nb\n c"}}},
		{"event and id", "event: ready\nid: 7\ndata: {}\n\ndata: x\n\n",
			[]Event{{Type: "ready", Data: "{}", ID: "7"}, {Type: "message", Data: "x", ID: "7"}}},
		{"CRLF and CR", "data: a\r\n\r\ndata: b\r\rdata: c\n\n",
			[]Event{{Type: "message", Data: "a"}, {Type: "message", Data: "b"}, {Type: "message", Data: "c"}}},
		{"BOM stripped once", "\uFEFFdata: a\n\n", []Event{{Type: "message", Data: "a"}}},
		{"second BOM kept", "\uFEFF\uFEFFdata: a\n\n", nil},
		{"comments and unknown fields", ": ping\nfoo: bar\ndata: a\n\n", []Event{{Type: "message", Data: "a"}}},
		{"no data, no event", "event: x\n\ndata: a\n\n", []Event{{Type: "message", Data: "a"}}},
		{"empty data line", "data\n\n", []Event{{Type: "message", Data: ""}}},
		{"field without colon", "data\ndata\n\n", []Event{{Type: "message", Data: "\n"}}},
		{"id with NUL ignored", "id: 1\ndata: a\n\nid: 2\x00\ndata: b\n\n",
			[]Event{{Type: "message", Data: "a", ID: "1"}, {Type: "message", Data: "b", ID: "1"}}},
		{"empty id resets", "id: 1\ndata: a\n\nid\ndata: b\n\n",
			[]Event{{Type: "message", Data: "a", ID: "1"}, {Type: "message", Data: "b"}}},
		{"retry digits only", "retry: 3000\ndata: a\n\nretry: 3s\ndata: b\n\n",
			[]Event{{Type: "message", Data: "a", Retry: intp(3000)}, {Type: "message", Data: "b"}}},
		{"partial event dropped", "data: a\n\ndata: b\n", []Event{{Type: "message", Data: "a"}}},
		{"one space stripped", "data:  two\n\n", []Event{{Type: "message", Data: " two"}}},
		{"invalid UTF-8 replaced", "data: \xff\n\n", []Event{{Type: "message", Data: "\uFFFD"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseSSE([]byte(tc.in)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestSSEParserChunks feeds a stream byte by byte, splitting CRLF and the
// BOM across writes.
func TestSSEParserChunks(t *testing.T) {
	in := "\uFEFFevent: e\r\ndata: a\r\n\r\ndata: b\r\rdata: c\n\n"
	want := ParseSSE([]byte(in))
	var p SSEParser
	var got []Event
	for i := range len(in) {
		got = append(got, p.Write([]byte{in[i]})...)
	}
	if !reflect.DeepEqual(got, want) || len(got) != 3 {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// FuzzSSEParser checks that the parser never panics and that splitting the
// input into chunks never changes the events.
func FuzzSSEParser(f *testing.F) {
	for _, s := range []string{"data: a\n\n", "\uFEFFid: 1\r\ndata: b\r\n\r\n", "retry: 10\rdata\r\r", ": c\n"} {
		f.Add([]byte(s), uint8(3))
	}
	f.Fuzz(func(t *testing.T, in []byte, size uint8) {
		want := ParseSSE(in)
		n := int(size%16) + 1
		var p SSEParser
		var got []Event
		for len(in) > 0 {
			k := min(n, len(in))
			got = append(got, p.Write(in[:k])...)
			in = in[k:]
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("chunks of %d: got %+v, want %+v", n, got, want)
		}
	})
}
