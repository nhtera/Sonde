// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestTraceParity checks the connection details of a call on every
// transport path: phase timings, the server IP and the verbose connection
// lines, for plain HTTP/1.1, HTTP/1.1 over TLS (forced and negotiated)
// and HTTP/2, on a fresh and on a reused connection.
func TestTraceParity(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
	plain := httptest.NewServer(handler)
	defer plain.Close()
	tls1 := httptest.NewTLSServer(handler)
	defer tls1.Close()
	tls2 := httptest.NewUnstartedServer(handler)
	tls2.EnableHTTP2 = true
	tls2.StartTLS()
	defer tls2.Close()

	tests := []struct {
		name, url, version string
		opts               Options
		tls                bool
	}{
		{"http/1.1", plain.URL, "HTTP/1.1", Options{}, false},
		{"https forced http/1.1", tls2.URL, "HTTP/1.1", Options{HTTPVersion: HTTP11}, true},
		{"https without ALPN", tls1.URL, "HTTP/1.1", Options{}, true},
		{"http/2", tls2.URL, "HTTP/2", Options{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var lines []string
			c := newTestClient(t, ClientConfig{Debug: func(l string) { mu.Lock(); lines = append(lines, l); mu.Unlock() }})
			opts := tt.opts
			opts.Insecure, opts.Verbose = true, true
			for i, fresh := range []bool{true, false} {
				mu.Lock()
				lines = nil
				mu.Unlock()
				call := get(t, c, tt.url, &opts)[0]
				tm, r := call.Timings, call.Response
				if r.Version != tt.version {
					t.Errorf("call %d: version %s, want %s", i, r.Version, tt.version)
				}
				if r.IP != "127.0.0.1" {
					t.Errorf("call %d: IP %q", i, r.IP)
				}
				if tm.PreTransfer <= 0 || tm.StartTransfer < tm.PreTransfer || tm.Total < tm.StartTransfer {
					t.Errorf("call %d: timings %+v", i, tm)
				}
				if fresh && (tm.Connect <= 0 || (tt.tls && tm.AppConnect < tm.Connect)) {
					t.Errorf("fresh call: connect %v, app connect %v", tm.Connect, tm.AppConnect)
				}
				if tt.tls && r.Certificate == nil {
					t.Errorf("call %d: no certificate", i)
				}
				mu.Lock()
				text := strings.Join(lines, "\n")
				mu.Unlock()
				if !strings.Contains(text, "Connected to 127.0.0.1 (127.0.0.1) port ") {
					t.Errorf("call %d: no connection line in %q", i, text)
				}
				if fresh && tt.tls && !strings.Contains(text, "TLS connection using TLSv1.") {
					t.Errorf("fresh call: no TLS line in %q", text)
				}
			}
		})
	}
}
