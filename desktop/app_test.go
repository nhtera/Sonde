// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"slices"
	"testing"
)

// windowOnly are the services only the window app registers: they open
// folders, reveal and trash files, and later export and reveal secrets.
var windowOnly = []string{"workspaceDesktop", "bodiesDesktop", "copyasReveal"}

func TestServicesPerMode(t *testing.T) {
	names := func(m Mode) []string {
		var out []string
		for _, r := range registry {
			if r.modes == nil || slices.Contains(r.modes, m) {
				out = append(out, r.name)
			}
		}
		return out
	}
	for _, m := range []Mode{ModeServer, ModeHarness} {
		for _, name := range windowOnly {
			if slices.Contains(names(m), name) {
				t.Errorf("mode %d registers the window-only service %s", m, name)
			}
		}
	}
	for _, name := range windowOnly {
		if !slices.Contains(names(ModeDesktop), name) {
			t.Errorf("the window app lacks %s", name)
		}
	}
}
