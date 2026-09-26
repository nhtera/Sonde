// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLoadLargeSpec loads the spec named by SONDE_LARGE_SPEC (a large real
// description, such as a public REST API's, kept out of the repository).
func TestLoadLargeSpec(t *testing.T) {
	path := os.Getenv("SONDE_LARGE_SPEC")
	if path == "" {
		t.Skip("SONDE_LARGE_SPEC is not set")
	}
	start := time.Now()
	s, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	d := time.Since(start)
	t.Logf("loaded %d operations in %s", len(s.Operations()), d)
	if d > 2*time.Second {
		t.Errorf("load took %s, want < 2s", d)
	}
}
