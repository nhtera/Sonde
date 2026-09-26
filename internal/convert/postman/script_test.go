// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// TestScriptsAreNeverExecuted proves a pre-request/test script that would
// have a visible side effect if run (writing a file) has no effect at all:
// Import only ever reads the script's text, to keep it as a comment and, in
// the "test" case, to look for a status-code assertion.
func TestScriptsAreNeverExecuted(t *testing.T) {
	dir := t.TempDir()
	canary := filepath.Join(dir, "pwned")
	script := "require('fs').writeFileSync(" + jsonString(t, canary) + ", 'pwned'); pm.response.to.have.status(200);"

	col := map[string]any{
		"info": map[string]any{"name": "x"},
		"item": []any{
			map[string]any{
				"name": "req",
				"event": []any{
					map[string]any{"listen": "test", "script": map[string]any{"exec": []string{script}}},
				},
				"request": map[string]any{"method": "GET", "url": "http://example.test"},
			},
		},
	}
	data, err := json.Marshal(col)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Import(data, Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("the script ran: %s exists", canary)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(out.Files))
	}
	src := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(src, "writeFileSync") {
		t.Error("the script text should still be kept as a comment")
	}
	if !strings.Contains(src, "HTTP 200") {
		t.Error("the recognized status assertion should still be translated")
	}
	sawWarning := false
	for _, w := range out.Warnings {
		if w.Kind == "script" {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Error("expected a script warning")
	}
}

func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
