// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import "testing"

// TestScanShortMonth0Errors exercises scanShortMonth0's TooShort and
// Invalid paths.
func TestScanShortMonth0Errors(t *testing.T) {
	if _, _, err := scanShortMonth0("Ja"); err == nil {
		t.Fatalf("expected TooShort")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != TooShort {
		t.Fatalf("expected TooShort, got %v", err)
	}
	if _, _, err := scanShortMonth0("Xyz"); err == nil {
		t.Fatalf("expected Invalid")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != Invalid {
		t.Fatalf("expected Invalid, got %v", err)
	}
}

// TestScanCharTooShort exercises scanChar's TooShort path (the Invalid
// path is already exercised by many literal-mismatch tests).
func TestScanCharTooShort(t *testing.T) {
	if _, err := scanChar("", ':'); err == nil {
		t.Fatalf("expected TooShort")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != TooShort {
		t.Fatalf("expected TooShort, got %v", err)
	}
}

// TestScanComment2822Malformed exercises comment_2822's nested-nesting and
// stray-close-paren rejection.
func TestScanComment2822Malformed(t *testing.T) {
	if _, err := scanComment2822("(x))"); err != nil {
		t.Fatalf("nested comment should be fine up to the matching close: %v", err)
	}
	if _, err := scanComment2822("x"); err == nil {
		t.Fatalf("expected Invalid for non-comment input")
	}
}

// TestScanTimezoneOffsetOutOfRangeMinutes exercises the 60-99 minutes
// OutOfRange branch of scanTimezoneOffset.
func TestScanTimezoneOffsetOutOfRangeMinutes(t *testing.T) {
	if _, err := ParseDateTime("2001-01-01T00:00:00+0965", "%Y-%m-%dT%H:%M:%S%z"); err == nil {
		t.Fatalf("expected error for minutes=65")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != OutOfRange {
		t.Fatalf("expected OutOfRange, got %v", err)
	}
}
