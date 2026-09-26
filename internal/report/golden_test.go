// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// assertGolden compares got against the contents of golden, rewriting the
// file instead when -update is passed.
func assertGolden(t *testing.T, golden string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil { //nolint:gosec // G306: committed fixture, world-readable
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden) //nolint:gosec // G304: golden is a fixture path built by the test itself
	if err != nil {
		t.Fatalf("reading golden file %s: %v (run with -update to create it)", golden, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s: output differs from golden:\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}
