// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package credential

import "testing"

func TestLikely(t *testing.T) {
	for _, c := range []struct {
		name, value string
		want        bool
	}{
		{"api_token", "x", true},
		{"SessionID", "x", true},
		{"region", "eu", false},
		{"user_id", "42", false},
		{"id", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig", true},
		{"id", "a.b.c", false}, // too short for a JWT
	} {
		if got := Likely(c.name, c.value); got != c.want {
			t.Errorf("Likely(%q, %q) = %v", c.name, c.value, got)
		}
	}
}

func TestMaskable(t *testing.T) {
	if Maskable("7") || Maskable("abcdefg") || !Maskable("abcdefgh") || !Maskable("ééééééééé") {
		t.Error("Maskable counts runes from 8")
	}
}
