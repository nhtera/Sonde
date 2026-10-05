// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build e2eharness

package host

import (
	"slices"
	"testing"
)

// TestHarnessRegistersUpdate: the harness's update service is the window
// app's own Service (the exception TestServicesPerMode makes for it).
func TestHarnessRegistersUpdate(t *testing.T) {
	var harness []string
	for _, r := range registry {
		if slices.Contains(r.modes, ModeHarness) {
			harness = append(harness, r.name)
		}
	}
	if !slices.Contains(harness, "update") {
		t.Fatalf("the harness registers %v, not update", harness)
	}
}
