// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import "testing"

func TestVersionOrder(t *testing.T) {
	// Ascending, semver.org's example plus the release train's shapes.
	order := []string{"0.1.0", "0.2.0-alpha", "0.2.0-alpha.1", "0.2.0-alpha.beta", "0.2.0-beta", "0.2.0-beta.2",
		"0.2.0-beta.11", "0.2.0-rc.1", "0.2.0-rc.2", "0.2.0", "0.2.1", "0.10.0", "1.0.0"}
	for i, as := range order {
		for j, bs := range order {
			a, ok1 := parseVersion(as)
			b, ok2 := parseVersion(bs)
			if !ok1 || !ok2 {
				t.Fatalf("parse %s %s", as, bs)
			}
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := a.compare(b); got != want {
				t.Errorf("compare(%s, %s) = %d, want %d", as, bs, got, want)
			}
		}
	}
	if v, _ := parseVersion("0.2.0+build.5"); v.compare(version{core: [3]int{0, 2, 0}}) != 0 {
		t.Error("build metadata changed the order")
	}
	for _, bad := range []string{"", "v0.2.0", "0.2", "01.2.3", "0.2.0-", "0.2.0 ", "dev", "0.2.0-rc..1"} {
		if _, ok := parseVersion(bad); ok {
			t.Errorf("parseVersion(%q) accepted", bad)
		}
	}
}
