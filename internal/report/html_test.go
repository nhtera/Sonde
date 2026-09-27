// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/runerr"
)

// TestWriteHTML_Cumulative checks a report grown over two invocations
// ends up listing every result, and that every result got its own,
// distinctly named, per-unit page.
func TestWriteHTML_Cumulative(t *testing.T) {
	dir := t.TempDir()

	run1 := []*engine.UnitResult{
		successResult("tests/test.1.hurl"),
		failureResult("tests/test.2.hurl"),
	}
	if err := WriteHTML(dir, run1, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	run2 := []*engine.UnitResult{successResult("tests/test.3.hurl")}
	if err := WriteHTML(dir, run2, redactTestSecret); err != nil {
		t.Fatal(err)
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tests/test.1.hurl", "tests/test.2.hurl", "tests/test.3.hurl"} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index.html missing %q:\n%s", want, index)
		}
	}

	manifest, err := readHTMLManifest(filepath.Join(dir, ".manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 3 {
		t.Fatalf("got %d manifest entries, want 3", len(manifest))
	}
	for _, m := range manifest {
		page := filepath.Join(dir, "store", m.ID+".html")
		if _, err := os.Stat(page); err != nil {
			t.Errorf("missing per-unit page for %s: %v", m.ID, err)
		}
	}
}

// TestWriteHTML_StoreIsFlat reproduces what
// testdata/conformance/hurl's tests_ok/secret{,_file}.sh actually need:
// they locate report pages with the shell globs "report-html/*.html" and
// "report-html/**/*.html", and without bash's globstar shopt, "**" only
// ever matches one extra path segment — so store/ must hold plain files
// directly, never a per-unit subdirectory (which "**" would not reach,
// and which make those scripts' own `find` call hard-error and abort
// under `set -e` before the secret-leak check ever runs).
func TestWriteHTML_StoreIsFlat(t *testing.T) {
	dir := t.TempDir()
	results := []*engine.UnitResult{successResult("t.hurl"), failureResult("t2.hurl")}
	if err := WriteHTML(dir, results, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(results) {
		t.Fatalf("got %d entries under store/, want %d", len(entries), len(results))
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("store/%s is a directory, want a flat .html file", e.Name())
		}
		if !strings.HasSuffix(e.Name(), ".html") {
			t.Errorf("store/%s does not have a .html extension", e.Name())
		}
	}
}

// TestWriteHTML_SameFileTwice checks re-running the same file gets a
// second, distinctly named, page instead of overwriting the first
// (architecture.md: "per-unit files uniquely named").
func TestWriteHTML_SameFileTwice(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHTML(dir, []*engine.UnitResult{successResult("t.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	if err := WriteHTML(dir, []*engine.UnitResult{failureResult("t.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	manifest, err := readHTMLManifest(filepath.Join(dir, ".manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 2 {
		t.Fatalf("got %d manifest entries, want 2", len(manifest))
	}
	if manifest[0].ID == manifest[1].ID {
		t.Errorf("both runs of t.hurl got the same id %q", manifest[0].ID)
	}
}

// TestWriteHTML_CaseInsensitiveIDs is the regression test for review
// finding #14 (2026-09-26): "A.hurl" and "a.hurl" produced ids differing
// only in case, which collide on a case-insensitive filesystem (APFS and
// NTFS, both by default) — the second write would silently overwrite the
// first's page.
func TestWriteHTML_CaseInsensitiveIDs(t *testing.T) {
	dir := t.TempDir()
	results := []*engine.UnitResult{successResult("A.hurl"), failureResult("a.hurl")}
	if err := WriteHTML(dir, results, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	manifest, err := readHTMLManifest(filepath.Join(dir, ".manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 2 {
		t.Fatalf("got %d manifest entries, want 2", len(manifest))
	}
	if strings.EqualFold(manifest[0].ID, manifest[1].ID) {
		t.Errorf("ids %q and %q collide case-insensitively", manifest[0].ID, manifest[1].ID)
	}
	for _, m := range manifest {
		if _, err := os.Stat(filepath.Join(dir, "store", m.ID+".html")); err != nil {
			t.Errorf("missing per-unit page for %s: %v", m.ID, err)
		}
	}
}

// TestWriteHTML_NoScriptInjection is the Go equivalent of
// testdata/conformance/hurl/tests_failed/html_report_injection: a
// response body containing a literal "<script>" tag must never survive
// unescaped into any file under the report (it reaches the page only
// through the assert failure message quoting the actual body).
func TestWriteHTML_NoScriptInjection(t *testing.T) {
	dir := t.TempDir()
	res := failureResult("html_report_injection.hurl")
	res.Entries[0].Calls[0].Response.Body = []byte("<script>alert('Hi')</script>")
	err := runerr.New(span(4, 1, 14), runerr.AssertBodyValue, true)
	err.Actual, err.Expected = "<script>alert('Hi')</script>", "Hello World"
	e := runErr(err, res.File, string(res.Source))
	res.Entries[0].Errors[0], res.Entries[0].Asserts[0].Err = e, e

	if err := WriteHTML(dir, []*engine.UnitResult{res}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	assertNoUnescapedScript(t, dir)
}

// TestWriteHTML_NoScriptInjectionInBody is TestWriteHTML_NoScriptInjection's
// counterpart for the response-body preview specifically: a passing call
// (no assert failure quoting the body) whose body preview is the payload's
// only way onto the page.
func TestWriteHTML_NoScriptInjectionInBody(t *testing.T) {
	dir := t.TempDir()
	res := successResult("inline_script.hurl")
	res.Entries[0].Calls[0].Response.Body = []byte("<script>alert('Hi')</script>")
	res.Entries[0].Calls[0].Response.Headers = nil // no Content-Type: shown as plain text

	if err := WriteHTML(dir, []*engine.UnitResult{res}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	assertNoUnescapedScript(t, dir)
}

func assertNoUnescapedScript(t *testing.T, dir string) {
	t.Helper()
	err := filepath.Walk(filepath.Join(dir, "store"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: path from filepath.Walk over a t.TempDir()
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "<script>") {
			t.Errorf("unescaped <script> in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWriteHTML_ParseError checks a file that never parsed still gets a
// page, showing the parse error instead of entries.
func TestWriteHTML_ParseError(t *testing.T) {
	dir := t.TempDir()
	res := parseErrorResult("bad.hurl")
	if err := WriteHTML(dir, []*engine.UnitResult{res}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	manifest, err := readHTMLManifest(filepath.Join(dir, ".manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 {
		t.Fatalf("got %d manifest entries, want 1", len(manifest))
	}
	page, err := os.ReadFile(filepath.Join(dir, "store", manifest[0].ID+".html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "Parse error") {
		t.Errorf("expected a parse error section, got:\n%s", page)
	}
}

// TestWriteHTML_Redacts checks the run's secret never survives into the
// index or any per-unit page.
func TestWriteHTML_Redacts(t *testing.T) {
	dir := t.TempDir()
	results := []*engine.UnitResult{successResult("t.hurl"), failureResult("t2.hurl"), errorResult("t3.hurl")}
	if err := WriteHTML(dir, results, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: path from filepath.Walk over a t.TempDir()
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), testSecret) {
			t.Errorf("secret leaked into %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
