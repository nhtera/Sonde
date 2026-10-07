// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// openFDs counts the process's open file descriptors.
func openFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skip("no /dev/fd")
	}
	return len(entries)
}

// TestNoFDLeak runs 1000 exchanges mixing kept-alive, closed, chunked,
// aborted (max-filesize) and HTTP/1.0 responses, then closes the client:
// no file descriptor stays open.
func TestNoFDLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/close":
			w.Header().Set("Connection", "close")
		case "/chunked":
			w.(http.Flusher).Flush()
		case "/big":
			_, _ = w.Write(make([]byte, 64<<10))
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	before := openFDs(t)
	c := newTestClient(t, ClientConfig{})
	paths := []string{"/keep", "/close", "/chunked", "/big", "/keep"}
	for i := range 1000 {
		opts := &Options{MaxFilesize: 1024}
		if i%7 == 0 {
			opts.HTTPVersion = HTTP10
		}
		_, _ = c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL + paths[i%len(paths)]}, opts)
	}
	_ = c.Close()
	srv.CloseClientConnections()
	deadline := time.Now().Add(2 * time.Second)
	for openFDs(t) > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if after := openFDs(t); after > before+2 {
		t.Errorf("%d file descriptors open before, %d after", before, after)
	}
}
