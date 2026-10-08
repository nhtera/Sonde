// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"os"
	"strings"
	"testing"
)

// TestTokensCoverSource: the tokens of every valid corpus file concatenate
// back to the file, so highlighting never drops or adds text.
func TestTokensCoverSource(t *testing.T) {
	for _, path := range append(conformanceFiles(t), localSyntaxFiles(t)...) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Parse(path, src, DialectFor(path))
		if err != nil {
			continue
		}
		var b strings.Builder
		for _, tok := range Tokens(f) {
			if tok.Kind != TokenOpen && tok.Kind != TokenClose {
				b.WriteString(tok.Text)
			}
		}
		if b.String() != string(Print(f)) {
			t.Errorf("%s: tokens do not cover the source", path)
		}
	}
}

// TestHighlightANSIReference is the reference formatter's own highlighting
// example.
func TestHighlightANSIReference(t *testing.T) {
	src := "\nGET https://foo.com\nheader1: value1\nheader2: value2\n[Form]\nfoo: bar\nbaz: 123\n" +
		"HTTP 200\n[Asserts]\njsonpath \"$.name\" == \"toto\"\n"
	want := "\n\x1b[33mGET\x1b[0m \x1b[32mhttps://foo.com\x1b[0m\n" +
		"\x1b[32mheader1\x1b[0m: \x1b[32mvalue1\x1b[0m\n" +
		"\x1b[32mheader2\x1b[0m: \x1b[32mvalue2\x1b[0m\n" +
		"\x1b[35m[Form]\x1b[0m\n" +
		"\x1b[32mfoo\x1b[0m: \x1b[32mbar\x1b[0m\n" +
		"\x1b[32mbaz\x1b[0m: \x1b[32m123\x1b[0m\n" +
		"HTTP 200\n" +
		"\x1b[35m[Asserts]\x1b[0m\n" +
		"\x1b[36mjsonpath\x1b[0m \x1b[32m\"$.name\"\x1b[0m \x1b[33m==\x1b[0m \x1b[32m\"toto\"\x1b[0m\n"
	f, err := Parse("t.hurl", []byte(src), DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(HighlightANSI(f)); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestHighlightANSIEmptyValue: an empty value still prints its colour
// codes, as the reference formatter does.
func TestHighlightANSIEmptyValue(t *testing.T) {
	f, err := Parse("t.hurl", []byte("GET http://x\nx-h:\n"), DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	want := "\x1b[33mGET\x1b[0m \x1b[32mhttp://x\x1b[0m\n\x1b[32mx-h\x1b[0m:\x1b[32m\x1b[0m\n"
	if got := string(HighlightANSI(f)); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestHighlightANSIKinds(t *testing.T) {
	src := "# c\nGET http://x\n[Options]\ndelay: 1000ms\ninsecure: true\nvariable: n=1\n" +
		"HTTP 200\n[Asserts]\nheader \"A\" count not == 2 # t\n"
	want := "\x1b[90m# c\x1b[0m\n\x1b[33mGET\x1b[0m \x1b[32mhttp://x\x1b[0m\n\x1b[35m[Options]\x1b[0m\n" +
		"\x1b[32mdelay\x1b[0m: \x1b[36m1000ms\x1b[0m\n\x1b[32minsecure\x1b[0m: \x1b[36mtrue\x1b[0m\n" +
		"\x1b[32mvariable\x1b[0m: n=\x1b[36m1\x1b[0m\nHTTP 200\n\x1b[35m[Asserts]\x1b[0m\n" +
		"\x1b[36mheader\x1b[0m \x1b[32m\"A\"\x1b[0m \x1b[33mcount\x1b[0m \x1b[33mnot\x1b[0m \x1b[33m==\x1b[0m \x1b[36m2\x1b[0m \x1b[90m# t\x1b[0m\n"
	f, err := Parse("t.hurl", []byte(src), DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(HighlightANSI(f)); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
