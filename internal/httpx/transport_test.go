// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/sandbox"
)

func writeFile(t *testing.T, box *sandbox.Root, name string, data []byte) {
	t.Helper()
	if err := box.WriteFile(name, data); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func certPEM(t *testing.T, srv *httptest.Server) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
}

func TestTLSInsecureRequired(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL}, &Options{})
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrTLS {
		t.Fatalf("err = %v, want ErrTLS", err)
	}

	calls := get(t, c, srv.URL, &Options{Insecure: true})
	if calls[0].Response.Status != 200 {
		t.Fatalf("status = %d", calls[0].Response.Status)
	}
	if calls[0].Response.Certificate == nil {
		t.Error("Certificate not populated")
	}
}

func TestTLSCACert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	box, err := sandbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	writeFile(t, box, "ca.pem", certPEM(t, srv))

	c, err := NewClient(ClientConfig{Sandbox: box, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	calls := get(t, c, srv.URL, &Options{CACert: filepath.Join(box.Dir(), "ca.pem")})
	if calls[0].Response.Status != 200 {
		t.Fatalf("status = %d", calls[0].Response.Status)
	}
}

func TestPinnedPublicKey(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	cert := srv.Certificate()
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	good := "sha256//" + base64.StdEncoding.EncodeToString(sum[:])

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, srv.URL, &Options{Insecure: true, PinnedPublicKey: good})
	if calls[0].Response.Status != 200 {
		t.Fatalf("status = %d", calls[0].Response.Status)
	}

	bad := "sha256//" + base64.StdEncoding.EncodeToString(sha256.New().Sum(nil))
	_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL}, &Options{Insecure: true, PinnedPublicKey: bad})
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrTLS {
		t.Fatalf("err = %v, want ErrTLS", err)
	}
}

// genCert issues a self-signed (or CA-signed, when signer is non-nil) leaf
// certificate for use in client-certificate tests.
func genCert(t *testing.T, cn string, isCA bool, signerCert *x509.Certificate, signerKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		DNSNames:              []string{"127.0.0.1", "localhost"},
	}
	parent, signerPriv := tmpl, key
	if signerCert != nil {
		parent, signerPriv = signerCert, signerKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signerPriv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return cert, key, certPEM
}

func TestClientCertificate(t *testing.T) {
	caCert, caKey, caPEMBytes := genCert(t, "test-ca", true, nil, nil)
	clientCert, clientKey, clientPEM := genCert(t, "test-client", false, caCert, caKey)
	_ = clientCert

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) == 0 {
			t.Error("no client certificate presented")
		}
		w.WriteHeader(200)
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	srv.StartTLS()
	defer srv.Close()

	box, err := sandbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	writeFile(t, box, "client.pem", clientPEM)
	writeFile(t, box, "client.key", keyPEM)
	_ = caPEMBytes // the CA only signs the client cert; the server cert is httptest's own

	c, err := NewClient(ClientConfig{Sandbox: box, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Insecure: true skips verifying the server's (httptest-generated)
	// certificate; this test is only about the client certificate being
	// presented and accepted by the server.
	calls := get(t, c, srv.URL, &Options{Insecure: true,
		ClientCert: filepath.Join(box.Dir(), "client.pem"), ClientKey: filepath.Join(box.Dir(), "client.key")})
	if calls[0].Response.Status != 200 {
		t.Fatalf("status = %d", calls[0].Response.Status)
	}
}

func TestHTTP2(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Proto", r.Proto)
		w.WriteHeader(200)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, srv.URL, &Options{Insecure: true, HTTPVersion: HTTP2})
	if calls[0].Response.Version != "HTTP/2" {
		t.Errorf("version = %q", calls[0].Response.Version)
	}
}

func TestHTTP1ForcedOverHTTP2Server(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, srv.URL, &Options{Insecure: true, HTTPVersion: HTTP11})
	if calls[0].Response.Version != "HTTP/1.1" {
		t.Errorf("version = %q, want HTTP/1.1", calls[0].Response.Version)
	}
}

// HTTP/2 over a plain http:// connection falls back to HTTP/1.1.
func TestHTTP2OverPlainHTTPFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	calls, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL}, &Options{HTTPVersion: HTTP2})
	if err != nil || calls[0].Response.Version != "HTTP/1.1" {
		t.Fatalf("calls = %v, err = %v", calls, err)
	}
}

func TestConnectTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { close(done); _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Accept but never write a response, forcing the client to time
		// out waiting for data rather than for the connect; hold the
		// connection open only until the test cleans up.
		<-done
		_ = conn.Close()
	}()

	c := newTestClient(t, ClientConfig{})
	_, err = c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: "http://" + ln.Addr().String() + "/"},
		&Options{Timeout: 100 * time.Millisecond})
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrTimeout {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestConnectRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens here now

	c := newTestClient(t, ClientConfig{})
	_, err = c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: "http://" + addr + "/"}, &Options{})
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrConnect {
		t.Fatalf("err = %v, want ErrConnect", err)
	}
}

func TestResolveHostOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, "http://fake.invalid:"+port+"/", &Options{Resolve: []string{"fake.invalid:" + port + ":127.0.0.1"}})
	if calls[0].Response.Status != 200 {
		t.Fatalf("status = %d", calls[0].Response.Status)
	}
}

func TestConnectToOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, "http://fake.invalid:9999/", &Options{ConnectTo: []string{"fake.invalid:9999:" + host + ":" + port}})
	if calls[0].Response.Status != 200 {
		t.Fatalf("status = %d", calls[0].Response.Status)
	}
}

