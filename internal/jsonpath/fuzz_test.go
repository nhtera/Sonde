// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

// fuzzDocument is evaluated against every query FuzzParse manages to
// parse, so a successful parse is also exercised through Eval.
const fuzzDocument = `{
  "a": [1, 2, 3, {"b": "hi", "c": null, "d": true}],
  "e": {"f": 1.5, "g": [], "h": {}},
  "i": "quoted \"string\" with \\ and /",
  "j": -0.0001,
  "k": 99999999999999999999999999999999
}`

func FuzzParse(f *testing.F) {
	seeds := []string{
		"$",
		"$.a",
		"$.a[0]",
		"$.a[-1]",
		"$['a']",
		"$..a",
		"$.*",
		"$..*",
		"$.a[0:2]",
		"$.a[0:2:1]",
		"$.a[::-1]",
		"$.a[?@.b]",
		"$.a[?@.b == 'hi']",
		"$.a[?@.b != 'hi' && @.c]",
		"$.a[?@.b || @.c]",
		"$.a[?!@.b]",
		"$.a[?length(@.b) > 1]",
		"$.a[?count(@.*) > 1]",
		"$.a[?match(@.b, 'h.')]",
		"$.a[?search(@.b, 'h')]",
		"$.a[?value(@.b) == 'hi']",
		"$['a','e']",
		"$.a[1,2]",
		"$..['b','c']",
		"$.i",
		"$.j",
		"$.k",
		"",
		"$.",
		"$[",
		"$['unterminated",
		"$.a[?@ == ]",
		"$.a[1e400]",
		"$.a[?@.b == \\u0041]",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	root, err := value.DecodeJSON(fuzzDocument)
	if err != nil {
		f.Fatalf("decoding fixture: %v", err)
	}

	f.Fuzz(func(_ *testing.T, query string) {
		q, err := Parse(query)
		if err != nil {
			return
		}
		// A query that parses must evaluate without panicking, on any
		// document shape (including one deliberately unrelated to the
		// selectors the query happens to use).
		_ = q.Eval(root)
		_ = q.Eval(value.Null{})
		_ = q.Eval(value.List{value.Int(1), value.Int(2)})
	})
}
