// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// TestNoHeaders checks that no-header drops a header of that name
// wherever it comes from: the entry, --header, or the defaults (Accept,
// User-Agent, and net/http's own User-Agent).
func TestNoHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "GET", URL: srv.URL, Headers: []exchange.Header{{Name: "Baz", Value: "1"}, {Name: "Keep", Value: "2"}}}
	opts := &Options{
		Headers:   []exchange.Header{{Name: "Foo", Value: "3"}},
		User:      "a:b",
		NoHeaders: []string{"user-agent", "Accept", "baz", "FOO", "Authorization"},
	}
	calls, err := c.Execute(t.Context(), spec, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"User-Agent", "Accept", "Baz", "Foo", "Authorization"} {
		if v, ok := got[name]; ok {
			t.Errorf("%s sent: %q", name, v)
		}
	}
	if got.Get("Keep") != "2" {
		t.Errorf("Keep = %q", got.Get("Keep"))
	}
	var recorded []string
	for _, h := range calls[0].Request.Headers {
		recorded = append(recorded, h.Name)
	}
	if want := []string{"Host", "Keep"}; !slices.Equal(recorded, want) {
		t.Errorf("recorded headers %v, want %v", recorded, want)
	}
}

// forwardProxy is an HTTP proxy that answers plain requests itself and
// tunnels CONNECT requests, recording the headers it receives.
type forwardProxy struct {
	*httptest.Server
	mu      sync.Mutex
	plain   []http.Header
	connect []http.Header
}

func newForwardProxy(t *testing.T) *forwardProxy {
	p := &forwardProxy{}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if r.Method != http.MethodConnect {
			p.plain = append(p.plain, r.Header.Clone())
			_, _ = io.WriteString(w, "from proxy")
			return
		}
		p.connect = append(p.connect, r.Header.Clone())
		target, err := net.Dial("tcp", r.Host) //nolint:gosec // G704: a test proxy tunneling to the test's own server
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
		go func() { _, _ = io.Copy(target, conn); _ = target.Close() }()
		go func() { _, _ = io.Copy(conn, target); _ = conn.Close() }()
	}))
	t.Cleanup(p.Close)
	return p
}

// TestProxyHeaders checks where proxy headers go: with the request of an
// http:// URL sent through the proxy, in the CONNECT request of an
// https:// one (not to the server), and nowhere without a proxy.
func TestProxyHeaders(t *testing.T) {
	var serverHeaders []http.Header
	var mu sync.Mutex
	record := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		serverHeaders = append(serverHeaders, r.Header.Clone())
		mu.Unlock()
	})
	tlsSrv := httptest.NewTLSServer(record)
	defer tlsSrv.Close()
	plainSrv := httptest.NewServer(record)
	defer plainSrv.Close()
	proxy := newForwardProxy(t)

	c := newTestClient(t, ClientConfig{})
	headers := []exchange.Header{{Name: "X-To-Proxy", Value: "p"}}
	opts := &Options{Proxy: proxy.URL, ProxyHeaders: headers, Insecure: true}

	calls := get(t, c, "http://example.test/a", opts)
	if string(calls[0].Response.Body) != "from proxy" {
		t.Fatalf("plain request did not go through the proxy: %q", calls[0].Response.Body)
	}
	if !slices.ContainsFunc(calls[0].Request.Headers, func(h exchange.Header) bool { return h.Name == "X-To-Proxy" }) {
		t.Errorf("recorded headers of the proxied request miss the proxy header: %v", calls[0].Request.Headers)
	}
	calls = get(t, c, tlsSrv.URL+"/b", opts)
	for _, h := range calls[0].Request.Headers {
		if h.Name == "X-To-Proxy" {
			t.Errorf("the tunneled request records the proxy header")
		}
	}
	get(t, c, plainSrv.URL+"/c", &Options{ProxyHeaders: headers})

	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if len(proxy.plain) != 1 || proxy.plain[0].Get("X-To-Proxy") != "p" {
		t.Errorf("plain proxied request headers: %v", proxy.plain)
	}
	if len(proxy.connect) != 1 || proxy.connect[0].Get("X-To-Proxy") != "p" {
		t.Errorf("CONNECT headers: %v", proxy.connect)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(serverHeaders) != 2 {
		t.Fatalf("server saw %d requests", len(serverHeaders))
	}
	for _, h := range serverHeaders {
		if h.Get("X-To-Proxy") != "" {
			t.Errorf("the server received the proxy header: %v", h)
		}
	}
}

