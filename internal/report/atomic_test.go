// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
)

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := atomicWriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path) //nolint:gosec // G304: path built from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	assertNoStrayTempFiles(t, dir)

	// Overwriting must leave the directory clean too.
	if err := atomicWriteFile(path, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path) //nolint:gosec // G304: path built from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
	assertNoStrayTempFiles(t, dir)
}

// TestReportWritersLeaveNoTempFiles is the regression test for review
// finding #15 (2026-09-26): junit, tap, json and html must all write
// through a temp-file-then-rename, and none of them should ever leave a
// stray temp file behind on the successful path.
func TestReportWritersLeaveNoTempFiles(t *testing.T) {
	results := []*engine.UnitResult{successResult("t.hurl"), failureResult("t2.hurl")}

	t.Run("junit", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteJUnit(filepath.Join(dir, "r.xml"), results, redactTestSecret); err != nil {
			t.Fatal(err)
		}
		assertNoStrayTempFiles(t, dir)
	})
	t.Run("tap", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteTAP(filepath.Join(dir, "r.tap"), results, redactTestSecret); err != nil {
			t.Fatal(err)
		}
		assertNoStrayTempFiles(t, dir)
	})
	t.Run("json", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteJSON(dir, results, redactTestSecret); err != nil {
			t.Fatal(err)
		}
		assertNoStrayTempFiles(t, dir)
	})
	t.Run("html", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteHTML(dir, results, redactTestSecret); err != nil {
			t.Fatal(err)
		}
		assertNoStrayTempFiles(t, dir)
	})
}

// assertNoStrayTempFiles walks dir looking for a atomicWriteFile temp
// file (".tmp-*") left behind.
func assertNoStrayTempFiles(t *testing.T, dir string) {
	t.Helper()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && strings.HasPrefix(filepath.Base(path), ".tmp-") {
			t.Errorf("stray temp file left behind: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
