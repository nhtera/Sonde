// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import "testing"

// TestErrorKindMessages checks Error's Display text matches chrono's
// ParseError Display for every kind.
func TestErrorKindMessages(t *testing.T) {
	cases := map[ErrorKind]string{
		OutOfRange: "input is out of range",
		Impossible: "no possible date and time matching input",
		NotEnough:  "input is not enough for unique date and time",
		Invalid:    "input contains invalid characters",
		TooShort:   "premature end of input",
		TooLong:    "trailing input",
		BadFormat:  "bad or unsupported format string",
	}
	for kind, want := range cases {
		e := newError(kind)
		if e.Error() != want {
			t.Errorf("kind %v: Error() = %q, want %q", kind, e.Error(), want)
		}
		if e.Kind() != kind {
			t.Errorf("Kind() = %v, want %v", e.Kind(), kind)
		}
	}
}

// TestErrorKindStringUnknown exercises ErrorKind.String's default branch.
func TestErrorKindStringUnknown(t *testing.T) {
	var k ErrorKind = 99
	if k.String() != "unknown parse error" {
		t.Fatalf("got %q", k.String())
	}
}
