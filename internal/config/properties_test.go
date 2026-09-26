// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func TestParsePropertiesInferred(t *testing.T) {
	content := "foo=bar\nflag=true\nid=123\n"
	got, err := ParseProperties([]byte(content), Inferred)
	if err != nil {
		t.Fatal(err)
	}
	want := []Assignment{
		{"foo", value.String("bar")},
		{"flag", value.Bool(true)},
		{"id", value.Int(123)},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, a := range want {
		if got[i] != a {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], a)
		}
	}
}

func TestParsePropertiesComments(t *testing.T) {
	content := "foo=bar\n# With some comments\n# bla bla bla\nflag=true\nid=123\n"
	got, err := ParseProperties([]byte(content), Forced)
	if err != nil {
		t.Fatal(err)
	}
	want := []Assignment{
		{"foo", value.String("bar")},
		{"flag", value.String("true")},
		{"id", value.String("123")},
	}
	for i, a := range want {
		if got[i] != a {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], a)
		}
	}
}

func TestParsePropertiesBlankLinesAndCRLF(t *testing.T) {
	got, err := ParseProperties([]byte("\r\nfoo=bar\r\n\r\n  \r\nbaz=1\r\n"), Inferred)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "foo" || got[1].Name != "baz" {
		t.Errorf("got %+v", got)
	}
}

func TestParsePropertiesError(t *testing.T) {
	if _, err := ParseProperties([]byte("noequals\n"), Inferred); err == nil {
		t.Fatal("expected an error for a line with no '='")
	}
}

// TestParsePropertiesForcedMissingValueNeverEchoesLine guards against a
// malformed secrets-file line (a likely YAML habit like "token: VALUE")
// leaking its value: the value is not yet registered for redaction, so it
// must never reach an error message, stderr, or a log.
func TestParsePropertiesForcedMissingValueNeverEchoesLine(t *testing.T) {
	const secretValue = "s3cr3t-and-must-never-appear"
	_, err := ParseProperties([]byte("token: "+secretValue+"\n"), Forced)
	if err == nil {
		t.Fatal("expected an error for a secrets line with no '='")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatalf("error leaked the secret value: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("error = %q, want it to name line 1", err.Error())
	}
	// Unlike a --secret flag error (TestParseAssignmentForcedMissingValue),
	// a file-sourced error names only the line: not even the name-looking
	// prefix before ':' is echoed.
	if strings.Contains(err.Error(), "token") {
		t.Errorf("error = %q, want it to name only the line, not any part of the line's text", err.Error())
	}
}

func TestParsePropertiesForcedMissingValueLineNumber(t *testing.T) {
	_, err := ParseProperties([]byte("a=1\nb=2\nbadline\n"), Forced)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %q, want it to name line 3", err.Error())
	}
}
