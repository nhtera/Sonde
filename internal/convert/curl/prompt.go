// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import "bytes"

// stripPromptLines removes a leading shell-prompt marker ("$ ", "% " or
// "> ", the common README/terminal-transcript convention) from the start
// of any line that would otherwise begin with one immediately before
// "curl" (M2, phase 8 review): "$ curl .../one\n$ curl .../two" would
// otherwise tokenize as one long command, since the second "curl" word is
// not first on its own line (the prompt marker is). This runs on the raw
// bytes, before tokenize, rather than inside the tokenizer or commands():
// tokenize's own '>' already means something else (a redirect
// terminator — see its own doc comment), and resolving that ambiguity is
// trivial here, where the marker is simply gone before tokenize ever
// sees the line, but would otherwise require threading extra state
// through the lexer.
func stripPromptLines(src []byte) []byte {
	lines := bytes.SplitAfter(src, []byte("\n"))
	changed := false
	for i, line := range lines {
		if rest, ok := cutPromptMarker(line); ok {
			lines[i] = rest
			changed = true
		}
	}
	if !changed {
		return src
	}
	return bytes.Join(lines, nil)
}

// cutPromptMarker removes a "$ ", "% " or "> " prefix from line (after any
// leading spaces or tabs), reporting whether it did, but only when what
// immediately follows is the word "curl" on its own (followed by
// whitespace, a line ending, or nothing) — never for a line that merely
// starts with one of those bytes for an unrelated reason.
func cutPromptMarker(line []byte) ([]byte, bool) {
	i := skipBlank(line, 0)
	if i+1 >= len(line) {
		return line, false
	}
	switch line[i] {
	case '$', '%', '>':
	default:
		return line, false
	}
	if line[i+1] != ' ' {
		return line, false
	}
	rest := line[i+2:]
	j := skipBlank(rest, 0)
	if !bytes.HasPrefix(rest[j:], []byte("curl")) {
		return line, false
	}
	if after := rest[j+4:]; len(after) > 0 {
		switch after[0] {
		case ' ', '\t', '\r', '\n':
		default:
			return line, false // e.g. "curly", not "curl"
		}
	}
	return rest, true
}

// skipBlank returns the index of the first byte in s at or after start
// that is not a space or tab.
func skipBlank(s []byte, start int) int {
	i := start
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}
