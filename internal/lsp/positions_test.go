// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"testing"
	"unicode/utf8"
)

func TestLineIndex(t *testing.T) {
	text := "a😀b\r\nc\u0301d\re\n\nf"
	tests := []struct {
		offset int
		utf16  Position
		utf8   Position
	}{
		{0, Position{0, 0}, Position{0, 0}},
		{1, Position{0, 1}, Position{0, 1}},
		{5, Position{0, 3}, Position{0, 5}}, // after the emoji
		{6, Position{0, 4}, Position{0, 6}}, // before \r\n
		{8, Position{1, 0}, Position{1, 0}},
		{11, Position{1, 2}, Position{1, 3}}, // combining mark counts
		{13, Position{2, 0}, Position{2, 0}}, // after lone \r
		{15, Position{3, 0}, Position{3, 0}},
		{16, Position{4, 0}, Position{4, 0}},
		{17, Position{4, 1}, Position{4, 1}},
	}
	for _, enc := range []bool{true, false} {
		x := newLineIndex(text, enc)
		for _, tt := range tests {
			want := tt.utf8
			if enc {
				want = tt.utf16
			}
			if got := x.position(tt.offset); got != want {
				t.Errorf("utf16=%v position(%d) = %+v, want %+v", enc, tt.offset, got, want)
			}
			if got := x.offset(want); got != tt.offset {
				t.Errorf("utf16=%v offset(%+v) = %d, want %d", enc, want, got, tt.offset)
			}
		}
	}
}

func TestLineIndexClamps(t *testing.T) {
	x := newLineIndex("a😀\r\nb", true)
	for _, tt := range []struct {
		p    Position
		want int
	}{
		{Position{0, 2}, 1},  // inside the surrogate pair: back to the emoji start
		{Position{0, 99}, 5}, // past the line end: the line end, not the next line
		{Position{9, 0}, 8},  // past the last line: end of text
	} {
		if got := x.offset(tt.p); got != tt.want {
			t.Errorf("offset(%+v) = %d, want %d", tt.p, got, tt.want)
		}
	}
	// An offset inside "\r\n" or inside a character reports the line end or
	// the character start.
	if got := x.position(6); got != (Position{0, 3}) {
		t.Errorf("position(6) = %+v", got)
	}
	if got := x.position(2); got != (Position{0, 1}) {
		t.Errorf("position(2) = %+v", got)
	}
	if got := x.position(-1); got != (Position{}) {
		t.Errorf("position(-1) = %+v", got)
	}
}

func FuzzLineIndex(f *testing.F) {
	f.Add("a😀b\r\nc\u0301d\re\n\nf", 3)
	f.Fuzz(func(t *testing.T, text string, off int) {
		if !utf8.ValidString(text) {
			return
		}
		for _, enc := range []bool{true, false} {
			x := newLineIndex(text, enc)
			p := x.position(off)
			back := x.offset(p)
			if back < 0 || back > len(text) || back < len(text) && !utf8.RuneStart(text[back]) {
				t.Fatalf("offset(%+v) = %d", p, back)
			}
			if x.position(back) != p {
				t.Fatalf("position(offset(%+v)) = %+v", p, x.position(back))
			}
		}
	})
}
