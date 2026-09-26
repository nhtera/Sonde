// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"strings"
	"testing"
)

func TestFirstHunk(t *testing.T) {
	tests := []struct {
		name, expected, actual, hunk string
		line                         int
	}{
		{"changed line", "a\nb\nc\n", "a\nB\nc\n", "-b\n+B\n", 1},
		{"first line", "a\nb\n", "x\nb\n", "-a\n+x\n", 0},
		{"deleted line", "a\nb\nc\n", "a\nc\n", "-b\n", 1},
		{"inserted line", "a\nc\n", "a\nb\nc\n", "+b\n", 0},
		{"inserted first", "b\n", "a\nb\n", "+a\n", 0},
		{"appended", "a\n", "a\nb\n", "+b\n", 0},
		{"two hunks", "a\nb\nc\nd\n", "a\nX\nc\nY\n", "-b\n+X\n", 1},
		{"no final newline", "a\nb", "a\nc", "-b+c", 1},
		{"block", "{\n  \"a\": 1,\n  \"b\": 2\n}\n", "{\n  \"a\": 1,\n  \"b\": 3\n}\n", "-  \"b\": 2\n+  \"b\": 3\n", 2},
	}
	for _, tt := range tests {
		hunk, line := firstHunk(tt.expected, tt.actual)
		if hunk != tt.hunk || line != tt.line {
			t.Errorf("%s: firstHunk = %q, %d; want %q, %d", tt.name, hunk, line, tt.hunk, tt.line)
		}
	}
}

// Large texts are diffed in bounded memory.
func TestFirstHunkLarge(t *testing.T) {
	expected := strings.Repeat("a\n", 5000)
	actual := "b\n" + strings.Repeat("c\n", 200000)
	hunk, line := firstHunk(expected, actual)
	if line != 0 || !strings.HasPrefix(hunk, "-a\n") || !strings.Contains(hunk, "+b\n") {
		t.Errorf("firstHunk = %.40q…, %d", hunk, line)
	}
}
