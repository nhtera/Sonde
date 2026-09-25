// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import "github.com/nhtera/sonde/internal/value"

// Eval evaluates the query against root, returning the selected nodes in
// document order: for a descendant segment, a node is visited before its
// children (pre-order); object members are visited in the order they are
// stored (DecodeJSON stores them sorted by key, so a query over decoded
// JSON always sees a deterministic order).
func (q *Query) Eval(root value.Value) []value.Value {
	return evalSegments(root, q.segments, root)
}

// eval evaluates a query embedded in a filter selector or function
// argument, which starts from the root ("$...") or from the current node
// ("@...").
func (fq filterQuery) eval(current, root value.Value) []value.Value {
	start := current
	if fq.absolute {
		start = root
	}
	return evalSegments(start, fq.segments, root)
}

func evalSegments(start value.Value, segments []segment, root value.Value) []value.Value {
	results := []value.Value{start}
	for _, seg := range segments {
		var next []value.Value
		for _, v := range results {
			next = append(next, evalSegment(seg, v, root)...)
		}
		results = next
	}
	return results
}

func evalSegment(seg segment, current, root value.Value) []value.Value {
	switch seg.kind {
	case segChild:
		var out []value.Value
		for _, sel := range seg.selectors {
			out = append(out, evalSelector(sel, current, root)...)
		}
		return out
	case segDescendant:
		var out []value.Value
		for _, d := range descendants(current) {
			for _, sel := range seg.selectors {
				out = append(out, evalSelector(sel, d, root)...)
			}
		}
		return out
	}
	return nil
}

// descendants returns current and every value nested inside it, in
// pre-order.
func descendants(v value.Value) []value.Value {
	nodes := []value.Value{v}
	switch vv := v.(type) {
	case value.Object:
		for _, m := range vv {
			nodes = append(nodes, descendants(m.Value)...)
		}
	case value.List:
		for _, e := range vv {
			nodes = append(nodes, descendants(e)...)
		}
	}
	return nodes
}

func evalSelector(sel selector, current, root value.Value) []value.Value {
	switch sel.kind {
	case selName:
		if obj, ok := current.(value.Object); ok {
			if v, found := obj.Get(sel.name); found {
				return []value.Value{v}
			}
		}
		return nil
	case selWildcard:
		switch v := current.(type) {
		case value.Object:
			out := make([]value.Value, len(v))
			for i, m := range v {
				out[i] = m.Value
			}
			return out
		case value.List:
			return append([]value.Value(nil), v...)
		}
		return nil
	case selIndex:
		if lst, ok := current.(value.List); ok {
			if v, found := indexInto(lst, sel.index); found {
				return []value.Value{v}
			}
		}
		return nil
	case selSlice:
		return evalSlice(sel, current)
	case selFilter:
		switch v := current.(type) {
		case value.Object:
			var out []value.Value
			for _, m := range v {
				if evalLogicalExpr(sel.filter, m.Value, root) {
					out = append(out, m.Value)
				}
			}
			return out
		case value.List:
			var out []value.Value
			for _, e := range v {
				if evalLogicalExpr(sel.filter, e, root) {
					out = append(out, e)
				}
			}
			return out
		}
		return nil
	}
	return nil
}

// indexInto resolves an index selector's value against a list, applying
// the "count from the end" rule for negative indices.
func indexInto(lst value.List, index int64) (value.Value, bool) {
	n := int64(len(lst))
	if index < -n || index >= n {
		return nil, false
	}
	if index < 0 {
		index += n
	}
	return lst[index], true
}

func evalSlice(sel selector, current value.Value) []value.Value {
	lst, ok := current.(value.List)
	if !ok || sel.sliceStep == 0 {
		return nil
	}
	n := int64(len(lst))
	lower, upper := sliceBounds(sel, n)
	var out []value.Value
	if sel.sliceStep > 0 {
		for i := lower; i < upper; i += sel.sliceStep {
			out = append(out, lst[i])
		}
	} else {
		for i := upper; i > lower; i += sel.sliceStep {
			out = append(out, lst[i])
		}
	}
	return out
}

func sliceStart(sel selector, n int64) int64 {
	if sel.sliceStart != nil {
		return *sel.sliceStart
	}
	if sel.sliceStep >= 0 {
		return 0
	}
	return n - 1
}

func sliceEnd(sel selector, n int64) int64 {
	if sel.sliceEnd != nil {
		return *sel.sliceEnd
	}
	if sel.sliceStep >= 0 {
		return n
	}
	return -n - 1
}

func sliceBounds(sel selector, n int64) (int64, int64) {
	start := normalizeIndex(sliceStart(sel, n), n)
	end := normalizeIndex(sliceEnd(sel, n), n)
	if sel.sliceStep > 0 {
		return clampInt64(start, 0, n), clampInt64(end, 0, n)
	}
	return clampInt64(end, -1, n-1), clampInt64(start, -1, n-1)
}

func normalizeIndex(i, n int64) int64 {
	if i >= 0 {
		return i
	}
	return n + i
}

func clampInt64(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
