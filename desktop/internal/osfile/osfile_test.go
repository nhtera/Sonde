// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package osfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrashFreedesktop(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	dir := t.TempDir()
	for range 2 {
		f := filepath.Join(dir, "a b%.hurl")
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := trashFreedesktop(f); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatal("file still there")
		}
	}
	for _, name := range []string{"a b%.hurl", "a b% 2.hurl"} {
		if _, err := os.Stat(filepath.Join(data, "Trash", "files", name)); err != nil {
			t.Errorf("trashed %s: %v", name, err)
		}
		info, err := os.ReadFile(filepath.Join(data, "Trash", "info", name+".trashinfo"))
		if err != nil || !strings.Contains(string(info), "a%20b%25.hurl") {
			t.Errorf("trashinfo %s: %q %v", name, info, err)
		}
	}
}

func TestMoveInto(t *testing.T) {
	trash, dir := t.TempDir(), t.TempDir()
	f := filepath.Join(dir, "x.hurl")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := moveInto(trash, f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(trash, "x.hurl")); err != nil {
		t.Fatal(err)
	}
}
