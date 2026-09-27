// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
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

// BenchmarkRunAll runs 100 one-request files against a server answering
// in 20 ms, one at a time and eight at a time.
func BenchmarkRunAll(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(20 * time.Millisecond)
	}))
	b.Cleanup(srv.Close)
	src := []byte("GET " + srv.URL + "\nHTTP 200\n")
	jobs := func(yield func(Job) bool) {
		for range 100 {
			if !yield(Job{Name: "bench.hurl", Source: src}) {
				return
			}
		}
	}
	for _, n := range []int{1, 8} {
		b.Run(fmt.Sprintf("jobs=%d", n), func(b *testing.B) {
			r := NewRunner(Options{})
			for b.Loop() {
				r.RunAll(context.Background(), jobs, RunAllOptions{Parallel: n})
			}
		})
	}
}

// BenchmarkRunAllMemory runs 10,000 files against a server answering at
// once; with no hooks retaining results, memory stays flat.
func BenchmarkRunAllMemory(b *testing.B) {
	srv := benchServer(b)
	src := []byte("GET " + srv.URL + "\nHTTP 200\n")
	jobs := func(yield func(Job) bool) {
		for range 10_000 {
			if !yield(Job{Name: "bench.hurl", Source: src}) {
				return
			}
		}
	}
	r := NewRunner(Options{})
	for b.Loop() {
		r.RunAll(context.Background(), jobs, RunAllOptions{Parallel: 8})
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		b.ReportMetric(float64(m.HeapAlloc)/(1<<20), "heap-MiB")
	}
}
