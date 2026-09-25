// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import "github.com/nhtera/sonde/internal/value"

// Unwrap applies the count rule used by queries and filters to collapse a
// node list to a single value: no nodes yields (nil, false); exactly one
// node yields that node; more than one yields them as a value.List.
func Unwrap(nodes []value.Value) (value.Value, bool) {
	switch len(nodes) {
	case 0:
		return nil, false
	case 1:
		return nodes[0], true
	default:
		return value.List(nodes), true
	}
}
