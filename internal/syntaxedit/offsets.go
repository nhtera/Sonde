// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxedit

import (
	"fmt"
	"sort"
	"unicode/utf8"
)

// checkpointEvery is the byte distance between two UTF-16 checkpoints of
// an offsets table: a conversion scans at most that many bytes.
const checkpointEvery = 4096

// offsets converts between byte offsets and UTF-16 code unit offsets of a
// source, the unit every offset of this package's API uses (as the
// language server's positions do).
type offsets struct {
	src []byte
	// u16[i] is the UTF-16 offset of byte i*checkpointEvery (a rune
	// boundary at or after it: see at).
	bytes []int
	u16   []int
}

func newOffsets(src []byte) *offsets {
	o := &offsets{src: src}
	b, u := 0, 0
	for b < len(src) {
		if b >= len(o.bytes)*checkpointEvery {
			o.bytes = append(o.bytes, b)
			o.u16 = append(o.u16, u)
		}
		r, size := utf8.DecodeRune(src[b:])
		b += size
		u += utf16Len(r)
	}
	o.bytes = append(o.bytes, len(src))
	o.u16 = append(o.u16, u)
	return o
}

// utf16Len is the number of UTF-16 code units of r (an invalid byte
// counts as one, as U+FFFD).
func utf16Len(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// toU16 converts a byte offset (a rune boundary) to UTF-16 code units.
func (o *offsets) toU16(b int) int {
	i := sort.Search(len(o.bytes), func(i int) bool { return o.bytes[i] > b }) - 1
	if i < 0 {
		i = 0
	}
	pos, u := o.bytes[i], o.u16[i]
	for pos < b && pos < len(o.src) {
		r, size := utf8.DecodeRune(o.src[pos:])
		pos += size
		u += utf16Len(r)
	}
	return u
}

// toByte converts a UTF-16 offset to a byte offset; an offset inside a
// surrogate pair or past the end is an error.
func (o *offsets) toByte(u int) (int, error) {
	if u < 0 || u > o.u16[len(o.u16)-1] {
		return 0, fmt.Errorf("syntaxedit: offset %d out of range", u)
	}
	i := sort.Search(len(o.u16), func(i int) bool { return o.u16[i] > u }) - 1
	if i < 0 {
		i = 0
	}
	pos, cur := o.bytes[i], o.u16[i]
	for cur < u {
		r, size := utf8.DecodeRune(o.src[pos:])
		pos += size
		cur += utf16Len(r)
	}
	if cur != u {
		return 0, fmt.Errorf("syntaxedit: offset %d splits a surrogate pair", u)
	}
	return pos, nil
}

// rng converts a byte range to a UTF-16 Range.
func (o *offsets) rng(start, end int) Range { return Range{Start: o.toU16(start), End: o.toU16(end)} }
