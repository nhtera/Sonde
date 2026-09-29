// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package appdirs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenCreatesPrivateDirs(t *testing.T) {
	base := t.TempDir()
	d, err := Open(filepath.Join(base, "config", "Sonde"), filepath.Join(base, "cache", "Sonde"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, dir := range []string{d.Config().Dir(), d.Cache().Dir()} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, want 0700", dir, fi.Mode().Perm())
		}
	}
	if err := d.Config().WriteFileAtomic(SettingsFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "config", "Sonde", SettingsFile)); err != nil {
		t.Fatal(err)
	}
}

func TestOpenNarrowsExistingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX permission bits")
	}
	base := t.TempDir()
	config := filepath.Join(base, "config")
	if err := os.Mkdir(config, 0o755); err != nil { //nolint:gosec // a directory others can read, to narrow
		t.Fatal(err)
	}
	if err := os.Chmod(config, 0o755); err != nil { //nolint:gosec // past the umask
		t.Fatal(err)
	}
	d, err := Open(config, filepath.Join(base, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	fi, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("mode %v, want 0700", fi.Mode().Perm())
	}
}

func TestOpenRejectsEmpty(t *testing.T) {
	if _, err := Open("", t.TempDir()); err == nil {
		t.Fatal("want an error for an empty directory")
	}
}
