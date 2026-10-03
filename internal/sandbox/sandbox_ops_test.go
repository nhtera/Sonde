// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func openRoot(t *testing.T) (*Root, string) {
	t.Helper()
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, dir
}

func TestWriteFileAtomic(t *testing.T) {
	r, dir := openRoot(t)
	if err := r.WriteFileAtomic("a/b/secret.env", []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFileAtomic("a/b/secret.env", []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a", "b", "secret.env"))
	if err != nil || string(got) != "two" {
		t.Fatalf("content %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, "a", "b", "secret.env"))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v, %v", fi.Mode().Perm(), err)
		}
	}
	entries, err := r.ReadDir("a/b")
	if err != nil || len(entries) != 1 {
		t.Errorf("leftover temporary files: %v %v", entries, err)
	}
}

func TestOpsStayInRoot(t *testing.T) {
	r, dir := openRoot(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	ops := map[string]func() error{
		"write ..":     func() error { return r.WriteFileAtomic("../x", nil, 0o600) },
		"write link":   func() error { return r.WriteFileAtomic("link/x", nil, 0o600) },
		"rename from":  func() error { return r.Rename("../x", "y") },
		"rename to":    func() error { return r.Rename("y", "link/y") },
		"remove":       func() error { return r.Remove("../x") },
		"readdir link": func() error { _, err := r.ReadDir("link"); return err },
		"mkdir link":   func() error { return r.MkdirAll("link/d", 0o755) },
		"stat abs":     func() error { _, err := r.Stat(outside); return err },
		"chmod link":   func() error { return r.Chmod("link", 0o600) },
	}
	if runtime.GOOS == "windows" {
		// Chmod acts on the link itself there, which is inside the root
		// (https://go.dev/issue/71492).
		delete(ops, "chmod link")
	}
	for name, op := range ops {
		f, err := os.Create(filepath.Join(dir, "y")) //nolint:gosec // G304: test file
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		if err := op(); !errors.Is(err, ErrDenied) {
			t.Errorf("%s: %v, want ErrDenied", name, err)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote outside the root: %v", entries)
	}
}

func TestOps(t *testing.T) {
	r, dir := openRoot(t)
	if err := r.MkdirAll("d/e", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.MkdirAll(".", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFileAtomic("d/f.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename("d/f.txt", "d/e/g.txt"); err != nil {
		t.Fatal(err)
	}
	fi, err := r.Stat(filepath.Join(dir, "d", "e", "g.txt")) // an absolute name inside the root
	if err != nil || fi.Size() != 1 {
		t.Fatalf("stat %v %v", fi, err)
	}
	if runtime.GOOS != "windows" {
		if err := r.Chmod("d/e/g.txt", 0o600); err != nil {
			t.Fatal(err)
		}
		if fi, _ := r.Stat("d/e/g.txt"); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", fi.Mode().Perm())
		}
	}
	entries, err := r.ReadDir("d")
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if err != nil || !slices.Equal(names, []string{"e"}) {
		t.Errorf("readdir %v %v", names, err)
	}
	if err := r.Remove("d/e/g.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Stat("d/e/g.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after remove: %v", err)
	}
	if err := r.Remove("d"); err == nil {
		t.Error("removed a non-empty directory")
	}
}

// atomicHelperEnv makes the test binary a writer that rewrites a file
// until it is killed.
const atomicHelperEnv = "SONDE_SANDBOX_ATOMIC_HELPER"

func TestAtomicWriterHelper(t *testing.T) {
	dir := os.Getenv(atomicHelperEnv)
	if dir == "" {
		t.Skip("helper process only")
	}
	r, err := Open(dir)
	if err != nil {
		os.Exit(2)
	}
	big := bytes.Repeat([]byte("n"), 16<<20)
	_ = os.WriteFile(filepath.Join(dir, "started"), nil, 0o600) //nolint:gosec // G703: the test's own temp dir
	for {
		if err := r.WriteFileAtomic("target", big, 0o600); err != nil {
			os.Exit(3)
		}
	}
}

// TestWriteFileAtomicKilled kills a process while it rewrites a file:
// the target always holds a whole version, never a truncated one.
func TestWriteFileAtomicKilled(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a process")
	}
	dir := t.TempDir()
	old := []byte("old content")
	if err := os.WriteFile(filepath.Join(dir, "target"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAtomicWriterHelper$") //nolint:gosec // G204: the test binary itself
		cmd.Env = append(os.Environ(), atomicHelperEnv+"="+dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(dir, "started")); err == nil || time.Now().After(deadline) {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(30 * time.Millisecond)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		got, err := os.ReadFile(filepath.Join(dir, "target")) //nolint:gosec // G304: test file
		if err != nil {
			t.Fatal(err)
		}
		whole := bytes.Equal(got, old) || len(got) == 16<<20 && bytes.Count(got, []byte("n")) == len(got)
		if !whole {
			t.Fatalf("target has %d bytes: a partial write", len(got))
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".sonde-tmp-") {
			t.Logf("a killed write leaves its temporary file %s (target intact)", e.Name())
		}
	}
}
