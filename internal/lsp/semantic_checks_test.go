// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUndefinedVariableNoProject checks the message used when the document
// is inside a workspace folder that has no sonde.yaml at all.
func TestUndefinedVariableNoProject(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, nil)
	c.initialize(false, dir, nil)
	diags := c.open(pathToURI(filepath.Join(dir, "a.hurl")), "GET http://a/\nHTTP 200\n[Asserts]\nstatus == 200\njsonpath \"$.x\" == \"{{missing}}\"\n")
	want := `undefined variable "missing": not captured earlier and no sonde.yaml environment selected`
	if !containsMessage(diags, want) {
		t.Fatalf("diagnostics = %+v, want a message containing %q", diags, want)
	}
}

// TestUndefinedVariableNoFolderSkipsCheck checks the check is skipped
// entirely for a document outside every workspace folder (single-file
// mode): with no configuration at all, a use can't be told genuinely
// undefined from one defined by a sonde.yaml the server simply never gets
// to see (phase 9 LSP review, M1).
func TestUndefinedVariableNoFolderSkipsCheck(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(false, "", nil)
	diags := c.open("file:///w/a.hurl", "GET http://a/\nHTTP 200\n[Asserts]\nstatus == 200\njsonpath \"$.x\" == \"{{missing}}\"\n")
	if containsMessage(diags, "undefined variable") {
		t.Errorf("diagnostics = %+v, want the check skipped with no workspace folder", diags)
	}
}

// TestUndefinedVariableCapturedIsFine checks a variable captured earlier
// in the document is not reported, even with a folder and no sonde.yaml at
// all (so the check does run: it just finds the capture sufficient).
func TestUndefinedVariableCapturedIsFine(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, nil)
	c.initialize(false, dir, nil)
	src := "GET http://a/\nHTTP 200\n[Captures]\nid: jsonpath \"$.id\"\n[Asserts]\njsonpath \"$.name\" == \"{{id}}\"\n"
	diags := c.open(pathToURI(filepath.Join(dir, "a.hurl")), src)
	if containsMessage(diags, "undefined variable") {
		t.Errorf("diagnostics = %+v, want no warning for a captured variable", diags)
	}
}

// TestUndefinedVariableSkippedOnParseError checks the check is skipped
// when the document has parse errors, avoiding false positives from a
// partial AST -- distinct from the no-configuration skip, so this uses a
// real folder to isolate the reason.
func TestUndefinedVariableSkippedOnParseError(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, nil)
	c.initialize(false, dir, nil)
	src := "GET http://a/\nHTTP 2x\n[Asserts]\njsonpath \"$.x\" == \"{{missing}}\"\n"
	diags := c.open(pathToURI(filepath.Join(dir, "a.hurl")), src)
	if containsMessage(diags, "undefined variable") {
		t.Errorf("diagnostics = %+v, want no undefined-variable warning on a broken parse", diags)
	}
}

// TestUndefinedVariableDiagnosticAstral checks the diagnostic's range
// under both position encodings, with an astral character before the
// placeholder shifting the UTF-16 and UTF-8 columns differently.
func TestUndefinedVariableDiagnosticAstral(t *testing.T) {
	dir := t.TempDir()
	src := "GET http://a/😀{{missing}}\nHTTP 200\n"
	nameOffset := strings.Index(src, "missing")
	for _, utf8 := range []bool{true, false} {
		c := newTestClient(t, nil)
		c.initialize(utf8, dir, nil)
		diags := c.open(pathToURI(filepath.Join(dir, "a.hurl")), src)

		var got *Diagnostic
		for i := range diags {
			if strings.Contains(diags[i].Message, `"missing"`) {
				got = &diags[i]
			}
		}
		if got == nil {
			t.Fatalf("utf8=%v: diagnostics = %+v, want an undefined-variable warning", utf8, diags)
		}
		want := newLineIndex(src, !utf8).rangeOf(nameOffset, nameOffset+len("missing"))
		if got.Range != want {
			t.Errorf("utf8=%v: range = %+v, want %+v", utf8, got.Range, want)
		}
	}
}

