// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

// buildBenchDocument builds a roughly 1 MB nested JSON document: a flat
// array of objects, each with a nested array, to exercise the descendant
// segment's tree walk.
func buildBenchDocument(b *testing.B) value.Value {
	b.Helper()
	var sb strings.Builder
	sb.WriteByte('[')
	// Each element below is about 90 bytes; 11000 of them make ~1 MB.
	const n = 11000
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id":%d,"name":"item-%d","tags":["a","b","c"],"meta":{"ok":true}}`, i, i)
	}
	sb.WriteByte(']')
	root, err := value.DecodeJSON(sb.String())
	if err != nil {
		b.Fatalf("decoding fixture: %v", err)
	}
	return root
}

// BenchmarkDescendantSegment checks that a descendant segment over a
// ~1 MB document runs in roughly linear time in the number of nodes, not
// quadratic: it evaluates once against the full document and once
// against its first half, and the second run should take about half as
// long (checked informally, e.g. with -benchtime, rather than asserted
// here).
func BenchmarkDescendantSegment(b *testing.B) {
	root := buildBenchDocument(b)
	q, err := Parse("$..name")
	if err != nil {
		b.Fatalf("parsing: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Eval(root)
	}
}

func BenchmarkDescendantSegmentHalf(b *testing.B) {
	full := buildBenchDocument(b)
	list, ok := full.(value.List)
	if !ok {
		b.Fatalf("fixture is not a list: %T", full)
	}
	half := value.List(list[:len(list)/2])
	q, err := Parse("$..name")
	if err != nil {
		b.Fatalf("parsing: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Eval(half)
	}
}

func BenchmarkFilterSelector(b *testing.B) {
	root := buildBenchDocument(b)
	q, err := Parse("$[?@.id > 5000]")
	if err != nil {
		b.Fatalf("parsing: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Eval(root)
	}
}
