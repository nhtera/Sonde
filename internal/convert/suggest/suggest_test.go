// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package suggest

import "testing"

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		`  pm.expect( a . b ) .to .eql( "x . y" ) `: `pm.expect(a.b).to.eql("x . y")`,
		`var  d=pm.response.json( )`:                `var d = pm.response.json()`,
		`a === b`:                                   `a===b`,
		`f(a ,b)`:                                   `f(a, b)`,
		`if (a <= 299)`:                             `if(a <=299)`,
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
