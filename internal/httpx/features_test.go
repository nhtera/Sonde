// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"slices"
	"testing"
)

func TestFeaturesMatchSupportedOptions(t *testing.T) {
	got := Features()
	for _, present := range []string{"HTTP2", "NTLM", "SPNEGO"} {
		if !slices.Contains(got, present) {
			t.Errorf("Features() = %v, want %s", got, present)
		}
	}
	// Not implemented yet. A script probing for HTTP3 (e.g. the upstream
	// http_version_not_supported test) relies on its absence; update this
	// when a feature lands.
	for _, absent := range []string{"HTTP3"} {
		if slices.Contains(got, absent) {
			t.Errorf("Features() = %v lists %s, which is not implemented", got, absent)
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
