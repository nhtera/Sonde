// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package regex

import "testing"

func TestCheck(t *testing.T) {
	for p, bad := range map[string]bool{
		`a{2}`: false, `a{2,}`: false, `a{2,3}`: false, `\p{L}`: false, `\x{4F}`: false,
		`[{]`: false, `[[:alpha:]{]`: false, `\{`: false, `[]{]`: false,
		`hi{`: true, `a{b}`: true, `{`: true, `a{,3}`: true, `(`: true,
	} {
		if got := Check(p) != ""; got != bad {
			t.Errorf("Check(%q) = %q", p, Check(p))
		}
	}
}
