// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package value

import "testing"

// Comparing huge exponents must not build huge exact numbers.
func BenchmarkCompareHugeExponent(b *testing.B) {
	big := BigInt("1e+999999")
	for b.Loop() {
		if Equal(big, Int(1)) {
			b.Fatal("equal")
		}
	}
}
