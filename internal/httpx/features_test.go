// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"slices"
	"testing"
)

func TestFeaturesMatchSupportedOptions(t *testing.T) {
	got := Features()
	for _, present := range []string{"HTTP2", "HTTP3", "NTLM", "SPNEGO"} {
		if !slices.Contains(got, present) {
			t.Errorf("Features() = %v, want %s", got, present)
		}
	}
	// Each listed feature is accepted, each omitted one refused.
	for _, p := range featureProbes {
		opts := p.opts
		supported := checkSupported(&opts) == nil
		if listed := slices.Contains(got, p.name); listed != supported {
			t.Errorf("%s: listed = %v, supported = %v", p.name, listed, supported)
		}
	}
}