// TestDeprecatedFilterAndPredicateDiagnostics checks a deprecated filter
// (decode, aliasing charsetDecode) and predicate (includes, aliasing
// contains) each get a tagged warning ranged on just the keyword.
func TestDeprecatedFilterAndPredicateDiagnostics(t *testing.T) {
	src := "GET http://a/\nHTTP 200\n[Asserts]\njsonpath \"$\" includes null\nbody decode \"utf-8\" == \"hi\"\n"
	c := newTestClient(t, nil)
	c.initialize(false, "", nil)
	diags := c.open("file:///w/a.hurl", src)

	lines := strings.Split(src, "\n")
	includesCol := indexCol(t, lines[3], "includes")
	decodeCol := indexCol(t, lines[4], "decode")

	var includesDiag, decodeDiag *Diagnostic
	for i := range diags {
		switch diags[i].Range.Start {
		case Position{Line: 3, Character: includesCol}:
			includesDiag = &diags[i]
		case Position{Line: 4, Character: decodeCol}:
			decodeDiag = &diags[i]
		}
	}
	if includesDiag == nil {
		t.Fatalf("diagnostics = %+v, want one for \"includes\"", diags)
	}
	if includesDiag.Range.End != (Position{Line: 3, Character: includesCol + uint32(len("includes"))}) {
		t.Errorf("includes range = %+v", includesDiag.Range)
	}
	if !hasTag(includesDiag.Tags, tagDeprecated) || !strings.Contains(includesDiag.Message, "contains") {
		t.Errorf("includes diagnostic = %+v", includesDiag)
	}

	if decodeDiag == nil {
		t.Fatalf("diagnostics = %+v, want one for \"decode\"", diags)
	}
	if decodeDiag.Range.End != (Position{Line: 4, Character: decodeCol + uint32(len("decode"))}) {
		t.Errorf("decode range = %+v", decodeDiag.Range)
	}
	if !hasTag(decodeDiag.Tags, tagDeprecated) || !strings.Contains(decodeDiag.Message, "charsetDecode") {
		t.Errorf("decode diagnostic = %+v", decodeDiag)
	}
}

// TestUnsupportedOptionDiagnostic checks an [Options] entry Sonde cannot
// send yet is warned about, ranged on the option name.
func TestUnsupportedOptionDiagnostic(t *testing.T) {
	src := "GET http://a/\n[Options]\nhttp3: true\nHTTP 200\n"
	c := newTestClientTable(t, nil, unsupportedTable(t))
	c.initialize(false, "", nil)
	diags := c.open("file:///w/a.hurl", src)

	want := Range{Start: Position{Line: 2, Character: 0}, End: Position{Line: 2, Character: uint32(len("http3"))}}
	var got *Diagnostic
	for i := range diags {
		if diags[i].Range == want {
			got = &diags[i]
		}
	}
	if got == nil {
		t.Fatalf("diagnostics = %+v, want a warning at %+v", diags, want)
	}
	if !strings.Contains(got.Message, "not supported by Sonde yet") {
		t.Errorf("message = %q", got.Message)
	}
}

// TestProjectErrorDiagnostic checks a broken sonde.yaml is reported once,
// at the start of the document, naming the file.
func TestProjectErrorDiagnostic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, nil)
	c.initialize(false, dir, nil)
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, "GET http://a/\nHTTP 200\n")

	var got *Diagnostic
	for i := range diags {
		if strings.Contains(diags[i].Message, "sonde.yaml") {
			got = &diags[i]
		}
	}
	if got == nil {
		t.Fatalf("diagnostics = %+v, want a sonde.yaml error", diags)
	}
	if got.Range != (Range{}) {
		t.Errorf("range = %+v, want the start of the document", got.Range)
	}
}

// TestSemanticDiagnosticsCap checks warnings are capped at
// maxSemanticDiagnostics, with a final information diagnostic naming how
// many were hidden (H2 in the phase 9 LSP review): an unbounded count from
// a file with many undefined placeholders and no sonde.yaml would
// otherwise publish an unbounded notification on every keystroke.
func TestSemanticDiagnosticsCap(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, nil)
	c.initialize(false, dir, nil)

	const n = maxSemanticDiagnostics + 20
	var b strings.Builder
	b.WriteString("GET http://a/\nHTTP 200\n[Asserts]\n")
	for i := range n {
		fmt.Fprintf(&b, "jsonpath \"$.a\" == \"{{v%d}}\"\n", i)
	}
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, b.String())

	if len(diags) != maxSemanticDiagnostics+1 {
		t.Fatalf("len(diags) = %d, want %d (the cap plus one information diagnostic)", len(diags), maxSemanticDiagnostics+1)
	}
	last := diags[len(diags)-1]
	wantHidden := n - maxSemanticDiagnostics
	if last.Severity != severityInformation {
		t.Errorf("last diagnostic severity = %d, want severityInformation", last.Severity)
	}
	if want := fmt.Sprintf("%d more warnings not shown", wantHidden); last.Message != want {
		t.Errorf("last diagnostic message = %q, want %q", last.Message, want)
	}
}

// indexCol returns substr's byte offset in line as a Position.Character,
// failing the test if it is not found (rather than converting Index's -1
// "not found" to a bogus huge uint32).
func indexCol(t *testing.T, line, substr string) uint32 {
	t.Helper()
	i := strings.Index(line, substr)
	if i < 0 {
		t.Fatalf("%q not found in %q", substr, line)
	}
	return uint32(i) //nolint:gosec // G115: i >= 0 just above; test lines never approach uint32
}

func hasTag(tags []int, tag int) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
