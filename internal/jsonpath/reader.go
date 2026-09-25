// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

// reader is a minimal rune-addressable cursor over a query string. The
// cursor position is a rune offset (not a byte offset), so it stays valid
// across multi-byte characters, which JSONPath name selectors and string
// literals may contain.
type reader struct {
	buf []rune
	pos int
}

func newReader(s string) *reader {
	return &reader{buf: []rune(s)}
}

// cursor returns the current rune offset.
func (r *reader) cursor() int { return r.pos }

// seek moves the cursor to a previously observed offset.
func (r *reader) seek(pos int) { r.pos = pos }

// isEOF reports whether every rune has been read.
func (r *reader) isEOF() bool { return r.pos >= len(r.buf) }

// peek returns the next rune without advancing the cursor.
func (r *reader) peek() (rune, bool) {
	if r.isEOF() {
		return 0, false
	}
	return r.buf[r.pos], true
}

// read returns the next rune, advancing the cursor.
func (r *reader) read() (rune, bool) {
	c, ok := r.peek()
	if ok {
		r.pos++
	}
	return c, ok
}

// readWhile consumes and returns runes while predicate holds.
func (r *reader) readWhile(predicate func(rune) bool) string {
	start := r.pos
	for !r.isEOF() && predicate(r.buf[r.pos]) {
		r.pos++
	}
	return string(r.buf[start:r.pos])
}
