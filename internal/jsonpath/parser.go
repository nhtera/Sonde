// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import "fmt"

// ParseError reports a query that does not match the JSONPath grammar
// (RFC 9535). Pos is a rune offset into the query string.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("jsonpath: at rune %d: %s", e.Pos, e.Msg)
}

func newParseError(pos int, msg string) *ParseError {
	return &ParseError{Pos: pos, Msg: msg}
}

// Query is a parsed JSONPath query, ready to be evaluated with Eval.
type Query struct {
	segments []segment
}

// Parse parses a JSONPath query (RFC 9535). The query must start with the
// root identifier "$" and consume the whole input.
func Parse(expr string) (*Query, error) {
	r := newReader(expr)
	if err := expectStr("$", r); err != nil {
		return nil, err
	}
	segments, err := parseSegments(r)
	if err != nil {
		return nil, err
	}
	if !r.isEOF() {
		return nil, newParseError(r.cursor(), "expecting end of query")
	}
	return &Query{segments: segments}, nil
}

// expectStr consumes s from the current position, or fails without
// consuming anything past the first mismatch (the cursor is left wherever
// reading stopped; callers that need to backtrack use matchStr instead).
func expectStr(s string, r *reader) error {
	start := r.cursor()
	for _, want := range s {
		got, ok := r.read()
		if !ok || got != want {
			return newParseError(start, "expecting "+quoteStr(s))
		}
	}
	return nil
}

// matchStr tries to consume s, restoring the cursor on failure.
func matchStr(s string, r *reader) bool {
	start := r.cursor()
	if err := expectStr(s, r); err != nil {
		r.seek(start)
		return false
	}
	return true
}

// skipWhitespace consumes zero or more of the four JSONPath blank
// characters (space, tab, newline, carriage return).
func skipWhitespace(r *reader) {
	for {
		c, ok := r.peek()
		if !ok || (c != ' ' && c != '\t' && c != '\n' && c != '\r') {
			return
		}
		r.read()
	}
}

func quoteStr(s string) string {
	return "\"" + s + "\""
}
