// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"testing"
	"unicode/utf8"
)

// FuzzCompletionHover runs completion and hover at every byte offset of a
// fuzzed document (plus a few offsets past the end), in both position
// encodings. Both are driven from arbitrary offsets by an editor, on text
// that may not parse, so neither may ever panic.
func FuzzCompletionHover(f *testing.F) {
	f.Add("GET http://a/\nHTTP 200\n[Asserts]\nstatus == 200\n", false)
	f.Add("GET http://a/😀/\n[Options]\nvariable: token={{x}}\n", true)
	f.Add("", false)
	f.Add("\xEF\xBB\xBFGET http://a/\n", false)
	f.Add("GET http://a/\n[Captures]\nid: jsonpath \"$.id\" redact\n[", true)
	f.Add("GET http://a/\nHTTP 200\n[Asserts]\n{{not a query", false)
	f.Add("GET http://a/\n# opencollection assertion: x eq 1\n", false)

	s, err := NewServer(Options{Version: "fuzz"})
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, text string, utf16 bool) {
		if !utf8.ValidString(text) {
			return // LSP transports text as JSON, which can't carry invalid UTF-8
		}
		d := newDocument("file:///fuzz.hurl", 1, text, utf16)

		// Every offset in the text, capped so one huge fuzzed input can't
		// make a single execution slow, plus a few offsets past the end.
		limit := min(len(text), 4096)
		probe := func(off int) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic at offset %d (utf16=%v) on %q: %v", off, utf16, text, r)
				}
			}()
			_ = s.completion(d, off)
			_ = s.hover(d, off)
		}
		for off := 0; off <= limit; off++ {
			probe(off)
		}
		probe(len(text))
		probe(len(text) + 1)
		probe(len(text) + 64)
	})
}
