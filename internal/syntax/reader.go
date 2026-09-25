// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"unicode/utf8"
)

// reader walks a UTF-8 source string rune by rune within [pos.Offset, end).
// Its whole state is the current Pos, so saving and restoring a position is
// a plain copy. Offsets always refer to the full file text.
type reader struct {
	src string // full file text
	end int    // exclusive byte limit of this reader
	pos Pos

	// scratch is reused to collect characters of a string literal; only
	// for rules that parse nothing else while collecting.
	scratch []encodedChar
}

func newReader(src string) *reader {
	return &reader{src: src, end: len(src), pos: Pos{Line: 1, Col: 1}}
}

// subReader reads the file range [from.Offset, end) starting at from; used
// to re-parse a slice (placeholder content, cookie paths) in place.
func (r *reader) subReader(from Pos, end int) *reader {
	return &reader{src: r.src, end: end, pos: from}
}

func (r *reader) isEOF() bool { return r.pos.Offset >= r.end }

// read consumes and returns the next rune; ok is false at EOF.
func (r *reader) read() (c rune, ok bool) {
	if r.pos.Offset >= r.end {
		return 0, false
	}
	c, size := utf8.DecodeRuneInString(r.src[r.pos.Offset:r.end])
	r.pos.Offset += size
	if !isCombining(c) {
		r.pos.Col++
	}
	if c == '\n' {
		r.pos.Col = 1
		r.pos.Line++
	}
	return c, true
}

// peek returns the next rune without consuming it.
func (r *reader) peek() (rune, bool) {
	if r.pos.Offset >= r.end {
		return 0, false
	}
	c, _ := utf8.DecodeRuneInString(r.src[r.pos.Offset:r.end])
	return c, true
}

// peekIs reports whether the next rune is c.
func (r *reader) peekIs(c rune) bool {
	x, ok := r.peek()
	return ok && x == c
}

// peekPrefix reports whether the remaining input starts with s.
func (r *reader) peekPrefix(s string) bool {
	return strings.HasPrefix(r.src[r.pos.Offset:r.end], s)
}

// peekFirst returns the first upcoming rune satisfying pred.
func (r *reader) peekFirst(pred func(rune) bool) (rune, bool) {
	for _, c := range r.src[r.pos.Offset:r.end] {
		if pred(c) {
			return c, true
		}
	}
	return 0, false
}

// readWhile consumes runes while pred holds and returns them as a source slice.
func (r *reader) readWhile(pred func(rune) bool) string {
	start := r.pos.Offset
	for {
		c, ok := r.peek()
		if !ok || !pred(c) {
			return r.src[start:r.pos.Offset]
		}
		r.read()
	}
}

// slice returns the source between from and the current position.
func (r *reader) slice(from Pos) string { return r.src[from.Offset:r.pos.Offset] }

func isCombining(c rune) bool { return c > '̀' && c < 'ͯ' }
