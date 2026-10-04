// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
)

func TestInstances(t *testing.T) {
	dir := t.TempDir()
	config := sandboxtest.Open(t, dir)
	a, err := lockInstance(config, 1001)
	if err != nil {
		t.Fatal(err)
	}
	b, err := lockInstance(config, 1002)
	if err != nil {
		t.Fatal(err)
	}
	if n := a.Others(); n != 1 {
		t.Fatalf("a sees %d others, want 1", n)
	}
	// A process that died without unlocking leaves a file no one holds.
	stale := filepath.Join(dir, instancesDir, "1003.lock")
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Just created, it may be a starting process's: counted, kept.
	if n := b.Others(); n != 2 {
		t.Fatalf("b sees %d others, want 2 (a, and a file being locked)", n)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if n := b.Others(); n != 1 {
		t.Fatalf("b sees %d others, want 1 (the stale file is not one)", n)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("the stale lock file stays: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if n := a.Others(); n != 0 {
		t.Fatalf("a sees %d others after b quit, want 0", n)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}
