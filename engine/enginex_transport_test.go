// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/netpolicy"
)

// TestTransportClasses checks the six transport classes, and that the
// error's kind is unchanged.
func TestTransportClasses(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(tlsSrv.Close)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + ln.Addr().String()
	_ = ln.Close()
	policy, err := netpolicy.Parse([]string{"allowed.test"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		want, url string
		options   string
		hosts     bool
		cancel    bool
	}{
		{want: "connect", url: closed + "/"},
		{want: "resolve", url: "http://nope.invalid/"},
		{want: "timeout", url: slow.URL + "/", options: "[Options]\nmax-time: 100ms\n"},
		{want: "tls", url: tlsSrv.URL + "/"},
		{want: "host-denied", url: slow.URL + "/", hosts: true},
		{want: "canceled", url: slow.URL + "/", cancel: true},
	} {
		t.Run(tc.want, func(t *testing.T) {
			r := NewRunner(Options{})
			if tc.hosts {
				enginex.SetHosts(r, policy)
			}
			ctx := context.Background()
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				time.AfterFunc(100*time.Millisecond, cancel)
			}
			src := "GET " + tc.url + "\n" + tc.options + "HTTP 200\n"
			res, err := r.RunSource(ctx, filepath.Join(t.TempDir(), "t.hurl"), []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Entries) != 1 || len(res.Entries[0].Errors) != 1 {
				t.Fatalf("entries %+v", res.Entries)
			}
			e := res.Entries[0].Errors[0]
			if got := enginex.Transport(e); got != tc.want {
				t.Errorf("Transport = %q (%v), want %q", got, e, tc.want)
			}
			if e.Kind() != ErrorHTTP {
				t.Errorf("Kind = %v", e.Kind())
			}
		})
	}

	r := NewRunner(Options{Variables: map[string]any{"base": server(t).URL}})
	res, err := r.RunSource(context.Background(), filepath.Join(t.TempDir(), "t.hurl"), []byte("GET {{base}}/hello\nHTTP 201\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := enginex.Transport(res.Errors()[0]); got != "" {
		t.Errorf("an assert failure has transport class %q", got)
	}
	if got := enginex.Transport(nil); got != "" {
		t.Errorf("nil: %q", got)
	}
	if !strings.Contains(res.Errors()[0].Error(), "status") {
		t.Errorf("error %v", res.Errors()[0])
	}
}
