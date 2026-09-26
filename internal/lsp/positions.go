// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"math"
	"sort"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/syntax"
)

// lineIndex converts between byte offsets in a document and LSP positions.
// It is the only place that knows about position encodings: lines end at
// "\n", "\r\n" or "\r" as the specification says, and characters are
// counted in UTF-16 code units or UTF-8 bytes.
type lineIndex struct {
	text   string
	utf16  bool
	starts []int // byte offset of each line start

	// A cursor remembers the last UTF-16 conversion, so converting many
	// offsets of one long line in source order stays linear.
	curLine, curOffset, curUnits int
}

func newLineIndex(text string, utf16 bool) *lineIndex {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			starts = append(starts, i+1)
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
			starts = append(starts, i+1)
		}
	}
	return &lineIndex{text: text, utf16: utf16, starts: starts, curLine: -1}
}

// lineEnd returns the offset of line's terminator (or of the end of text).
func (x *lineIndex) lineEnd(line int) int {
	end := len(x.text)
	if line+1 < len(x.starts) {
		end = x.starts[line+1]
		if end > 0 && x.text[end-1] == '\n' {
			end--
		}
		if end > x.starts[line] && x.text[end-1] == '\r' {
			end--
		}
	}
	return end
}

// position returns the LSP position of offset, clamped to the text.
func (x *lineIndex) position(offset int) Position {
	offset = max(0, min(offset, len(x.text)))
	line := sort.Search(len(x.starts), func(i int) bool { return x.starts[i] > offset }) - 1
	start := x.starts[line]
	// An offset inside a "\r\n" pair belongs to the end of its line.
	end := x.lineEnd(line)
	offset = min(offset, max(end, start))
	for offset > start && offset < len(x.text) && !utf8.RuneStart(x.text[offset]) {
		offset--
	}
	if !x.utf16 {
		return Position{Line: u32(line), Character: u32(offset - start)}
	}
	from, units := start, 0
	if x.curLine == line && x.curOffset <= offset {
		from, units = x.curOffset, x.curUnits
	}
	units += x.units(x.text[from:offset])
	x.curLine, x.curOffset, x.curUnits = line, offset, units
	return Position{Line: u32(line), Character: u32(units)}
}

// u32 converts a non-negative line or column, bounded by the 64 MiB input
// limit, to the protocol's uint32.
func u32(n int) uint32 { return uint32(min(max(n, 0), math.MaxUint32)) }

// offset returns the byte offset of p. A line past the end maps to the end
// of the text; a character past the end of its line, or in the middle of a
// character, maps to the next character boundary at or before it.
func (x *lineIndex) offset(p Position) int {
	if int(p.Line) >= len(x.starts) {
		return len(x.text)
	}
	start, end := x.starts[p.Line], x.lineEnd(int(p.Line))
	want := int(p.Character)
	n := 0
	for i, c := range x.text[start:end] {
		w := x.width(c)
		if n+w > want {
			return start + i
		}
		n += w
	}
	return end
}

// rangeOf returns the range of the byte span [from, to).
func (x *lineIndex) rangeOf(from, to int) Range {
	return Range{Start: x.position(from), End: x.position(to)}
}

// spanRange returns the range of an AST span.
func (x *lineIndex) spanRange(s syntax.Span) Range {
	return x.rangeOf(s.Start.Offset, s.End.Offset)
}

func (x *lineIndex) units(s string) int {
	if !x.utf16 {
		return len(s)
	}
	n := 0
	for _, c := range s {
		n += x.width(c)
	}
	return n
}

func (x *lineIndex) width(c rune) int {
	switch {
	case !x.utf16:
		return utf8.RuneLen(max(c, 0))
	case c >= 0x10000:
		return 2
	default:
		return 1
	}
}
