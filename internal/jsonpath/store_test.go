// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

// The "bookstore" document is the worked example from RFC 9535 §1.3,
// used throughout the spec and by many JSONPath test suites.
const storeDocument = `
{
  "book": [
    {
      "category": "reference",
      "author": "Nigel Rees",
      "title": "Sayings of the Century",
      "price": 8.95
    },
    {
      "category": "fiction",
      "author": "Evelyn Waugh",
      "title": "Sword of Honour",
      "price": 12.99
    },
    {
      "category": "fiction",
      "author": "Herman Melville",
      "title": "Moby Dick",
      "isbn": "0-553-21311-3",
      "price": 8.99
    },
    {
      "category": "fiction",
      "author": "J. R. R. Tolkien",
      "title": "The Lord of the Rings",
      "isbn": "0-395-19395-8",
      "price": 22.99
    }
  ],
  "bicycle": {
    "color": "red",
    "price": 19.95
  }
}
`

func mustDecode(t *testing.T, text string) value.Value {
	t.Helper()
	v, err := value.DecodeJSON(text)
	if err != nil {
		t.Fatalf("decoding %q: %v", text, err)
	}
	return v
}

func evalStore(t *testing.T, query string) []value.Value {
	t.Helper()
	root := mustDecode(t, storeDocument)
	q, err := Parse(query)
	if err != nil {
		t.Fatalf("parsing %q: %v", query, err)
	}
	return q.Eval(root)
}

func book(t *testing.T, i int) value.Value {
	t.Helper()
	results := evalStore(t, "$.book")
	books, ok := results[0].(value.List)
	if !ok {
		t.Fatalf("$.book did not evaluate to a list: %v", results)
	}
	return books[i]
}

func TestStoreNonEmptyResults(t *testing.T) {
	root := mustDecode(t, storeDocument)

	assertEval(t, "$", []value.Value{root})
	assertEval(t, "$['book']", []value.Value{mustGet(t, root, "book")})
	assertEval(t, "$.book", []value.Value{mustGet(t, root, "book")})
	assertEval(t, "$.book[0]", []value.Value{book(t, 0)})
	assertEval(t, "$.book[0].author", []value.Value{value.String("Nigel Rees")})
	assertEval(t, "$.*", []value.Value{mustGet(t, root, "bicycle"), mustGet(t, root, "book")})
	assertEval(t, "$.book[:2]", []value.Value{book(t, 0), book(t, 1)})
	assertEval(t, "$.book[0,1]", []value.Value{book(t, 0), book(t, 1)})
	assertEval(t, "$.book[?@.isbn]", []value.Value{book(t, 2), book(t, 3)})
	assertEval(t, "$.book[?@.price<10]", []value.Value{book(t, 0), book(t, 2)})
	assertLen(t, "$..book[?(@.author != 'Charles Dickens')]", 4)
	assertLen(t, "$..book[?(@.author != 'Nigel Rees')]", 3)
	assertEval(t, "$.book[*].author", []value.Value{
		value.String("Nigel Rees"), value.String("Evelyn Waugh"),
		value.String("Herman Melville"), value.String("J. R. R. Tolkien"),
	})
	assertEval(t, "$..author", []value.Value{
		value.String("Nigel Rees"), value.String("Evelyn Waugh"),
		value.String("Herman Melville"), value.String("J. R. R. Tolkien"),
	})
	assertEval(t, "$..book[2]", []value.Value{book(t, 2)})
	assertEval(t, "$..book[2].title", []value.Value{value.String("Moby Dick")})
	assertEval(t, "$..book[-1]", []value.Value{book(t, 3)})
	assertEval(t, "$..book[-1:].title", []value.Value{value.String("The Lord of the Rings")})
	assertEval(t, "$..book[:2]", []value.Value{book(t, 0), book(t, 1)})
	assertEval(t, "$..book[0,1]", []value.Value{book(t, 0), book(t, 1)})
	assertEval(t, "$..book[-4, -3]", []value.Value{book(t, 0), book(t, 1)})
	assertLen(t, "$..price", 5)
	assertEval(t, "$.bicycle.price", []value.Value{value.Float(19.95)})
}

func TestStoreEmptyNodelist(t *testing.T) {
	for _, query := range []string{"$.book[4]", "$.book[-5]", "$[0]", "$.book[*].not_exist"} {
		if got := evalStore(t, query); len(got) != 0 {
			t.Errorf("%s: want empty, got %v", query, got)
		}
	}
}

func mustGet(t *testing.T, v value.Value, key string) value.Value {
	t.Helper()
	obj, ok := v.(value.Object)
	if !ok {
		t.Fatalf("%v is not an object", v)
	}
	member, ok := obj.Get(key)
	if !ok {
		t.Fatalf("missing member %q", key)
	}
	return member
}

func assertEval(t *testing.T, query string, want []value.Value) {
	t.Helper()
	got := evalStore(t, query)
	if !nodeListDeepEqual(want, got) {
		t.Errorf("%s: want %v, got %v", query, want, got)
	}
}

func assertLen(t *testing.T, query string, want int) {
	t.Helper()
	if got := evalStore(t, query); len(got) != want {
		t.Errorf("%s: want %d results, got %d", query, want, len(got))
	}
}

func nodeListDeepEqual(a, b []value.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !value.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestNegativeFractionLiterals(t *testing.T) {
	doc := value.List{value.Int(-1), value.Float(-0.25), value.Float(-1.5), value.Int(3)}
	for expr, want := range map[string]int{
		"$[?@ == -1.5]": 1, "$[?@ == -0.25]": 1, "$[?@ > -1.5]": 3, "$[?@ < -0.5]": 2, "$[?@ == -1.5e0]": 1, "$[?@ == -15e-1]": 1,
	} {
		q, err := Parse(expr)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if got := len(q.Eval(doc)); got != want {
			t.Errorf("%s: %d nodes, want %d", expr, got, want)
		}
	}
}
