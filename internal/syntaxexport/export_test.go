// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

const fixtures = "../../testdata/conformance/hurlfmt"

// exportFixtures calls check with each export fixture that has an
// expected file with the given suffix, and its parsed input.
func exportFixtures(t *testing.T, suffix string, check func(name string, f *syntax.File, want string)) {
	t.Helper()
	inputs, err := filepath.Glob(filepath.Join(fixtures, "tests_export", "*.hurl"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, in := range inputs {
		if strings.HasSuffix(in, ".lint.hurl") {
			continue
		}
		want, err := os.ReadFile(strings.TrimSuffix(in, ".hurl") + suffix)
		if err != nil {
			continue
		}
		src, err := os.ReadFile(in)
		if err != nil {
			t.Fatal(err)
		}
		f, err := syntax.Parse(in, src, syntax.DialectHurl)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		n++
		check(filepath.Base(in), f, string(want))
	}
	if n < 19 {
		t.Fatalf("checked %d fixtures, want 19", n)
	}
}

// The reference command appends a final newline when the output lacks one.
func withNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func TestHTMLMatchesReferenceFixtures(t *testing.T) {
	exportFixtures(t, ".html", func(name string, f *syntax.File, want string) {
		if got := withNewline(HTML(f, false)); got != want {
			t.Errorf("%s: HTML differs:\n got %q\nwant %q", name, got, want)
		}
	})
}

func TestHTMLStandalone(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(fixtures, "tests_ok", "html_standalone.hurl"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(fixtures, "tests_ok", "html_standalone.out"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := syntax.Parse("html_standalone.hurl", src, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if got := withNewline(HTML(f, true)); got != string(want) {
		t.Errorf("standalone HTML differs:\n got %q\nwant %q", got, want)
	}
}

func TestJSONMatchesReferenceFixtures(t *testing.T) {
	exportFixtures(t, ".json", func(name string, f *syntax.File, want string) {
		got := JSON(f)
		if !json.Valid([]byte(got)) {
			t.Errorf("%s: JSON output is not valid JSON", name)
		}
		if got := withNewline(got); got != want {
			t.Errorf("%s: JSON differs:\n got %s\nwant %s", name, got, want)
		}
	})
}
