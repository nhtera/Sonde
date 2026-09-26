// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// TestImportSkipsWindowsCmdSyntax covers decision 3 (phase 8 review): a
// command copied from a Windows Command Prompt ("Copy as cURL (cmd)")
// uses "^" for line continuation and "^\"" for an escaped quote, neither
// of which this tokenizer treats specially (it only understands a POSIX
// shell). Left unhandled, the caret ends up glued onto whatever word it
// was adjacent to, corrupting a header or URL value instead of failing
// loudly. Such a command must be skipped with a clear reason instead.
func TestImportSkipsWindowsCmdSyntax(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
	}{
		{"caret-line-continuation", "curl -H \"Accept: */*\" ^\n  \"https://example.com\"\n"},
		{"caret-crlf-continuation", "curl -H \"Accept: */*\" ^\r\n  \"https://example.com\"\r\n"},
		{"caret-escaped-quote", "curl -d \"{^\"a^\":1}\" https://example.com\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Import([]byte(tt.src), syntax.DialectSonde)
			// This input has only one command, and it is skipped, so Import
			// reports the usual "no usable curl command" whole-input error
			// (see its own doc comment); the reason still appears in it.
			if err == nil {
				t.Fatalf("Import succeeded, want the command skipped as Windows cmd syntax; file:\n%s", syntax.Format(res.File))
			}
			if !strings.Contains(err.Error(), windowsCmdReason) {
				t.Fatalf("err = %v, want it to mention %q", err, windowsCmdReason)
			}

			// With a second, valid command alongside it, the Windows-cmd one
			// must be individually skipped rather than failing the import.
			res, err = Import([]byte(tt.src+"curl https://example.com/ok\n"), syntax.DialectSonde)
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if len(res.File.Entries) != 1 {
				t.Fatalf("got %d entries, want 1 (only the valid command)", len(res.File.Entries))
			}
			if len(res.Skipped) != 1 || res.Skipped[0].Reason != windowsCmdReason {
				t.Fatalf("Skipped = %+v, want one entry with reason %q", res.Skipped, windowsCmdReason)
			}
		})
	}
}

// TestImportKeepsBashStyleCommandsWithCaretLike proves the detection does
// not false-positive on ordinary bash-style input that happens to use a
// backslash line continuation or a literal caret character that is not
// adjacent to a newline or a quote.
func TestImportKeepsBashStyleCommandsWithCaretLike(t *testing.T) {
	src := "curl -H \"Accept: */*\" \\\n  \"https://example.com/a^b?x=1\"\n"
	res, err := Import([]byte(src), syntax.DialectSonde)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Fatalf("Skipped = %+v, want none", res.Skipped)
	}
	if got := string(syntax.Format(res.File)); !strings.Contains(got, "a^b") {
		t.Errorf("output = %q, want the literal caret preserved", got)
	}
}
