// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/syntax"
)

// TestPostmanPerf imports a 5,000-request collection, built in memory, and
// checks it completes in under 3s (docs/guides/import-export.md's importer
// non-functional requirement). Skipped under -short; the deadline itself is
// not enforced under -race, whose instrumentation overhead dwarfs it.
func TestPostmanPerf(t *testing.T) {
	if testing.Short() {
		t.Skip("perf test skipped with -short")
	}
	const n = 5000
	items := make([]any, n)
	for i := range items {
		items[i] = map[string]any{
			"name": fmt.Sprintf("request-%04d", i),
			"request": map[string]any{
				"method": "GET",
				"url":    fmt.Sprintf("{{base_url}}/items/%d?token={{token}}", i),
				"header": []any{map[string]any{"key": "X-Request-Id", "value": "{{$uuid}}"}},
			},
		}
	}
	col := map[string]any{
		"info":     map[string]any{"name": "perf"},
		"variable": []any{map[string]any{"key": "base_url", "value": "https://example.test"}},
		"item":     items,
	}
	data, err := json.Marshal(col)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	out, err := Import(data, Options{Dialect: syntax.DialectHurl})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != n {
		t.Fatalf("files = %d, want %d", len(out.Files), n)
	}
	t.Logf("imported %d requests in %s", n, elapsed)
	if elapsed > 3*time.Second && !raceEnabled {
		t.Errorf("import took %s, want < 3s", elapsed)
	}
}
