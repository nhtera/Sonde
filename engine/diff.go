// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import "strings"

// diffWindow bounds the lines of each text compared after their common
// prefix and suffix, so the table stays small (the first hunk starts at
// the beginning of the window).
const diffWindow = 1000

// firstHunk diffs two texts line by line and returns the first hunk (no
// context lines): each changed line prefixed with "-" (expected) or "+"
// (actual), deletions first, and the 0-based line of the expected text
// the hunk refers to.
func firstHunk(expected, actual string) (string, int) {
	a, b := splitLines(expected), splitLines(actual)
	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	end := 0
	for end < len(a)-start && end < len(b)-start && a[len(a)-1-end] == b[len(b)-1-end] {
		end++
	}
	a, b = a[start:len(a)-end], b[start:len(b)-end]
	a, b = a[:min(len(a), diffWindow)], b[:min(len(b), diffWindow)]

	// Longest common subsequence table, suffix form.
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var dels, ins []string
	for i, j := 0, 0; i < len(a) || j < len(b); {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i, j = len(a), len(b) // end of the first hunk
		case j >= len(b) || i < len(a) && lcs[i+1][j] >= lcs[i][j+1]:
			dels = append(dels, "-"+a[i])
			i++
		default:
			ins = append(ins, "+"+b[j])
			j++
		}
	}
	line := start
	if len(dels) == 0 && start > 0 {
		line = start - 1
	}
	return strings.Join(append(dels, ins...), ""), line
}

// splitLines splits text into lines keeping their line terminators.
func splitLines(s string) []string {
	var lines []string
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
	}
	return lines
}
