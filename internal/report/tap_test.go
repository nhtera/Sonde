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

// TestWriteTAP_Golden reproduces testdata/conformance/hurl/tests_ok/tap/tap.sh:
// a first run of two files (one passing, one failing) then a second run
// of a third, passing, file — the report accumulates and renumbers.
func TestWriteTAP_Golden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.tap")

	run1 := []*engine.UnitResult{
		successResult("tests/test.1.hurl"),
		failureResult("tests/test.2.hurl"),
	}
	if err := WriteTAP(path, run1, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	run2 := []*engine.UnitResult{successResult("tests/test.3.hurl")}
	if err := WriteTAP(path, run2, redactTestSecret); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "../../testdata/report/tap.golden", got)
}

// TestWriteTAP_InvalidExistingReport reproduces
// testdata/conformance/hurl/tests_failed/parse_error_tap: --report-tap
// pointed at a file that already exists but is not a TAP report at all.
// The reference CLI's oracle says the run itself still succeeds, but the
// report write fails with this exact message (exit 127, a "report
// cannot be written" runtime error per architecture.md's exit code
// table) — and never touches the file it could not parse.
func TestWriteTAP_InvalidExistingReport(t *testing.T) {
	const fixturePath = "../../testdata/conformance/hurl/tests_failed/parse_error_tap/parse_error_tap.tap"
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "result.tap")
	if err := os.WriteFile(path, fixture, 0o644); err != nil { //nolint:gosec // G306: test fixture
		t.Fatal(err)
	}

	res := successResult("tests_ok/hello/hello.hurl")
	err = WriteTAP(path, []*engine.UnitResult{res}, redactTestSecret)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	assertGolden(t, "../../testdata/report/tap_invalid_header.golden", []byte(err.Error()+"\n"))

	if want := "Invalid TAP Header <I'm not a TAP file!>"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(fixture) {
		t.Errorf("report file was modified despite the write failing:\n%s", after)
	}
}

// TestEscapeTAPDescription checks the TAP 13 escaping rules directly: see
// escapeTAPDescription's doc comment for why each one matters.
func TestEscapeTAPDescription(t *testing.T) {
	cases := map[string]string{
		"plain.hurl":         "plain.hurl",
		"has#hash.hurl":      `has\#hash.hurl`,
		"multi#one#two.hurl": `multi\#one\#two.hurl`,
		"line\nbreak.hurl":   "line break.hurl",
		"cr\rreturn.hurl":    "cr return.hurl",
		"crlf\r\nboth.hurl":  "crlf both.hurl",
	}
	for in, want := range cases {
		if got := escapeTAPDescription(in); got != want {
			t.Errorf("escapeTAPDescription(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWriteTAP_EscapesHash is the regression test for review finding #20
// (2026-09-26): an unescaped '#' in a description is read by a TAP
// consumer as the start of a directive (e.g. "# SKIP", "# TODO"), which
// can silently hide a real failure.
func TestWriteTAP_EscapesHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.tap")
	res := failureResult("tests/issue#42.hurl")
	if err := WriteTAP(path, []*engine.UnitResult{res}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `tests/issue\#42.hurl`) {
		t.Errorf(`expected the '#' escaped as '\#', got:\n%s`, got)
	}
}

// TestWriteTAP_Redacts checks the run's secret never survives into a
// reported file name.
func TestWriteTAP_Redacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.tap")
	// The secret is not normally part of a filename, but the rule is
	// "every string goes through redact": prove it does even here.
	res := successResult("tests/" + testSecret + ".hurl")
	if err := WriteTAP(path, []*engine.UnitResult{res}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), testSecret) {
		t.Errorf("secret leaked into TAP report:\n%s", got)
	}
}
