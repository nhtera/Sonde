// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
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
