// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package styled builds text made of styled runs, printed either plain or
// with ANSI escape codes.
package styled

import "strings"

// Style is a set of text attributes: at most one color, optionally bold.
type Style uint8

// Styles.
const (
	Plain Style = 0
	Bold  Style = 1 << iota
	Red
	Green
	Blue
	Gray
)

// ansi returns the escape sequence that starts s, or "" for Plain.
func (s Style) ansi() string {
	var codes []string
	if s&Bold != 0 {
		codes = append(codes, "1")
	}
	switch {
	case s&Red != 0:
		codes = append(codes, "31")
	case s&Green != 0:
		codes = append(codes, "32")
	case s&Blue != 0:
		codes = append(codes, "34")
	case s&Gray != 0:
		codes = append(codes, "90")
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

type run struct {
	text  string
	style Style
}

// Text is a sequence of styled runs; adjacent runs of the same style are
// merged, so a single escape sequence covers them.
type Text struct{ runs []run }

// Push appends s with style st; an empty s is ignored.
func (t *Text) Push(s string, st Style) {
	if s == "" {
		return
	}
	if n := len(t.runs); n > 0 && t.runs[n-1].style == st {
		t.runs[n-1].text += s
		return
	}
	t.runs = append(t.runs, run{s, st})
}

// Append appends the runs of o.
func (t *Text) Append(o Text) {
	for _, r := range o.runs {
		t.Push(r.text, r.style)
	}
}

// Split cuts t around each sep, keeping the styles; like strings.Split,
// n separators give n+1 parts.
func (t Text) Split(sep string) []Text {
	parts := []Text{{}}
	for _, r := range t.runs {
		for i, s := range strings.Split(r.text, sep) {
			if i > 0 {
				parts = append(parts, Text{})
			}
			parts[len(parts)-1].Push(s, r.style)
		}
	}
	return parts
}

// HasSuffix reports whether the unstyled text ends with s.
func (t Text) HasSuffix(s string) bool { return strings.HasSuffix(t.String(false), s) }

// String returns the text, with ANSI escape codes when color is true.
func (t Text) String(color bool) string {
	var b strings.Builder
	for _, r := range t.runs {
		esc := ""
		if color {
			esc = r.style.ansi()
		}
		if esc == "" {
			b.WriteString(r.text)
			continue
		}
		b.WriteString(esc + r.text + "\x1b[0m")
	}
	return b.String()
}
