// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2EReportFlagsWriteFiles checks that each of the four --report-*
// flags is wired to its internal/report writer: the report file (or, for
// --report-html/--report-json, the directory) exists and is non-empty
// after the run.
func TestE2EReportFlagsWriteFiles(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	dir := t.TempDir()
	junit := filepath.Join(dir, "junit.xml")
	tap := filepath.Join(dir, "report.tap")
	jsonDir := filepath.Join(dir, "json")
	htmlDir := filepath.Join(dir, "html")

	code, _, errOut := runArgs(t, file,
		"--report-junit", junit,
		"--report-tap", tap,
		"--report-json", jsonDir,
		"--report-html", htmlDir,
	)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}

	for _, path := range []string{junit, tap} {
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", path)
		}
	}
	for _, dir := range []string{jsonDir, htmlDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		if len(entries) == 0 {
			t.Errorf("%s has no files", dir)
		}
	}
}

// TestE2EReportsCumulativeAcrossInvocations checks that a report file
// accumulates results across separate process invocations, matching
// tests_ok/junit's conformance fixture: report.WriteJUnit reads and merges
// any content already at the path rather than truncating it.
func TestE2EReportsCumulativeAcrossInvocations(t *testing.T) {
	srv := testServer(t)
	dir := t.TempDir()
	junit := filepath.Join(dir, "junit.xml")
	first := writeTemp(t, "first.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	second := writeTemp(t, "second.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")

	if code, _, errOut := runArgs(t, first, "--report-junit", junit); code != ExitOK {
		t.Fatalf("first run: exit code = %d; stderr=%s", code, errOut)
	}
	firstSize, err := os.ReadFile(junit)
	if err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runArgs(t, second, "--report-junit", junit); code != ExitOK {
		t.Fatalf("second run: exit code = %d; stderr=%s", code, errOut)
	}
	secondContent, err := os.ReadFile(junit)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondContent) <= len(firstSize) {
		t.Errorf("second run's report did not grow: first=%d bytes, second=%d bytes", len(firstSize), len(secondContent))
	}
	if !strings.Contains(string(secondContent), "first.hurl") || !strings.Contains(string(secondContent), "second.hurl") {
		t.Errorf("report missing one of the two files:\n%s", secondContent)
	}
}

// TestE2EReportsNotWrittenWithoutFlag checks that no report file appears
// when no --report-* flag is given, and that a parse error still succeeds
// in writing a report when one is (parse_error_tap's oracle: reports cover
// files that ran before, or as, a parse error).
func TestE2EReportsNotWrittenWithoutFlag(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	if code, _, errOut := runArgs(t, file); code != ExitOK {
		t.Fatalf("exit code = %d; stderr=%s", code, errOut)
	}
}

// TestE2EReportsNotWrittenOnParseError checks upstream parity confirmed
// against the reference CLI's own main.rs: a parse error (or an unreadable
// input file) aborts the whole run immediately — no report is written at
// all, even for files that ran and succeeded before it, and no --test
// summary is printed either.
func TestE2EReportsNotWrittenOnParseError(t *testing.T) {
	srv := testServer(t)
	ok := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	bad := writeTemp(t, "bad.hurl", "not a hurl file at all {{{")
	dir := t.TempDir()
	tap := filepath.Join(dir, "report.tap")

	code, _, errOut := runArgs(t, "--jobs", "1", "--test", "--report-tap", tap, ok, bad)
	if code != ExitParse {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitParse, errOut)
	}
	if _, err := os.Stat(tap); !os.IsNotExist(err) {
		t.Errorf("report.tap exists (err=%v), want no report written at all on a parse error", err)
	}
	if strings.Contains(errOut, "Executed files:") {
		t.Errorf("stderr has the --test summary, want none on a parse error:\n%s", errOut)
	}
	if !strings.Contains(errOut, "ok.hurl") {
		t.Errorf("stderr missing the per-file line for the file that ran before the parse error:\n%s", errOut)
	}
}
