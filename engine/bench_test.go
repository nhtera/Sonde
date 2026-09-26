// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func benchServer(b *testing.B) *httptest.Server {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id": 42, "name": "Bob"}`)
	}))
	b.Cleanup(srv.Close)
	return srv
}

// BenchmarkRawHTTP is the baseline: one GET with a fresh transport, as
// each run of a file uses its own connections.
func BenchmarkRawHTTP(b *testing.B) {
	srv := benchServer(b)
	for b.Loop() {
		tr := &http.Transport{}
		resp, err := (&http.Client{Transport: tr}).Get(srv.URL)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		tr.CloseIdleConnections()
	}
}

// BenchmarkRunSource runs a one-entry file with an implicit status
// assert and a JSONPath assert.
func BenchmarkRunSource(b *testing.B) {
	srv := benchServer(b)
	src := []byte("GET " + srv.URL + "\nHTTP 200\n[Asserts]\njsonpath \"$.id\" == 42\n")
	r := NewRunner(Options{})
	defer r.Close()
	for b.Loop() {
		res, err := r.RunSource(context.Background(), "bench.hurl", src)
		if err != nil || !res.Success {
			b.Fatalf("run failed: %v", err)
		}
	}
}
