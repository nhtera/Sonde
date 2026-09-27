// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/cli from the command tree")

// TestCLIReferenceUpToDate renders docs/cli from the command tree and
// compares it byte for byte with the committed pages (go test -update
// rewrites them; wired into `make docs`, alongside
// internal/docs.TestCompatUpToDate).
func TestCLIReferenceUpToDate(t *testing.T) {
	dir := filepath.Join("..", "..", "docs", "cli")
	pages := renderCLIReference(newReferenceRootCmd())

	if *update {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		existing, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		want := make(map[string]bool, len(pages))
		for _, p := range pages {
			want[p.name] = true
		}
		for _, e := range existing {
			if !e.IsDir() && !want[e.Name()] {
				if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, p := range pages {
			if err := os.WriteFile(filepath.Join(dir, p.name), []byte(p.body), 0o644); err != nil { //nolint:gosec // G306: committed docs, world-readable
				t.Fatal(err)
			}
		}
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v (run `go test ./internal/cli -run TestCLIReferenceUpToDate -update`)", dir, err)
	}
	var got []string
	for _, e := range entries {
		if !e.IsDir() {
			got = append(got, e.Name())
		}
	}
	sort.Strings(got)

	wantNames := make([]string, len(pages))
	for i, p := range pages {
		wantNames[i] = p.name
	}
	if !equalStrings(got, wantNames) {
		t.Fatalf("%s has a stale file set: has %v, want %v; run `make docs`", dir, got, wantNames)
	}

	for _, p := range pages {
		path := filepath.Join(dir, p.name)
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v (run `make docs`)", path, err)
		}
		if string(want) != p.body {
			t.Errorf("%s is stale: run `make docs` (or `go test ./internal/cli -run TestCLIReferenceUpToDate -update`) to regenerate it", path)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
