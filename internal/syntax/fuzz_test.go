// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"path/filepath"
	"testing"
)

// FuzzParse checks that no input panics, and that accepted input round-trips
// and formats idempotently, in both dialects.
func FuzzParse(f *testing.F) {
	errFiles, _ := filepath.Glob(filepath.Join(conformanceDir, "tests_error_parser", "*.hurl"))
	for _, path := range append(conformanceFiles(f), errFiles...) {
		f.Add(readFile(f, path))
	}
	f.Add([]byte(sondeStreams))
	f.Fuzz(func(t *testing.T, src []byte) {
		for _, d := range []Dialect{DialectHurl, DialectSonde} {
			fuzzParse(t, src, d)
		}
	})
}

func fuzzParse(t *testing.T, src []byte, d Dialect) {
	file, err := Parse("fuzz", src, d)
	if err != nil {
		if _, ok := err.(*Error); !ok {
			t.Fatalf("error of type %T, want *Error", err)
		}
		return
	}
	if got := Print(file); !bytes.Equal(got, src) {
		t.Fatalf("round trip differs at byte %d", firstDiff(got, src))
	}
	formatted := Format(file)
	again, err := Parse("fuzz", formatted, d)
	if err != nil {
		t.Fatalf("formatted output does not parse: %v\n%q", err, formatted)
	}
	if !bytes.Equal(Format(again), formatted) {
		t.Fatalf("Format is not idempotent for %q", formatted)
	}
}