// TestProxyHeadersSeparateTransports checks that entries with different
// proxy headers do not share a pooled tunnel, whose CONNECT request
// carried the first entry's headers.
func TestProxyHeadersSeparateTransports(t *testing.T) {
	a := transportCacheKey(&Options{Proxy: "http://p:1", ProxyHeaders: []exchange.Header{{Name: "A", Value: "1"}}})
	b := transportCacheKey(&Options{Proxy: "http://p:1", ProxyHeaders: []exchange.Header{{Name: "A", Value: "2"}}})
	if a == b {
		t.Errorf("same transport key %q for different proxy headers", a)
	}
	if transportCacheKey(&Options{HTTPVersion: HTTP2}) == transportCacheKey(&Options{HTTPVersion: HTTP2PriorKnowledge}) {
		t.Error("same transport key for HTTP/2 and HTTP/2 with prior knowledge")
	}
}

// TestHTTP2PriorKnowledge checks that prior knowledge speaks cleartext
// HTTP/2 to an http:// URL, and negotiates as usual over TLS (falling back
// to HTTP/1.1 for a server without HTTP/2).
func TestHTTP2PriorKnowledge(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Proto) //nolint:gosec // G705: plain-text protocol name in a test server
	})
	h2c := httptest.NewUnstartedServer(handler)
	h2c.Config.Protocols = new(http.Protocols)
	h2c.Config.Protocols.SetHTTP1(true)
	h2c.Config.Protocols.SetUnencryptedHTTP2(true)
	h2c.Start()
	defer h2c.Close()
	h1TLS := httptest.NewTLSServer(handler)
	defer h1TLS.Close()
	h2TLS := httptest.NewUnstartedServer(handler)
	h2TLS.EnableHTTP2 = true
	h2TLS.StartTLS()
	defer h2TLS.Close()

	c := newTestClient(t, ClientConfig{})
	opts := &Options{HTTPVersion: HTTP2PriorKnowledge, Insecure: true}
	for _, tt := range []struct{ url, want string }{
		{h2c.URL, "HTTP/2.0"},
		{h2TLS.URL, "HTTP/2.0"},
		{h1TLS.URL, "HTTP/1.1"},
	} {
		calls := get(t, c, tt.url, opts)
		if got := string(calls[0].Response.Body); got != tt.want {
			t.Errorf("%s: server saw %s, want %s", tt.url, got, tt.want)
		}
	}
	// Without prior knowledge, an http:// URL asking for HTTP/2 gets
	// HTTP/1.1 (there is no upgrade).
	calls := get(t, c, h2c.URL, &Options{HTTPVersion: HTTP2})
	if got := string(calls[0].Response.Body); !strings.HasPrefix(got, "HTTP/1.") {
		t.Errorf("http2 over cleartext: %s", got)
	}
}

// TestProxyHeadersNotToSOCKS checks that an http:// request tunneled by a
// SOCKS proxy (from the environment) does not get the proxy headers: it
// reaches the server itself.
func TestProxyHeadersNotToSOCKS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	prep := &preparedRequest{req: req}
	socks := func(*http.Request) (*url.URL, error) { return url.Parse("socks5://127.0.0.1:1080") }
	opts := &Options{ProxyHeaders: []exchange.Header{{Name: "Proxy-Authorization", Value: "x"}}}
	if err := addProxyHeaders(prep, &builtTransport{proxy: socks}, opts); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Proxy-Authorization") != "" || len(prep.headers) != 0 {
		t.Errorf("proxy headers added for a SOCKS proxy: %v", req.Header)
	}
}

