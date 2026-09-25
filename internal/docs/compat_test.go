// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package docs

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/compat.md from internal/docs/table.yaml")

// TestCompatUpToDate renders docs/compat.md from table.yaml and compares it
// byte for byte with the committed file (go test -update rewrites it; wired
// into `make docs`).
func TestCompatUpToDate(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderCompat(tbl)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join("..", "..", "docs", "compat.md")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil { //nolint:gosec // G306: committed doc, world-readable
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run `go test ./internal/docs -run TestCompatUpToDate -update`)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale: run `make docs` (or `go test ./internal/docs -run TestCompatUpToDate -update`) to regenerate it", path)
	}
}
