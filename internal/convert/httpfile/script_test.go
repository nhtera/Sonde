// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// TestScriptsNeverExecuted imports a .http file whose pre-request and
// response handler scripts would have an observable side effect (writing a
// file, per the JetBrains/REST Client scripting APIs) if they ran. Neither
// script has any way to run here: httpfile has no script engine at all, only
// a text scanner. This test pins that down as a regression guard, and checks
// the script text still ends up fully readable as a comment, with a warning.
func TestScriptsNeverExecuted(t *testing.T) {
	dir := t.TempDir()
	canary := filepath.Join(dir, "canary")
	src := "< {%\n" +
		"  require('fs').writeFileSync('" + canary + "', 'pwned');\n" +
		"%}\n" +
		"GET https://example.test/x\n\n" +
		"> {%\n" +
		"  require('fs').writeFileSync('" + canary + "', 'pwned');\n" +
		"%}\n"
	out, err := Import("req", []byte(src), syntax.DialectHurl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("script side effect ran: canary file exists (err=%v)", err)
	}

	rendered := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(rendered, "writeFileSync") {
		t.Errorf("script text should still be visible as a comment:\n%s", rendered)
	}
	if strings.Contains(rendered, "\n{%") || strings.Contains(rendered, "\n<") {
		t.Errorf("script markers should not survive as request syntax:\n%s", rendered)
	}

	var scriptWarnings int
	for _, w := range out.Warnings {
		if w.Kind == convert.WarnScript {
			scriptWarnings++
		}
	}
	if scriptWarnings != 2 {
		t.Errorf("script warnings = %d, want 2 (pre-request + response handler)", scriptWarnings)
	}
}

// TestExternalScriptsNeverRead checks that an external script reference
// (never executed either) is kept as a plain path comment, not read from
// disk.
func TestExternalScriptsNeverRead(t *testing.T) {
	src := "< ./does-not-exist-and-is-never-read.js\nGET https://example.test/x\n\n> ./also-never-read.js\n"
	out, err := Import("req", []byte(src), syntax.DialectHurl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(syntax.Format(out.Files[0].File))
	for _, want := range []string{"does-not-exist-and-is-never-read.js", "also-never-read.js"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered file should mention %q:\n%s", want, rendered)
		}
	}
}