// TestHTTP2PriorKnowledgeThroughProxy checks that an HTTP proxy receiving
// an http:// request gets HTTP/1.1, as curl sends it.
func TestHTTP2PriorKnowledgeThroughProxy(t *testing.T) {
	proxy := newForwardProxy(t)
	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, "http://example.test/", &Options{Proxy: proxy.URL, HTTPVersion: HTTP2PriorKnowledge})
	if string(calls[0].Response.Body) != "from proxy" || calls[0].Response.Version != "HTTP/1.1" {
		t.Errorf("through the proxy: %s %q", calls[0].Response.Version, calls[0].Response.Body)
	}
}

// TestNoHeaderMultipartContentType checks that no-header also drops the
// Content-Type of a multipart body.
func TestNoHeaderMultipartContentType(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "POST", URL: srv.URL, Multipart: []MultipartParam{{Param: &Param{Name: "a", Value: "b"}}}}
	calls, err := c.Execute(t.Context(), spec, &Options{NoHeaders: []string{"content-type"}})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := got["Content-Type"]; ok {
		t.Errorf("Content-Type sent: %q", v)
	}
	for _, h := range calls[0].Request.Headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			t.Errorf("Content-Type recorded: %q", h.Value)
		}
	}
}

// TestWireLayer checks what the own HTTP/1.x layer adds over net/http: an
// HTTP/1.0 request line, request headers in order and case with one Host,
// response headers in wire order and case (an invalid name included),
// and the legacy fallback.
func TestWireLayer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	heads := make(chan string, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				buf := make([]byte, 4096)
				n, _ := conn.Read(buf)
				heads <- string(buf[:n])
				_, _ = io.WriteString(conn, "HTTP/1.0 200 OK\r\nzeta: 1\r\n<script>x</script>: 2\r\nAlpha: 3\r\nContent-Length: 2\r\n\r\nok")
			}()
		}
	}()
	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "GET", URL: "http://" + ln.Addr().String() + "/p", Headers: []exchange.Header{
		{Name: "x-lower", Value: "a"}, {Name: "Host", Value: "virtual.test"},
	}}
	calls, err := c.Execute(t.Context(), spec, &Options{HTTPVersion: HTTP10, NoHeaders: []string{"User-Agent"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := "GET /p HTTP/1.0\r\nHost: virtual.test\r\nAccept: */*\r\nx-lower: a\r\n\r\n"; <-heads != want {
		t.Errorf("request head, want %q", want)
	}
	var names []string
	for _, h := range calls[0].Response.Headers {
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "zeta,<script>x</script>,Alpha,Content-Length" || calls[0].Response.Version != "HTTP/1.0" {
		t.Errorf("response %s headers %v", calls[0].Response.Version, names)
	}
	var recorded []string
	for _, h := range calls[0].Request.Headers {
		recorded = append(recorded, h.Name+": "+h.Value)
	}
	if strings.Join(recorded, "|") != "Host: virtual.test|Accept: */*|x-lower: a" {
		t.Errorf("recorded request headers %q", recorded)
	}

	t.Setenv("SONDE_HTTP1_WIRE", "legacy")
	legacy := newTestClient(t, ClientConfig{})
	if _, err := legacy.Execute(t.Context(), spec, &Options{HTTPVersion: HTTP10}); err == nil {
		t.Error("legacy: http1.0 accepted")
	}
	// net/http refuses the invalid header name the own layer reads.
	if _, err := legacy.Execute(t.Context(), spec, &Options{}); err == nil || !strings.Contains(err.Error(), "malformed MIME header") {
		t.Errorf("legacy: %v", err)
	}
	if head := <-heads; !strings.HasPrefix(head, "GET /p HTTP/1.1\r\n") {
		t.Errorf("legacy request head %q", head)
	}
}

// TestEntryFramingHeadersNotSent checks that Content-Length and
// Transfer-Encoding headers of an entry do not reach the wire: the body is
// framed by the length sonde computes, once.
func TestEntryFramingHeadersNotSent(t *testing.T) {
	var got http.Header
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "POST", URL: srv.URL, Body: Body{Kind: BodyText, Data: []byte("abc")},
		Headers: []exchange.Header{{Name: "Content-Length", Value: "99"}, {Name: "Transfer-Encoding", Value: "chunked"}}}
	calls, err := c.Execute(t.Context(), spec, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	if body != "abc" || got.Get("Transfer-Encoding") != "" {
		t.Errorf("server read %q, headers %v", body, got)
	}
	n := 0
	for _, h := range calls[0].Request.Headers {
		if strings.EqualFold(h.Name, "Content-Length") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d Content-Length headers recorded", n)
	}
}

// TestIDNHost checks that an internationalized host name goes on the
// wire (Host header, recorded request) in its ASCII form.
func TestIDNHost(t *testing.T) {
	for in, want := range map[string]string{
		"bücher.example":    "xn--bcher-kva.example",
		"example.test":      "example.test",
		"[::1]":             "[::1]",
		"bücher.example:81": "xn--bcher-kva.example:81",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://"+in+"/", nil)
		req.Host = ""
		if got := hostHeader(req); got != want {
			t.Errorf("hostHeader(%s) = %s, want %s", in, got, want)
		}
	}
}

// TestAllProxy checks that ALL_PROXY is used when no scheme-specific
// proxy variable is set, and that NO_PROXY still applies.
func TestAllProxy(t *testing.T) {
	proxy := newForwardProxy(t)
	for _, name := range []string{"http_proxy", "HTTP_PROXY", "https_proxy", "HTTPS_PROXY", "no_proxy", "NO_PROXY", "all_proxy"} {
		t.Setenv(name, "")
	}
	t.Setenv("ALL_PROXY", proxy.URL)
	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, "http://example.test/", &Options{})
	if string(calls[0].Response.Body) != "from proxy" {
		t.Errorf("ALL_PROXY not used: %q", calls[0].Response.Body)
	}
	t.Setenv("NO_PROXY", "example.test")
	c = newTestClient(t, ClientConfig{})
	if _, err := c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: "http://example.test/"}, &Options{ConnectTimeout: time.Second}); err == nil {
		t.Error("NO_PROXY ignored: the request went through the proxy")
	}
}

