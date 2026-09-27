// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"strings"
	"testing"
)

// TestSondeCompletion checks the Sonde extensions are proposed in a .sonde
// file only.
func TestSondeCompletion(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	src := "GET ws://a/\n[\n[Options]\nsonde\n[SondeMessages]\nre\nHTTP 101\n[Asserts]\nsonde\n"
	for _, tc := range []struct {
		uri  string
		want bool
	}{{"file:///w/a.sonde", true}, {"file:///w/a.hurl", false}} {
		c.open(tc.uri, src)
		check := func(pos Position, label string) {
			t.Helper()
			if got := hasLabel(c.completion(tc.uri, pos), label); got != tc.want {
				t.Errorf("%s %v: %q offered = %v", tc.uri, pos, label, got)
			}
		}
		check(Position{1, 1}, "SondeMessages")
		check(Position{3, 5}, "sonde-stream-timeout")
		check(Position{8, 5}, "sondeStream")
	}
	items := c.completion("file:///w/a.sonde", Position{5, 2})
	if it := item(t, items, "send"); it.TextEdit.NewText != "send: " {
		t.Errorf("send = %+v", it)
	}
	item(t, items, "receive")
	item(t, items, "close")
}

func TestSondeHoverAndDiagnostics(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	src := "GET ws://a/\n[Options]\nsonde-stream-timeout: 5s\n[SondeMessages]\nreceive\nHTTP 101\n[Asserts]\nsondeStream count == 1\n"
	if diags := c.open("file:///w/a.sonde", src); len(diags) != 0 {
		t.Errorf("sonde diagnostics = %+v", diags)
	}
	for _, tc := range []struct {
		pos  Position
		want string
	}{
		{Position{2, 3}, "stops an event stream after this time"},
		{Position{3, 3}, "WebSocket steps"},
		{Position{4, 2}, "waits for the next message"},
		{Position{7, 3}, "the data of each event or message received"},
	} {
		if h := c.hover("file:///w/a.sonde", tc.pos); h == nil || !strings.Contains(h.Contents.Value, tc.want) {
			t.Errorf("hover %v = %+v, want %q", tc.pos, h, tc.want)
		}
	}
	diags := c.open("file:///w/a.hurl", src)
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "option `sonde-stream-timeout` requires a .sonde file") {
		t.Errorf("hurl diagnostics = %+v", diags)
	}
}
