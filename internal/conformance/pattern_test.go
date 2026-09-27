// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import "testing"

// These mirror integration/test_pattern.py in the reference repository so
// our regex translation agrees with the oracle's.

func TestParsePatternNoEscaping(t *testing.T) {
	got := parsePattern("Hello World!")
	want := "^Hello World!$"
	if got != want {
		t.Errorf("parsePattern() = %q, want %q", got, want)
	}
}

func TestParsePatternRegexIsland(t *testing.T) {
	got := parsePattern("Hello <<<.*>>>!")
	want := "^Hello .*!$"
	if got != want {
		t.Errorf("parsePattern() = %q, want %q", got, want)
	}
}

func TestParsePatternJSON(t *testing.T) {
	got := parsePattern(`{"time":<<<\d+>>>}`)
	want := `^\{"time":\d+\}$`
	if got != want {
		t.Errorf("parsePattern() = %q, want %q", got, want)
	}
}

func TestEscapeRegexMetacharacters(t *testing.T) {
	cases := []struct{ in, want string }{
		{"***", `\*\*\*`},
		{`\`, `\\`},
		{"a.b", `a\.b`},
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := escapeRegexMetacharacters(c.in); got != c.want {
			t.Errorf("escapeRegexMetacharacters(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompileLinePatternMatches(t *testing.T) {
	re, err := compileLinePattern(`Duration: <<<[0-9.]+>>> seconds`)
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("Duration: 0.0423 seconds") {
		t.Errorf("expected match")
	}
	if re.MatchString("Duration: abc seconds") {
		t.Errorf("expected no match")
	}
}

func TestIgnoreLines(t *testing.T) {
	in := "keep me\n** curl debug\nlibcurl.so.4: no version information available (required by curl)\nkeep too"
	want := "keep me\nkeep too"
	if got := ignoreLines(in); got != want {
		t.Errorf("ignoreLines() = %q, want %q", got, want)
	}
}

func TestUniversalNewlines(t *testing.T) {
	in := "a\r\nb\rc\nd"
	want := "a\nb\nc\nd"
	if got := universalNewlines(in); got != want {
		t.Errorf("universalNewlines() = %q, want %q", got, want)
	}
}

func TestSplitCRLF(t *testing.T) {
	got := splitCRLF("a\r\nb\nc")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("splitCRLF() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitCRLF()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDecodeStringPlain(t *testing.T) {
	if got := decodeString([]byte("hello")); got != "hello" {
		t.Errorf("decodeString() = %q, want %q", got, "hello")
	}
}

func TestDecodeStringUTF8BOM(t *testing.T) {
	b := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...)
	if got := decodeString(b); got != "hello" {
		t.Errorf("decodeString() = %q, want %q", got, "hello")
	}
}

func TestDecodeStringUTF16LE(t *testing.T) {
	// BOM (FF FE) + "hi" as UTF-16LE.
	b := []byte{0xFF, 0xFE, 'h', 0x00, 'i', 0x00}
	if got := decodeString(b); got != "hi" {
		t.Errorf("decodeString() = %q, want %q", got, "hi")
	}
}

func TestDecodeStringUTF16BE(t *testing.T) {
	b := []byte{0xFE, 0xFF, 0x00, 'h', 0x00, 'i'}
	if got := decodeString(b); got != "hi" {
		t.Errorf("decodeString() = %q, want %q", got, "hi")
	}
}