// TestEnvironmentProxyRefused checks that a proxy from the environment
// with a scheme sonde cannot speak is an error, not a plain-HTTP request
// that would send its credentials in clear; and that http_proxy wins over
// ALL_PROXY.
func TestEnvironmentProxyRefused(t *testing.T) {
	for _, name := range []string{"http_proxy", "HTTP_PROXY", "https_proxy", "HTTPS_PROXY", "no_proxy", "NO_PROXY", "all_proxy", "ALL_PROXY"} {
		t.Setenv(name, "")
	}
	for _, value := range []string{"socks4://u:p@127.0.0.1:1080", "socks4a://127.0.0.1:1080"} {
		t.Setenv("ALL_PROXY", value)
		c := newTestClient(t, ClientConfig{})
		_, err := c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: "http://example.test/"}, &Options{})
		var herr *Error
		if !asError(err, &herr) || herr.Kind != ErrInvalidURL {
			t.Errorf("ALL_PROXY=%s: %v", value, err)
		}
	}
	proxy := newForwardProxy(t)
	t.Setenv("ALL_PROXY", "socks4://127.0.0.1:1")
	t.Setenv("http_proxy", proxy.URL)
	c := newTestClient(t, ClientConfig{})
	if calls := get(t, c, "http://example.test/", &Options{}); string(calls[0].Response.Body) != "from proxy" {
		t.Error("http_proxy does not win over ALL_PROXY")
	}
}
