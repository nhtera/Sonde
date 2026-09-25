// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"strings"
	"testing"
)

func TestParseAccepts(t *testing.T) {
	queries := []string{
		"$",
		"$.store",
		"$['store']",
		"$[\"store\"]",
		"$.store.book[0].title",
		"$.store.book[*].author",
		"$..author",
		"$.store.*",
		"$..book[2]",
		"$..book[-1]",
		"$..book[0,1]",
		"$..book[:2]",
		"$..book[1:2]",
		"$..book[::-1]",
		"$..book[?@.isbn]",
		"$..book[?@.price<10]",
		"$..book[?@.price<10 && @.category=='fiction']",
		"$..book[?@.price<10 || @.price>20]",
		"$..book[?!@.isbn]",
		"$..book[?(@.price<10)]",
		"$..*",
		"$..[0]",
		"$[?length(@.a) > 1]",
		"$[?count(@.*) == 2]",
		"$[?match(@.a, 'a.*')]",
		"$[?search(@.a, 'a')]",
		"$[?value(@.a) == 'x']",
		"$['a\\u0041b']",
		"$.☺",
		"$[1:5:2]",
		"$[::-9007199254740991]",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, err := Parse(q); err != nil {
				t.Errorf("Parse(%q): unexpected error: %v", q, err)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	queries := []string{
		"",
		"xxx",
		"$ ",
		"$.",
		"$.1",
		"$..1",
		"$..",
		"$[",
		"$['unterminated",
		"$[?@.a ==]",
		"$[?(@.a]",
		"$[9007199254740992]",
		"$[-9007199254740992]",
		"$[?@.a==1e]",
		"$[?@.a==1e-]",
		"$[?@.a==1eE1]",
		"$[?@.a==1e--1]",
		"$[?@.a==--1]",
		"$[?@.a==1.]",
		"$['bad \\x escape']",
		"$['incomplete \\u12']",
		"$[?length(@.*)]",
		"$[?count(1)]",
		"$[?match(@.a, '[')]",
		"$['a",
		"$[?@.a == 'unterminated]",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, err := Parse(q); err == nil {
				t.Errorf("Parse(%q): expected an error, got none", q)
			}
		})
	}
}

func TestParseErrorMessage(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error is %T, want *ParseError", err)
	}
	if pe.Pos != 0 {
		t.Errorf("Pos = %d, want 0", pe.Pos)
	}
	if !strings.Contains(pe.Error(), "$") {
		t.Errorf("Error() = %q, want it to mention the missing %q", pe.Error(), "$")
	}
}

func TestParseErrorPosIsRuneOffset(t *testing.T) {
	// "$['é" is 4 runes but 5 UTF-8 bytes ("é" takes 2 bytes): the
	// unterminated-string error must report the rune offset (4), not the
	// byte offset (5).
	_, err := Parse("$['é")
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error is %T, want *ParseError", err)
	}
	if pe.Pos != 4 {
		t.Errorf("Pos = %d, want 4 (rune offset after \"$['é\")", pe.Pos)
	}
}

func TestNumberLiteralClassification(t *testing.T) {
	tests := []struct {
		selector string
		field    string // "a" always holds the number
	}{
		{"$[?@.a==110]", "integer"},
		{"$[?@.a==110.0]", "float"},
		{"$[?@.a==1.1e2]", "float"},
		{"$[?@.a==99999999999999999999999999999999]", "big integer"},
	}
	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			if _, err := Parse(tt.selector); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
