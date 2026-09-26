// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

func BenchmarkValidateResponse(b *testing.B) {
	for _, name := range []string{"petstore-3.0.yaml", "petstore-3.1.yaml"} {
		b.Run(name, func(b *testing.B) {
			s, err := Load(context.Background(), "../../testdata/openapi/"+name, LoadOptions{})
			if err != nil {
				b.Fatal(err)
			}
			v := s.Validator(Options{})
			req := &exchange.Request{Method: "GET", URL: "http://x/v1/pets"}
			resp := &exchange.Response{Status: 200, Body: []byte(`[{"id": 1, "name": "Rex", "kind": "pet"}, {"id": 2, "name": "Milo", "kind": "pet"}]`),
				Headers: exchange.Headers{{Name: "Content-Type", Value: "application/json"}, {Name: "X-Rate-Limit", Value: "10"}}}
			b.ReportAllocs()
			for b.Loop() {
				if vs := v.ValidateResponse(context.Background(), req, resp); len(vs) != 0 {
					b.Fatal(vs)
				}
			}
		})
	}
}
