// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package regex holds the regular expression rules shared by the parser
// and evaluation: patterns must be valid for the Go engine and must not
// rely on its leniencies (a `{` that is not a counted repetition).
package regex

import (
	"regexp"
	"strings"
)

var countedRepetition = regexp.MustCompile(`^\{[0-9]+(,[0-9]*)?\}`)

// Check compiles pattern and also rejects a `{` that does not start
// a valid counted repetition (Go's engine would silently treat it as a
// literal brace). Braces inside escapes such as \p{L} or \x{4F} and inside
// character classes are not repetitions. It returns an error message or "".
func Check(pattern string) string {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i = skipEscape(pattern, i)
		case '[':
			i = skipClass(pattern, i)
		case '{':
			if !countedRepetition.MatchString(pattern[i:]) {
				return "repetition quantifier expects a valid decimal"
			}
		}
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return strings.TrimPrefix(err.Error(), "error parsing regexp: ")
	}
	return ""
}

// skipEscape returns the index of the last byte of the escape at p[i].
func skipEscape(p string, i int) int {
	if i+1 >= len(p) {
		return i
	}
	switch p[i+1] {
	case 'p', 'P', 'x':
		if i+2 < len(p) && p[i+2] == '{' {
			if j := strings.IndexByte(p[i+2:], '}'); j >= 0 {
				return i + 2 + j
			}
		}
	}
	return i + 1
}

// skipClass returns the index of the `]` closing the class opened at p[i]
// (a leading `]` is literal; `[:name:]` and nested classes are skipped).
func skipClass(p string, i int) int {
	j := i + 1
	if j < len(p) && p[j] == '^' {
		j++
	}
	if j < len(p) && p[j] == ']' {
		j++
	}
	for ; j < len(p); j++ {
		switch p[j] {
		case '\\':
			j = skipEscape(p, j)
		case '[':
			if strings.HasPrefix(p[j:], "[:") {
				if k := strings.Index(p[j+2:], ":]"); k >= 0 {
					j += 2 + k + 1
					continue
				}
			}
			j = skipClass(p, j)
		case ']':
			return j
		}
	}
	return len(p) - 1
}
