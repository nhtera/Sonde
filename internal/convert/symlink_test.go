// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteConfinedThroughSymlink checks that a symlink inside the output
// directory pointing outside it cannot be used to escape a write.
func TestWriteConfinedThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "out")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}

	// os.Root rejects a symlink that leaves the root outright, so the write
	// fails rather than silently landing outside dir.
	out := Output{Files: []GeneratedFile{{Path: "escape/a", File: mustFile(t, "https://example.com")}}}
	if _, err := Write(dir, out, Options{}); err == nil {
		t.Fatal("want an error: the symlink leaves dir")
	}
	if _, err := os.Stat(filepath.Join(outside, "a.hurl")); err == nil {
		t.Fatal("write escaped through the symlink into outside/")
	}
}
