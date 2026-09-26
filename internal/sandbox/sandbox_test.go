// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func newRoot(t *testing.T) (*Root, string) {
	t.Helper()
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, dir
}

func TestOpenDir(t *testing.T) {
	r, dir := newRoot(t)
	if r.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", r.Dir(), dir)
	}
}

func TestOpenRelative(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(wd, dir)
	if err != nil {
		t.Skip("temp dir not reachable relative to wd")
	}
	r, err := Open(rel)
	if err != nil {
		t.Fatalf("Open(%q): %v", rel, err)
	}
	defer r.Close()
	if r.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", r.Dir(), dir)
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	r, _ := newRoot(t)
	if err := r.WriteFile("a.txt", []byte("hello")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := r.ReadFile("a.txt")
	if err != nil || string(got) != "hello" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
}

func TestWriteFileMissingParent(t *testing.T) {
	r, dir := newRoot(t)
	if err := r.WriteFile("a/b/c.txt", []byte("data")); err == nil || errors.Is(err, ErrDenied) {
		t.Fatalf("WriteFile = %v, want a not-exist error", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Errorf("parent directory created: %v", err)
	}
}

func TestReadFileAbsoluteInside(t *testing.T) {
	r, dir := newRoot(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := r.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(got) != "hi" {
		t.Fatalf("ReadFile(abs inside) = %q, %v", got, err)
	}
}

func TestReadFileAbsoluteOutside(t *testing.T) {
	r, _ := newRoot(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := r.ReadFile(filepath.Join(other, "secret.txt"))
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("ReadFile(abs outside) err = %v, want ErrDenied", err)
	}
}

func TestReadFileDotDotEscape(t *testing.T) {
	r, _ := newRoot(t)
	for _, name := range []string{
		"..",
		"../outside.txt",
		"a/../../outside.txt",
		"a/../../../outside.txt",
	} {
		_, err := r.ReadFile(name)
		if !errors.Is(err, ErrDenied) {
			t.Errorf("ReadFile(%q) err = %v, want ErrDenied", name, err)
		}
	}
}

func TestReadFileDotDotWithinRoot(t *testing.T) {
	r, dir := newRoot(t)
	if err := os.Mkdir(filepath.Join(dir, "a"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFile("a/b.txt", []byte("b")); err != nil {
		t.Fatal(err)
	}
	// a/c/../b.txt stays inside the root even though it contains "..".
	got, err := r.ReadFile("a/c/../b.txt")
	if err != nil || string(got) != "b" {
		t.Fatalf("ReadFile(a/c/../b.txt) = %q, %v", got, err)
	}
}

func TestSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	r, dir := newRoot(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(other, "secret.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadFile("link.txt"); !errors.Is(err, ErrDenied) {
		t.Fatalf("ReadFile(symlink out) err = %v, want ErrDenied", err)
	}
	if _, err := r.Path("link.txt"); !errors.Is(err, ErrDenied) {
		t.Fatalf("Path(symlink out) err = %v, want ErrDenied", err)
	}

	// A relative symlink that walks out through ".." also escapes.
	if err := os.Symlink("../secret.txt", filepath.Join(dir, "rel-link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join(filepath.Dir(dir), "secret.txt"))
	if _, err := r.ReadFile("rel-link.txt"); !errors.Is(err, ErrDenied) {
		t.Fatalf("ReadFile(rel symlink out) err = %v, want ErrDenied", err)
	}
}

func TestSymlinkInside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	r, dir := newRoot(t)
	if err := r.WriteFile("target.txt", []byte("ok")); err != nil {
		t.Fatal(err)
	}
	// os.Root refuses to follow an absolute symlink target even when it
	// happens to resolve inside the root, so use a relative target.
	if err := os.Symlink("target.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	got, err := r.ReadFile("link.txt")
	if err != nil || string(got) != "ok" {
		t.Fatalf("ReadFile(symlink in) = %q, %v", got, err)
	}
	p, err := r.Path("link.txt")
	if err != nil {
		t.Fatalf("Path(symlink in): %v", err)
	}
	if filepath.Dir(p) != dir {
		t.Errorf("Path(symlink in) = %q, want dir %q", p, dir)
	}
}

func TestPath(t *testing.T) {
	r, dir := newRoot(t)
	p, err := r.Path("sock/a.sock")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, "sock", "a.sock")
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}

func TestPathEscape(t *testing.T) {
	r, _ := newRoot(t)
	if _, err := r.Path("../a.sock"); !errors.Is(err, ErrDenied) {
		t.Errorf("Path(..) err = %v, want ErrDenied", err)
	}
}

func TestWriteFileEscape(t *testing.T) {
	r, _ := newRoot(t)
	if err := r.WriteFile("../out.txt", []byte("x")); !errors.Is(err, ErrDenied) {
		t.Errorf("WriteFile(..) err = %v, want ErrDenied", err)
	}
}

func TestReadFileNotExist(t *testing.T) {
	r, _ := newRoot(t)
	_, err := r.ReadFile("missing.txt")
	if err == nil || errors.Is(err, ErrDenied) {
		t.Errorf("ReadFile(missing) err = %v, want plain not-exist error", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadFile(missing) err = %v, want ErrNotExist", err)
	}
}

func TestClose(t *testing.T) {
	r, _ := newRoot(t)
	if err := r.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// A relative name is normalized against the root: `../<root>/x` is inside.
func TestParentThroughRootName(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "build")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // test
	if err := r.WriteFile("../build/out.bin", []byte("x")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if b, err := r.ReadFile("../build/out.bin"); err != nil || string(b) != "x" {
		t.Fatalf("ReadFile = %q, %v", b, err)
	}
	if _, err := r.ReadFile("../other/out.bin"); !errors.Is(err, ErrDenied) {
		t.Fatalf("escape err = %v", err)
	}
}