func TestUnixSocket(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	box, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	c, err := NewClient(ClientConfig{Sandbox: box, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	calls := get(t, c, "http://localhost/", &Options{UnixSocket: "s.sock"})
	if calls[0].Response.Status != 200 {
		t.Fatalf("status = %d", calls[0].Response.Status)
	}
}

func TestHTTPProxy(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("from-origin"))
	}))
	defer origin.Close()

	var proxied bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = true
		resp, err := http.DefaultTransport.RoundTrip(&http.Request{Method: r.Method, URL: r.URL, Header: r.Header})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, origin.URL+"/x", &Options{Proxy: proxy.URL})
	if !proxied {
		t.Error("request did not go through proxy")
	}
	if string(calls[0].Response.Body) != "from-origin" {
		t.Errorf("body = %q", calls[0].Response.Body)
	}
}

func TestNoProxyBypass(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("direct"))
	}))
	defer origin.Close()

	var proxied bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxied = true
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()

	originHost, _, _ := net.SplitHostPort(strings.TrimPrefix(origin.URL, "http://"))
	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, origin.URL+"/x", &Options{Proxy: proxy.URL, NoProxy: originHost})
	if proxied {
		t.Error("request went through proxy despite NoProxy")
	}
	if string(calls[0].Response.Body) != "direct" {
		t.Errorf("body = %q", calls[0].Response.Body)
	}
}

func TestMaxFilesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 1000))
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL}, &Options{MaxFilesize: 100})
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrMaxFilesize {
		t.Fatalf("err = %v, want ErrMaxFilesize", err)
	}
}

func TestMaxRecvSpeed(t *testing.T) {
	const size = 20000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, size))
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	start := time.Now()
	calls := get(t, c, srv.URL, &Options{MaxRecvSpeed: 10000}) // ~2s for 20000 bytes
	elapsed := time.Since(start)
	if len(calls[0].Response.Body) != size {
		t.Fatalf("body size = %d", len(calls[0].Response.Body))
	}
	if elapsed < 500*time.Millisecond {
		t.Errorf("elapsed = %v, expected throttling to slow the transfer down", elapsed)
	}
}

func TestNoProxyMatch(t *testing.T) {
	cases := []struct {
		host, list string
		want       bool
	}{
		{"example.com", "example.com", true},
		{"sub.example.com", "example.com", true},
		{"example.com", ".example.com", true},
		{"other.com", "example.com", false},
		{"anything", "*", true},
		{"a.com", "", false},
	}
	for _, tt := range cases {
		if got := noProxyMatch(tt.host, tt.list); got != tt.want {
			t.Errorf("noProxyMatch(%q, %q) = %v, want %v", tt.host, tt.list, got, tt.want)
		}
	}
}

func TestParseProxyURL(t *testing.T) {
	u, err := parseProxyURL("localhost:8080")
	if err != nil || u.Scheme != "http" || u.Host != "localhost:8080" {
		t.Errorf("parseProxyURL bare host: %+v, %v", u, err)
	}
	u2, err := parseProxyURL("socks5://localhost")
	if err != nil || u2.Port() != "1080" {
		t.Errorf("parseProxyURL socks5 default port: %+v, %v", u2, err)
	}
}

// A rate below the read chunk size still transfers the whole body.
func TestRateLimitedReaderSmallRate(t *testing.T) {
	r := newRateLimitedReader(context.Background(), bytes.NewReader(make([]byte, 3000)), 2000)
	n, err := io.Copy(io.Discard, r)
	if err != nil || n != 3000 {
		t.Fatalf("copied %d bytes, err %v", n, err)
	}
}

// The http_proxy environment variable applies unless --noproxy excludes
// the host; a proxied request counts as rerouted.
func TestEnvironmentProxy(t *testing.T) {
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.String())
		_, _ = w.Write([]byte("from-proxy"))
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, "http://origin.test/x", &Options{})
	if len(seen) != 1 || seen[0] != "http://origin.test/x" || string(calls[0].Response.Body) != "from-proxy" {
		t.Errorf("proxy saw %q, body %q", seen, calls[0].Response.Body)
	}
	u, _ := url.Parse("http://origin.test/x")
	if !isRerouted(u, &Options{}) {
		t.Error("isRerouted = false through an environment proxy")
	}
	if isRerouted(u, &Options{NoProxy: "origin.test"}) {
		t.Error("isRerouted = true for a --noproxy host")
	}
}

func TestSplitCertPassword(t *testing.T) {
	windows := runtime.GOOS == "windows"
	for _, tc := range []struct {
		spec, file, password string
		onlyWindows          bool
	}{
		{spec: "client.pem", file: "client.pem"},
		{spec: "client.pem:secret", file: "client.pem", password: "secret"},
		{spec: `a\:b.pem:secret`, file: "a:b.pem", password: "secret"},
		{spec: `C:\certs\client.pem`, file: `C:\certs\client.pem`, onlyWindows: true},
		{spec: "c:/certs/client.pem:secret", file: "c:/certs/client.pem", password: "secret", onlyWindows: true},
		{spec: "C:secret", file: "C", password: "secret"},
	} {
		file, password := splitCertPassword(tc.spec)
		if tc.onlyWindows && !windows {
			continue
		}
		if file != tc.file || password != tc.password {
			t.Errorf("splitCertPassword(%q) = %q, %q, want %q, %q", tc.spec, file, password, tc.file, tc.password)
		}
	}
}
