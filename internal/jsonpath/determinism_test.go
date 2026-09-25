// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"fmt"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

// TestDeterminism guards against a query result depending on map/slice
// iteration order: object members must always be visited key-sorted (the
// order DecodeJSON stores them in), for both a direct wildcard and a
// descendant segment.
func TestDeterminism(t *testing.T) {
	doc := "{"
	for i := 0; i < 500; i++ {
		if i > 0 {
			doc += ","
		}
		doc += fmt.Sprintf("%q:{\"v\":%d,\"nested\":{\"x\":%d}}", fmt.Sprintf("key%04d", i), i, i)
	}
	doc += "}"
	root, err := value.DecodeJSON(doc)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}

	for _, query := range []string{"$.*", "$..v", "$..x"} {
		q, err := Parse(query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		first := q.Eval(root)
		for i := 0; i < 100; i++ {
			got := q.Eval(root)
			if !nodeListDeepEqual(first, got) {
				t.Fatalf("%s: iteration %d differs from the first evaluation", query, i)
			}
		}
	}
}
