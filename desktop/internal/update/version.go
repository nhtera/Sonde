// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"cmp"
	"regexp"
	"strconv"
	"strings"
)

// semverRE is a semantic version without leading v; build metadata is
// allowed and ignored.
var semverRE = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// version is a parsed semantic version.
type version struct {
	core [3]int
	pre  []string // the prerelease identifiers; none for a release
}

// parseVersion parses "1.2.3", "1.2.3-rc.1"; ok is false for anything else.
func parseVersion(s string) (version, bool) {
	m := semverRE.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	var v version
	for i := range 3 {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return version{}, false
		}
		v.core[i] = n
	}
	if m[4] != "" {
		v.pre = strings.Split(m[4], ".")
	}
	return v, true
}

// prerelease reports whether v has a prerelease part (rc, beta, dev).
func (v version) prerelease() bool { return len(v.pre) > 0 }

// compare orders versions by semver precedence: -1, 0 or 1.
func (v version) compare(o version) int {
	for i := range 3 {
		if c := cmp.Compare(v.core[i], o.core[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(v.pre) == 0 && len(o.pre) == 0:
		return 0
	case len(v.pre) == 0:
		return 1 // a release follows its prereleases
	case len(o.pre) == 0:
		return -1
	}
	for i := 0; i < len(v.pre) && i < len(o.pre); i++ {
		a, b := v.pre[i], o.pre[i]
		an, aerr := strconv.Atoi(a)
		bn, berr := strconv.Atoi(b)
		var c int
		switch {
		case aerr == nil && berr == nil:
			c = cmp.Compare(an, bn)
		case aerr == nil:
			c = -1 // numeric identifiers sort before alphanumeric ones
		case berr == nil:
			c = 1
		default:
			c = strings.Compare(a, b)
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(v.pre), len(o.pre))
}
