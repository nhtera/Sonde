// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const conformanceDir = "../../testdata/conformance/hurl"

// conformanceFiles lists vendored .hurl files, excluding the parser-error
// suite (those files are meant to fail).
func conformanceFiles(t testing.TB) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(conformanceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "tests_error_parser" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".hurl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 270 {
		t.Fatalf("found %d conformance files, want >= 270 (run scripts/sync-hurl-conformance.sh)", len(files))
	}
	return files
}

// expectedFailures are vendored files outside tests_error_parser that are
// meant to be rejected (their .err oracle shows the error).
var expectedFailures = map[string]ErrorKind{
	"tests_failed_not_linted/parallel_io_b.hurl": ErrInvalidUTF8,
	"tests_pty/stderr/stderr.hurl":               ErrMethod,
}

func conformanceName(path string) string {
	rel, _ := filepath.Rel(conformanceDir, path)
	return filepath.ToSlash(rel)
}

func localSyntaxFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob("../../testdata/syntax/*.hurl")
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func readFile(t testing.TB, path string) []byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestParseConformanceFiles(t *testing.T) {
	for _, path := range conformanceFiles(t) {
		_, err := Parse(path, readFile(t, path), DialectHurl)
		want, fail := expectedFailures[conformanceName(path)]
		switch {
		case fail && err == nil:
			t.Errorf("%s: parsed, want error kind %d", path, want)
		case fail && err.(*Error).Kind != want:
			t.Errorf("%s: got %v, want error kind %d", path, err, want)
		case !fail && err != nil:
			t.Errorf("%s: %v", path, err)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	files := append(conformanceFiles(t), localSyntaxFiles(t)...)
	for _, path := range files {
		if _, fail := expectedFailures[conformanceName(path)]; fail {
			continue
		}
		src := readFile(t, path)
		checkRoundTrip(t, path, src)
		// Same file with CRLF line endings (none are committed as fixtures).
		checkRoundTrip(t, path+" (crlf)", bytes.ReplaceAll(src, []byte("\n"), []byte("\r\n")))
	}
}

func checkRoundTrip(t *testing.T, name string, src []byte) {
	t.Helper()
	f, err := Parse(name, src, DialectHurl)
	if err != nil {
		if strings.HasSuffix(name, "(crlf)") {
			return // CRLF can legitimately break raw multiline or XML content
		}
		t.Errorf("%s: parse: %v", name, err)
		return
	}
	if got := Print(f); !bytes.Equal(got, src) {
		t.Errorf("%s: Print(Parse(src)) differs from src at byte %d", name, firstDiff(got, src))
	}
}

func firstDiff(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}
