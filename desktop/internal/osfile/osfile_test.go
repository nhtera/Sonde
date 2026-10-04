// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package osfile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestStartOutlivesTheCall: a started program runs to its end after
// start returns (Reveal in Finder and Open in default app once killed
// theirs at once).
func TestStartOutlivesTheCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	f := filepath.Join(t.TempDir(), "done")
	if err := start("sh", "-c", `sleep 0.2; touch "$0"`, f); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(f); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the program did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

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

// TestTrashScriptTakesNoPath: on Windows the path must never be part of
// the PowerShell command line (it would be script text).
func TestTrashScriptTakesNoPath(t *testing.T) {
	src, err := os.ReadFile("osfile.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `"-Command", script)`) || strings.Contains(string(src), `"& {"+script+"}", path)`) {
		t.Error("the trash script must read the path from the environment only")
	}
}
