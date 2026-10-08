// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"

	"github.com/nhtera/sonde/internal/netpolicy"
)

// h3Server serves HTTP/3 on a local UDP port with the certificate of an
// httptest TLS server, and returns its https:// URL.
func h3Server(t *testing.T) string {
	t.Helper()
	cert := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(cert.Close)
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http3.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_, _ = io.WriteString(w, r.Proto+" "+string(b)) //nolint:gosec // G705: plain-text echo in a test server
		}),
		TLSConfig: http3.ConfigureTLSConfig(&tls.Config{Certificates: cert.TLS.Certificates}), //nolint:gosec // G402: a test server
	}
	go func() { _ = srv.Serve(pc) }()
	t.Cleanup(func() { _ = srv.Close(); _ = pc.Close() })
	return "https://" + pc.LocalAddr().String()
}

// TestHTTP3 checks a request over QUIC: the version, the timings, the
// server IP and certificate, and a second request on the same
// connection.
func TestHTTP3(t *testing.T) {
	url := h3Server(t)
	c := newTestClient(t, ClientConfig{})
	opts := &Options{HTTPVersion: HTTP3, Insecure: true}
	for i := range 2 {
		calls, err := c.Execute(t.Context(), &RequestSpec{Method: "POST", URL: url, Body: Body{Kind: BodyText, Data: []byte("body")}}, opts)
		if err != nil {
			t.Fatal(err)
		}
		r, tm := calls[0].Response, calls[0].Timings
		if r.Version != "HTTP/3" || string(r.Body) != "HTTP/3.0 body" || r.IP != "127.0.0.1" || r.Certificate == nil {
			t.Errorf("request %d: %s %q ip %q cert %v", i, r.Version, r.Body, r.IP, r.Certificate != nil)
		}
		if i == 0 && (tm.Connect <= 0 || tm.AppConnect <= 0 || tm.PreTransfer <= 0) {
			t.Errorf("timings %+v", tm)
		}
	}
	if err := c.Close(); err != nil {
		t.Error(err)
	}
}

// TestHTTP3Fallback checks that --http3 falls back to TCP when QUIC
// cannot connect, with the body sent again whole.
func TestHTTP3Fallback(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_, _ = io.WriteString(w, r.Proto+" "+string(b)) //nolint:gosec // G705: plain-text echo in a test server
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	opts := &Options{HTTPVersion: HTTP3, Insecure: true, ConnectTimeout: 300 * time.Millisecond}
	for range 2 { // the second request goes to TCP at once
		calls, err := c.Execute(t.Context(), &RequestSpec{Method: "POST", URL: srv.URL, Body: Body{Kind: BodyText, Data: []byte("body")}}, opts)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(calls[0].Response.Body); got != "HTTP/2.0 body" {
			t.Errorf("fallback answer %q", got)
		}
	}
}

// TestHTTP3HostPolicy checks that a host policy is applied to the QUIC
// address (connect-to included) before any packet, and that a denial
// never falls back to TCP.
func TestHTTP3HostPolicy(t *testing.T) {
	url := h3Server(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(url, "https://"))
	policy, err := netpolicy.Parse([]string{"127.0.0.1", "allowed.test"})
	if err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, ClientConfig{Hosts: policy})
	opts := &Options{HTTPVersion: HTTP3, Insecure: true, ConnectTo: []string{"allowed.test:" + port + ":127.0.0.2:" + port}}
	start := time.Now()
	_, err = c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: "https://allowed.test:" + port + "/"}, opts)
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrHostDenied {
		t.Fatalf("err = %v, want a host denial", err)
	}
	if time.Since(start) > time.Second {
		t.Error("the denial waited for a handshake or a fallback")
	}
	// Allowed: QUIC to the server.
	if _, err := c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: url}, &Options{HTTPVersion: HTTP3, Insecure: true}); err != nil {
		t.Error(err)
	}
}

// TestHTTP3PolicyAsTCP checks that the QUIC address is allowed as the
// TCP path allows a host: by its name when no resolve rule applies (a
// name rule allows it, an address range does not), and by each address
// of a resolve rule.
func TestHTTP3PolicyAsTCP(t *testing.T) {
	byName, _ := netpolicy.Parse([]string{"localhost"})
	byRange, _ := netpolicy.Parse([]string{"127.0.0.0/8", "::1"})
	ctx := t.Context()
	if _, err := (dialOptions{hosts: byName, network: "tcp"}).resolveUDP(ctx, "localhost:443"); err != nil {
		t.Errorf("name rule: %v", err)
	}
	if _, err := (dialOptions{hosts: byRange, network: "tcp"}).resolveUDP(ctx, "localhost:443"); err == nil {
		t.Error("an address range allowed a name, which TCP refuses")
	}
	d := dialOptions{hosts: byRange, network: "tcp", resolve: parseResolve([]string{"name.test:443:127.0.0.1"})}
	if _, err := d.resolveUDP(ctx, "name.test:443"); err != nil {
		t.Errorf("resolve rule to an allowed address: %v", err)
	}
}

// TestHTTP3FallbackWithinConnectTimeout checks that QUIC and TCP share
// the connect timeout.
func TestHTTP3FallbackWithinConnectTimeout(t *testing.T) {
	c := newTestClient(t, ClientConfig{})
	start := time.Now()
	// 192.0.2.1 (TEST-NET-1) never answers.
	_, err := c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: "https://192.0.2.1/"}, &Options{HTTPVersion: HTTP3, ConnectTimeout: 600 * time.Millisecond})
	if err == nil {
		t.Fatal("a blackholed address answered")
	}
	if elapsed := time.Since(start); elapsed > 1200*time.Millisecond {
		t.Errorf("QUIC then TCP took %v, the connect timeout is 600ms", elapsed)
	}
}
