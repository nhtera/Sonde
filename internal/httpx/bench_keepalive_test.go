// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nhtera/sonde/internal/sandbox"
)

// BenchmarkKeepAlive sends sequential requests from one client over a
// kept-alive connection: the cost of the HTTP/1.1 request path itself.
func BenchmarkKeepAlive(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id": 42, "name": "Bob"}`)
	}))
	defer srv.Close()
	box, err := sandbox.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer box.Close()
	c, err := NewClient(ClientConfig{Sandbox: box})
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	spec := &RequestSpec{Method: "GET", URL: srv.URL + "/item"}
	opts := &Options{}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Execute(context.Background(), spec, opts); err != nil {
			b.Fatal(err)
		}
	}
}
