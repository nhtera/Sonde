// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

func setupGlobTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := []string{
		"a.hurl",
		"b.hurl",
		"sub/c.hurl",
		"sub/nested/d.hurl",
		"other/e.txt",
	}
	for _, f := range files {
		full := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("GET http://a\nHTTP 200\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGlobFiles(t *testing.T) {
	dir := setupGlobTree(t)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	tests := []struct {
		pattern string
		want    []string
	}{
		{"*.hurl", []string{"a.hurl", "b.hurl"}},
		{"./*.hurl", []string{"a.hurl", "b.hurl"}},
		{"sub/*.hurl", []string{filepath.Join("sub", "c.hurl")}},
		{"**/*.hurl", []string{
			"a.hurl", "b.hurl",
			filepath.Join("sub", "c.hurl"),
			filepath.Join("sub", "nested", "d.hurl"),
		}},
		{"other/*.hurl", nil},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got, err := globFiles(tt.pattern)
			if err != nil {
				t.Fatalf("globFiles(%q): %v", tt.pattern, err)
			}
			sort.Strings(got)
			want := slices.Clone(tt.want)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Fatalf("globFiles(%q) = %v, want %v", tt.pattern, got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("globFiles(%q)[%d] = %q, want %q", tt.pattern, i, got[i], want[i])
				}
			}
		})
	}
}

func TestGlobFilesInvalidPattern(t *testing.T) {
	if _, err := globFiles("["); err == nil {
		t.Fatal("expected an error for a malformed glob pattern")
	}
}

// TestGlobFilesDotDot checks a pattern starting with ".." (nothing for
// filepath.Clean to cancel it against, unlike "sub/../*.hurl"): it must
// walk up a directory literally, not be matched against directory entries
// (which never include "." or "..").
func TestGlobFilesDotDot(t *testing.T) {
	dir := setupGlobTree(t)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	got, err := globFiles("../*.hurl")
	if err != nil {
		t.Fatalf("globFiles(%q): %v", "../*.hurl", err)
	}
	want := []string{filepath.Join("..", "a.hurl"), filepath.Join("..", "b.hurl")}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("globFiles(%q) = %v, want %v", "../*.hurl", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("globFiles(%q)[%d] = %q, want %q", "../*.hurl", i, got[i], want[i])
		}
	}
}
