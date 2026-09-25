// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"path/filepath"
	"testing"
)

// FuzzParse checks that no input panics, and that accepted input round-trips
// and formats idempotently.
func FuzzParse(f *testing.F) {
	errFiles, _ := filepath.Glob(filepath.Join(conformanceDir, "tests_error_parser", "*.hurl"))
	for _, path := range append(conformanceFiles(f), errFiles...) {
		f.Add(readFile(f, path))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		file, err := Parse("fuzz.hurl", src, DialectHurl)
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
		again, err := Parse("fuzz.hurl", formatted, DialectHurl)
		if err != nil {
			t.Fatalf("formatted output does not parse: %v\n%q", err, formatted)
		}
		if !bytes.Equal(Format(again), formatted) {
			t.Fatalf("Format is not idempotent for %q", formatted)
		}
	})
}
