// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// This file has the line-oriented text analysis completion (and hover) use
// to find their context without a full parse: while an entry is being
// typed, its own AST usually does not exist yet, since internal/syntax's
// recovery gives up on a whole entry at its first error.

// bom is the UTF-8 byte order mark internal/syntax.Parse skips at the start
// of a document (its own copy is unexported).
const bom = "\xEF\xBB\xBF"

// lineText returns line's raw text, excluding its terminator. A byte order
// mark at the very start of the document is stripped from line 0, so the
// method-line heuristic below sees the same text the parser does.
func lineText(d *document, line int) string {
	if line < 0 || line >= len(d.lines.starts) {
		return ""
	}
	text := d.text[d.lines.starts[line]:d.lines.lineEnd(line)]
	if line == 0 {
		text = strings.TrimPrefix(text, bom)
	}
	return text
}

// isStatusLine reports whether trimmed (already stripped of leading
// whitespace) starts a response: "HTTP", "HTTP/1.1 200", "HTTP 200", ...
func isStatusLine(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "HTTP") {
		return false
	}
	rest := trimmed[len("HTTP"):]
	return rest == "" || rest[0] == '/' || rest[0] == ' ' || rest[0] == '\t'
}

// sectionHeaderName returns the name of a complete "[Name]" line.
func sectionHeaderName(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	end := strings.IndexByte(trimmed, ']')
	if end < 0 {
		return "", false
	}
	return trimmed[1:end], true
}

// markerKind is what precedingMarker found scanning upward from the cursor.
type markerKind int

const (
	markerNone    markerKind = iota // start of file/entry: the cursor is on the request line
	markerMethod                    // a request line, nothing structural since
	markerStatus                    // a response status line, nothing structural since
	markerSection                   // a "[Name]" section header
)

// marker is the nearest structural line above the cursor.
type marker struct {
	kind    markerKind
	section string // the section name, when kind == markerSection
}

// precedingMarker scans upward from just above line for the nearest request
// line, response status line or section header, skipping blank and comment
// lines. It bounds completion to the entry (and section) the cursor is in
// without needing that entry to have parsed.
func precedingMarker(d *document, line int) marker {
	for l := line - 1; l >= 0; l-- {
		text := lineText(d, l)
		trimmed := strings.TrimLeft(text, " \t")
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			continue
		case isStatusLine(trimmed):
			return marker{kind: markerStatus}
		case syntax.IsMethodLine(text):
			return marker{kind: markerMethod}
		case strings.HasPrefix(trimmed, "["):
			if name, ok := sectionHeaderName(trimmed); ok {
				return marker{kind: markerSection, section: name}
			}
		}
	}
	return marker{kind: markerNone}
}

// atFirstToken reports whether col is still within the first token of
// text — no space, tab or colon typed after the indentation yet — and
// returns that token's start column.
func atFirstToken(text string, col int) (start int, ok bool) {
	indent := len(text) - len(strings.TrimLeft(text, " \t"))
	if col < indent || strings.ContainsAny(text[indent:min(col, len(text))], " \t:") {
		return 0, false
	}
	return indent, true
}

// bracketContext reports whether col is inside a "[...]" that starts at the
// first non-blank character of text (a section header being typed), and
// the column right after the "[".
func bracketContext(text string, col int) (bodyStart int, ok bool) {
	indent := len(text) - len(strings.TrimLeft(text, " \t"))
	if indent >= len(text) || text[indent] != '[' {
		return 0, false
	}
	bodyStart = indent + 1
	if col < bodyStart || col > len(text) {
		return 0, false
	}
	if strings.IndexByte(text[bodyStart:col], ']') >= 0 {
		return 0, false // already closed before the cursor
	}
	return bodyStart, true
}

// isNameByte matches a {{ }} variable or function name character.
func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// isOptionNameByte matches an [Options] name character.
func isOptionNameByte(c byte) bool {
	return isNameByte(c) || c == '.'
}

// identRange returns the run of isWord bytes touching offset, bounded by
// [lo, hi). offset is clamped into [lo, hi] first: a completion/hover
// request can carry any int32 position, converted to a byte offset that a
// malformed or unusual client may still put outside the document.
func identRange(text string, offset, lo, hi int, isWord func(byte) bool) (start, end int) {
	offset = max(lo, min(offset, hi))
	start, end = offset, offset
	for start > lo && isWord(text[start-1]) {
		start--
	}
	for end < hi && isWord(text[end]) {
		end++
	}
	return start, end
}

// placeholderAt reports whether offset is inside an unterminated "{{ ... }}"
// (a template never spans a line), and the range of the variable or
// function name touching offset within it. offset is clamped to the line's
// bounds first, for the same reason as identRange.
func placeholderAt(d *document, offset int) (inside bool, nameStart, nameEnd int) {
	line := int(d.lines.position(offset).Line)
	lo, hi := d.lines.starts[line], d.lines.lineEnd(line)
	offset = max(lo, min(offset, hi))
	text := d.text
	open := strings.LastIndex(text[lo:offset], "{{")
	if open < 0 {
		return false, 0, 0
	}
	open += lo
	if strings.Contains(text[open+2:offset], "}}") {
		return false, 0, 0 // already closed before the cursor
	}
	start, end := identRange(text, offset, open+2, hi, isNameByte)
	return true, start, end
}

// segment is one whitespace-delimited token of a query/filter/predicate or
// option line. A quoted string ("...", `...`), a /regex/ literal or a
// {{ }} placeholder counts as a single segment even when it contains
// spaces, since those are how query and filter arguments are written.
type segment struct {
	text       string
	start, end int // byte offsets in the document
}

// segments tokenizes text[lo:hi), one line of the document.
func segments(text string, lo, hi int) []segment {
	var out []segment
	i := lo
	for i < hi {
		for i < hi && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
		if i >= hi {
			break
		}
		start := i
		switch {
		case text[i] == '"' || text[i] == '`':
			i = skipDelimited(text, i, hi, text[i])
		case text[i] == '/':
			i = skipDelimited(text, i, hi, '/')
		case strings.HasPrefix(text[i:hi], "{{"):
			if end := strings.Index(text[i+2:hi], "}}"); end >= 0 {
				i += 2 + end + 2
			} else {
				i = hi
			}
		default:
			for i < hi && text[i] != ' ' && text[i] != '\t' {
				i++
			}
		}
		out = append(out, segment{text: text[start:i], start: start, end: i})
	}
	return out
}

// skipDelimited advances past a delim-quoted run starting at i (delim
// itself), respecting backslash escapes. Without a closing delimiter on the
// line, it treats delim as a one-byte segment.
func skipDelimited(text string, i, hi int, delim byte) int {
	j := i + 1
	for j < hi {
		if text[j] == '\\' && j+1 < hi {
			j += 2
			continue
		}
		if text[j] == delim {
			return j + 1
		}
		j++
	}
	return i + 1
}

// segmentPosition returns the index of the segment offset falls in, or
// would extend; len(segs) once offset is past every segment.
func segmentPosition(segs []segment, offset int) int {
	for i, sg := range segs {
		if offset <= sg.end {
			return i
		}
	}
	return len(segs)
}

// replaceStart returns where a completion's TextEdit should start
// replacing text: the start of the segment at idx when the cursor is at or
// past it, otherwise offset (inserting into blank space).
func replaceStart(segs []segment, idx, offset int) int {
	if idx < len(segs) && offset >= segs[idx].start {
		return segs[idx].start
	}
	return offset
}
