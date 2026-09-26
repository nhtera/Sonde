// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// parseEntries parses src for a RenderCurl test.
func parseEntries(t *testing.T, src string) *syntax.File {
	t.Helper()
	f, err := syntax.Parse("<test>", []byte(src), syntax.DialectHurl)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
	return f
}

// TestRenderCurlPerEntryIsolation checks that one entry's failure (an
// undefined variable in a typed [Options] value, which has no literal
// {{name}} fallback) is reported in that entry's own Err and does not
// discard any other entry's command.
func TestRenderCurlPerEntryIsolation(t *testing.T) {
	f := parseEntries(t, "GET http://a/one\n\n"+
		"GET http://a/two\n[Options]\nmax-redirs: {{n}}\n\n"+
		"GET http://a/three\n")
	entries, err := RenderCurl(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("RenderCurl: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	if entries[0].Err != nil || !strings.Contains(entries[0].Command, "/one") {
		t.Errorf("entry 1 = %+v, want a clean /one command", entries[0])
	}
	if entries[1].Err == nil {
		t.Errorf("entry 2 (undefined {{n}} in max-redirs) succeeded, want an Err")
	}
	if entries[2].Err != nil || !strings.Contains(entries[2].Command, "/three") {
		t.Errorf("entry 3 = %+v, want a clean /three command (not discarded by entry 2's failure)", entries[2])
	}
}

// TestRenderCurlMissingDoesNotLeak checks that an undefined variable
// discovered in one entry is not remembered as defined for a later entry
// that references the same name: each entry reports its own Undefined
// list.
func TestRenderCurlMissingDoesNotLeak(t *testing.T) {
	f := parseEntries(t, "GET http://a/one\nX-Token: {{token}}\n\nGET http://a/two\nX-Token: {{token}}\n")
	entries, err := RenderCurl(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("RenderCurl: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	for i, e := range entries {
		if e.Err != nil {
			t.Fatalf("entry %d: %v", i+1, e.Err)
		}
		if len(e.Undefined) != 1 || e.Undefined[0] != "token" {
			t.Errorf("entry %d Undefined = %v, want [token] (leaked across entries if empty)", i+1, e.Undefined)
		}
		if !strings.Contains(e.Command, "{{token}}") {
			t.Errorf("entry %d command = %q, want a literal {{token}}", i+1, e.Command)
		}
	}
}

// TestRenderCurlUndefinedURLRenders checks that an undefined variable
// used in the URL itself still renders as a literal {{name}} instead of
// aborting the entry (checkURL is skipped for export).
func TestRenderCurlUndefinedURLRenders(t *testing.T) {
	f := parseEntries(t, "GET {{base_url}}/c\n")
	entries, err := RenderCurl(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("RenderCurl: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Err != nil {
		t.Fatalf("entry 1: %v, want no error (base_url should stay literal)", entries[0].Err)
	}
	if !strings.Contains(entries[0].Command, "{{base_url}}/c") {
		t.Errorf("command = %q, want a literal {{base_url}}", entries[0].Command)
	}
	if len(entries[0].Undefined) != 1 || entries[0].Undefined[0] != "base_url" {
		t.Errorf("Undefined = %v, want [base_url]", entries[0].Undefined)
	}
}

// TestRenderCurlEntryRangeValidation checks that an invalid
// FromEntry/ToEntry range is a whole-call error, not a silently empty
// result.
func TestRenderCurlEntryRangeValidation(t *testing.T) {
	f := parseEntries(t, "GET http://a/\n")
	for _, tc := range []struct {
		name string
		opt  Options
	}{
		{"negative FromEntry", Options{FromEntry: -1}},
		{"negative ToEntry", Options{ToEntry: -1}},
		{"FromEntry after ToEntry", Options{FromEntry: 3, ToEntry: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RenderCurl(context.Background(), f, tc.opt); err == nil {
				t.Error("RenderCurl returned no error for an invalid entry range")
			}
		})
	}
}

// TestRenderCurlContextCanceled checks that an already-canceled context
// stops RenderCurl instead of rendering every entry.
func TestRenderCurlContextCanceled(t *testing.T) {
	f := parseEntries(t, "GET http://a/one\n\nGET http://a/two\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	entries, err := RenderCurl(ctx, f, Options{})
	if err == nil {
		t.Error("RenderCurl with a canceled context returned no error")
	}
	if len(entries) != 0 {
		t.Errorf("RenderCurl with a canceled context rendered %d entries, want 0", len(entries))
	}
}

// TestRenderCurlSkippedEntryOmitted checks that a skip: true entry, which
// a run never sends either, is left out of the result rather than
// reported as an error.
func TestRenderCurlSkippedEntryOmitted(t *testing.T) {
	f := parseEntries(t, "GET http://a/one\n[Options]\nskip: true\n\nGET http://a/two\n")
	entries, err := RenderCurl(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("RenderCurl: %v", err)
	}
	if len(entries) != 1 || entries[0].Index != 2 {
		t.Fatalf("entries = %+v, want exactly entry 2", entries)
	}
}

// TestRenderCurlRedactsSecrets checks that RenderCurl redacts a
// registered secret in every entry's command.
func TestRenderCurlRedactsSecrets(t *testing.T) {
	f := parseEntries(t, "GET http://a/\nAuthorization: Bearer {{tok}}\n")
	entries, err := RenderCurl(context.Background(), f, Options{Secrets: map[string]string{"tok": "s3cr3t-value"}})
	if err != nil {
		t.Fatalf("RenderCurl: %v", err)
	}
	if strings.Contains(entries[0].Command, "s3cr3t-value") {
		t.Errorf("command leaks the secret: %s", entries[0].Command)
	}
	if !strings.Contains(entries[0].Command, "***") {
		t.Errorf("command has no redaction marker: %s", entries[0].Command)
	}
}
