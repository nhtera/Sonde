// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package styled

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	var s Text
	s.Push(" 8 | a\n", Bold|Blue)
	s.Push("   |", Bold|Blue)
	s.Push("   ", Plain)
	s.Push("-x", Red)
	s.Push("", Green)
	s.Push(" ...", Gray)
	if got, want := s.String(false), " 8 | a\n   |   -x ..."; got != want {
		t.Errorf("plain = %q, want %q", got, want)
	}
	want := "\x1b[1;34m 8 | a\n   |\x1b[0m   \x1b[31m-x\x1b[0m\x1b[90m ...\x1b[0m"
	if got := s.String(true); got != want {
		t.Errorf("ansi = %q, want %q", got, want)
	}
	if !s.HasSuffix("...") {
		t.Error("HasSuffix(...) = false")
	}
}

func TestSplit(t *testing.T) {
	var s Text
	s.Push("a\nb", Bold)
	s.Push("c\n", Red)
	parts := s.Split("\n")
	var got []string
	for _, p := range parts {
		got = append(got, p.String(true))
	}
	want := []string{"\x1b[1ma\x1b[0m", "\x1b[1mb\x1b[0m\x1b[31mc\x1b[0m", ""}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Split = %q, want %q", got, want)
	}
}
