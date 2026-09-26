// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

// FuzzLoad loads arbitrary documents as specs: loading and validating a
// response never panics.
func FuzzLoad(f *testing.F) {
	for _, name := range []string{"petstore-3.0.yaml", "petstore-3.1.yaml", "edge-cases-3.1.yaml", "swagger-2.0.yaml"} {
		b, err := os.ReadFile(filepath.Join("../../testdata/openapi", name)) //nolint:gosec // G304: test fixture
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte("openapi: 3.1.0\npaths: {/a: {get: {responses: {'200': {$ref: '#/x'}}}}}\n"))
	f.Add([]byte("{\"swagger\": \"2.0\", \"paths\": {\"/a\": null}}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "spec.yaml")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(context.Background(), path, LoadOptions{})
		if err != nil {
			return
		}
		v := s.Validator(Options{})
		v.ValidateResponse(context.Background(), &exchange.Request{Method: "GET", URL: "http://x/a"},
			&exchange.Response{Status: 200, Body: []byte(`{"a": 1}`), Headers: exchange.Headers{{Name: "Content-Type", Value: "application/json"}}})
		_, _ = s.Generate(GenerateOptions{})
	})
}
