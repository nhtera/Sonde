// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
)

// TestWriteJUnit_Golden reproduces the two-invocation scenario of
// testdata/conformance/hurl/tests_ok/junit/junit.sh: a first run of two
// files (one passing, one failing) then a second run of a third,
// passing, file — each `--test --report-junit ...` call becomes one
// <testsuite>, appended to the same <testsuites> root.
func TestWriteJUnit_Golden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.xml")

	run1 := []*engine.UnitResult{
		successResult("tests/test.1.hurl"),
		failureResult("tests/test.2.hurl"),
	}
	if err := WriteJUnit(path, run1, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	run2 := []*engine.UnitResult{successResult("tests/test.3.hurl")}
	if err := WriteJUnit(path, run2, redactTestSecret); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "../../testdata/report/junit.golden", got)
	assertWellFormedXML(t, got)
}

// TestWriteJUnit_ErrorElement checks a non-assert runtime error produces
// an <error> element (as opposed to <failure> for an assert).
func TestWriteJUnit_ErrorElement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.xml")
	if err := WriteJUnit(path, []*engine.UnitResult{errorResult("t.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "<error>") {
		t.Errorf("expected an <error> element, got:\n%s", got)
	}
	if strings.Contains(string(got), "<failure>") {
		t.Errorf("did not expect a <failure> element, got:\n%s", got)
	}
	assertWellFormedXML(t, got)
}

// TestWriteJUnit_ParseErrorTestcase is the regression test for review
// finding #4 (2026-09-26): a file that never parsed used to render as an
// empty, passing <testcase/> (renderErrors has no entries to walk on a
// ParseError). It must instead carry an <error> with the rendered parse
// error, and count in "errors", not slip past unnoticed as a pass.
func TestWriteJUnit_ParseErrorTestcase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.xml")
	if err := WriteJUnit(path, []*engine.UnitResult{parseErrorResult("bad.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `<testsuite tests="1" errors="1" failures="0">`) {
		t.Errorf("expected a testsuite with one error, not a pass:\n%s", got)
	}
	if !strings.Contains(string(got), "<error>") {
		t.Errorf("expected a <testcase> with an <error> child:\n%s", got)
	}
	if strings.Contains(string(got), `time="0.000" />`) {
		t.Errorf("testcase is still self-closed (a pass), want an <error> child:\n%s", got)
	}
	assertWellFormedXML(t, got)
}

// TestWriteJUnit_InterruptedTestcase is the regression test for review
// finding #4: a unit stopped before its last entry can have no error of
// its own (its last attempt simply never ran) and must not look like a
// pass either.
func TestWriteJUnit_InterruptedTestcase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.xml")
	if err := WriteJUnit(path, []*engine.UnitResult{interruptedResult("stopped.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `<testsuite tests="1" errors="1" failures="0">`) {
		t.Errorf("expected a testsuite with one error, not a pass:\n%s", got)
	}
	if !strings.Contains(string(got), "<error>interrupted") {
		t.Errorf("expected an <error> mentioning \"interrupted\":\n%s", got)
	}
	assertWellFormedXML(t, got)
}

// TestWriteJUnit_InvalidXMLCharsRoundTrip is the regression test for
// review finding #5 (2026-09-26): an assert message quoting a response
// body can contain bytes illegal in XML 1.0 (a control character, an
// invalid UTF-8 sequence). Left as-is, the file WriteJUnit produced was
// not valid XML, and the *next* cumulative WriteJUnit call failed to
// parse it back — so every later run on that path errored out until
// someone deleted the file. They must instead come out as U+FFFD, and a
// second cumulative call must succeed.
func TestWriteJUnit_InvalidXMLCharsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.xml")
	if err := WriteJUnit(path, []*engine.UnitResult{invalidXMLCharResult("hostile.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertWellFormedXML(t, got)
	if strings.ContainsRune(string(got), 0x1b) {
		t.Errorf("raw ESC (U+001B) survived into the JUnit file:\n%s", got)
	}
	if !strings.Contains(string(got), "�") {
		t.Errorf("expected U+FFFD in place of the illegal characters, got:\n%s", got)
	}

	// The actual regression: a second, cumulative call must be able to
	// read the first call's output back and append to it.
	if err := WriteJUnit(path, []*engine.UnitResult{successResult("ok.hurl")}, redactTestSecret); err != nil {
		t.Fatalf("second WriteJUnit call could not read back its own earlier output: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertWellFormedXML(t, got)
	if strings.Count(string(got), "<testsuite ") != 2 {
		t.Errorf("expected 2 testsuites after the second call, got:\n%s", got)
	}
}

// TestWriteJUnit_Redacts checks the run's secret never survives into a
// rendered failure/error message.
func TestWriteJUnit_Redacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.xml")
	results := []*engine.UnitResult{failureResult("t.hurl"), errorResult("t2.hurl")}
	if err := WriteJUnit(path, results, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), testSecret) {
		t.Errorf("secret leaked into JUnit report:\n%s", got)
	}
}

// assertWellFormedXML checks data parses as a well-formed XML document
// (encoding/xml round trip), and additionally runs xmllint --noout when
// it is available on PATH.
func assertWellFormedXML(t *testing.T, data []byte) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		_, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("not well-formed XML: %v", err)
		}
	}

	path, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not available, skipping additional well-formedness check")
	}
	f, err := os.CreateTemp(t.TempDir(), "*.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(path, "--noout", f.Name()).CombinedOutput() //nolint:gosec // G204: path from exec.LookPath, f.Name() from os.CreateTemp
	if err != nil {
		t.Errorf("xmllint --noout: %v\n%s", err, out)
	}
}
