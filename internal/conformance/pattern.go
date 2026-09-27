// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"regexp"
	"strings"
	"unicode/utf16"
)

// This file ports the oracle's small text-comparison helpers: the
// "<<<...>>>" line-pattern syntax, the curl debug-log filter, and the
// stdout/stderr byte-decoding rules. The behavior mirrors the upstream
// integration test runner (integration/test_script.py and
// integration/test_pattern.py in the reference repository) so that our
// pass/fail verdicts agree with it.

var patternTokenRE = regexp.MustCompile(`<<<([^>]+)>>>`)

// regexMetacharacters are the characters parsePattern escapes in the
// literal (non-`<<<...>>>`) portions of a pattern line.
const regexMetacharacters = `\/.{}()[]^$*+?|`

// escapeRegexMetacharacters escapes every RE2 metacharacter in s so it
// matches literally.
func escapeRegexMetacharacters(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		if strings.ContainsRune(regexMetacharacters, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// parsePattern turns one oracle line (which may contain `<<<regex>>>`
// islands) into a Go regexp source anchored with ^...$. Text outside
// `<<<...>>>` is escaped literally; text inside is inserted verbatim as a
// regular expression.
func parsePattern(line string) string {
	var b strings.Builder
	last := 0
	for _, loc := range patternTokenRE.FindAllStringSubmatchIndex(line, -1) {
		b.WriteString(escapeRegexMetacharacters(line[last:loc[0]]))
		b.WriteString(line[loc[2]:loc[3]])
		last = loc[1]
	}
	b.WriteString(escapeRegexMetacharacters(line[last:]))
	return "^" + b.String() + "$"
}

// compileLinePattern compiles one oracle line into a matcher. Patterns come
// from vendored, trusted test fixtures.
func compileLinePattern(line string) (*regexp.Regexp, error) {
	return regexp.Compile(parsePattern(line))
}

// ignoreLines drops lines the oracle considers noise: curl's "**" debug
// trace lines, and a known libcurl.so warning that varies by host.
func ignoreLines(text string) string {
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(line, "**") {
			continue
		}
		if strings.Contains(line, "libcurl.so.4: no version information available") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// splitCRLF splits actual process output on either "\n" or "\r\n", matching
// the oracle's re.split(r"\r?\n", ...) over decoded actual output.
func splitCRLF(text string) []string {
	return regexp.MustCompile(`\r?\n`).Split(text, -1)
}

// universalNewlines rewrites "\r\n" and lone "\r" to "\n", matching Python's
// text-mode file reads (the default newline=None "universal newlines").
func universalNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOM = []byte{0xFF, 0xFE}
	utf16BEBOM = []byte{0xFE, 0xFF}
)

// decodeString decodes raw process output the way the oracle's
// decode_string does: sniff a UTF-8 or UTF-16 BOM, otherwise assume UTF-8.
// Invalid bytes are replaced rather than rejected, since the process under
// test is untrusted and a decode error should show up as a text mismatch,
// not a harness crash.
func decodeString(b []byte) string {
	switch {
	case hasPrefix(b, utf8BOM):
		return string(b[len(utf8BOM):])
	case hasPrefix(b, utf16LEBOM):
		return decodeUTF16(b[len(utf16LEBOM):], false)
	case hasPrefix(b, utf16BEBOM):
		return decodeUTF16(b[len(utf16BEBOM):], true)
	default:
		return string(b)
	}
}

func hasPrefix(b, prefix []byte) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if b[i] != p {
			return false
		}
	}
	return true
}

func decodeUTF16(b []byte, bigEndian bool) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		if bigEndian {
			units[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			units[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	return string(utf16.Decode(units))
}
