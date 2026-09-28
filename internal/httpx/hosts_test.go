// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/nhtera/sonde/internal/netpolicy"
)

// countingServer is a test server that counts its requests.
func countingServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if h != nil {
			h(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// countingListener accepts and counts connections without answering.
func countingListener(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var accepts atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			_ = conn.Close()
		}
	}()
	return ln.Addr().String(), &accepts
}

func hostPolicy(t *testing.T, patterns ...string) *netpolicy.Policy {
	t.Helper()
	p, err := netpolicy.Parse(patterns)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func wantDenied(t *testing.T, err error) {
	t.Helper()
	var he *Error
	if !errors.As(err, &he) || he.Kind != ErrHostDenied || !errors.Is(err, netpolicy.ErrDenied) {
		t.Fatalf("err = %v, want a host denial", err)
	}
}

func hostPort(t *testing.T, rawURL string) (string, string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname(), u.Port()
}

// TestHostsURL covers the URL check: the request and every redirect hop.
func TestHostsURL(t *testing.T) {
	other, otherHits := countingServer(t, nil)
	srv, hits := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/away" {
			http.Redirect(w, r, other.URL+"/x", http.StatusFound)
		}
	})
	_, port := hostPort(t, srv.URL)

	c := newTestClient(t, ClientConfig{Hosts: hostPolicy(t, "127.0.0.1:"+port)})
	if _, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL + "/ok"}, &Options{}); err != nil {
		t.Fatalf("allowed host: %v", err)
	}
	calls, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL + "/away"}, &Options{FollowLocation: true})
	wantDenied(t, err)
	if len(calls) != 1 || calls[0].Response.Status != http.StatusFound {
		t.Fatalf("calls before the denied redirect = %d", len(calls))
	}
	_, err = c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: other.URL}, &Options{})
	wantDenied(t, err)
	// A '%' in a name: the resolver would look up the whole name, so it
	// must not match the allowed part before it.
	lc := newTestClient(t, ClientConfig{Hosts: hostPolicy(t, "localhost:"+port, "127.0.0.1:"+port)})
	for _, u := range []string{
		"http://localhost%25.127.0.0.1.nip.io:" + port + "/",
		"http://127.0.0.1%25.evil.test:" + port + "/",
		"http://l%C3%B6calhost:" + port + "/",
	} {
		_, err = lc.Execute(context.Background(), &RequestSpec{Method: "GET", URL: u}, &Options{})
		wantDenied(t, err)
	}
	if hits.Load() != 2 || otherHits.Load() != 0 {
		t.Fatalf("hits = %d, other = %d", hits.Load(), otherHits.Load())
	}
}

// TestHostsDial covers the connection check: an allowed URL host whose
// bytes would go elsewhere is refused before any connection is made.
func TestHostsDial(t *testing.T) {
	addr, accepts := countingListener(t)
	lhost, lport := hostPort(t, "http://"+addr)
	policy := hostPolicy(t, "allowed.test")
	spec := func() *RequestSpec { return &RequestSpec{Method: "GET", URL: "http://allowed.test/"} }

	tests := []struct {
		name string
		opts Options
	}{
		{"connect-to", Options{ConnectTo: []string{"allowed.test:80:" + lhost + ":" + lport}}},
		{"resolve", Options{Resolve: []string{"allowed.test:80:" + lhost}}},
		{"proxy", Options{Proxy: "http://" + addr}},
		{"socks5 proxy", Options{Proxy: "socks5://" + addr}},
		{"unix socket", Options{UnixSocket: "sock"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, ClientConfig{Hosts: policy})
			_, err := c.Execute(context.Background(), spec(), &tt.opts)
			wantDenied(t, err)
		})
	}
	if n := accepts.Load(); n != 0 {
		t.Fatalf("%d connections reached the listener", n)
	}
}

// TestHostsEnvironmentProxy: a proxy from the environment is a host like
// any other, allowed only when it is in the list.
func TestHostsEnvironmentProxy(t *testing.T) {
	proxy, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("via proxy")) })
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	spec := &RequestSpec{Method: "GET", URL: "http://allowed.test/"}

	c := newTestClient(t, ClientConfig{Hosts: hostPolicy(t, "allowed.test")})
	_, err := c.Execute(context.Background(), spec, &Options{})
	wantDenied(t, err)
	if hits.Load() != 0 {
		t.Fatal("the proxy was reached")
	}

	phost, pport := hostPort(t, proxy.URL)
	c = newTestClient(t, ClientConfig{Hosts: hostPolicy(t, "allowed.test", phost+":"+pport)})
	calls, err := c.Execute(context.Background(), spec, &Options{})
	if err != nil || string(calls[0].Response.Body) != "via proxy" {
		t.Fatalf("allowed proxy: calls=%v err=%v", calls, err)
	}
}

// TestHostsUpgrade: a WebSocket handshake is checked like a request.
func TestHostsUpgrade(t *testing.T) {
	c := newTestClient(t, ClientConfig{Hosts: hostPolicy(t, "allowed.test")})
	_, err := c.Upgrade(context.Background(), &RequestSpec{Method: "GET", URL: "ws://denied.test/"}, &Options{})
	wantDenied(t, err)
}
