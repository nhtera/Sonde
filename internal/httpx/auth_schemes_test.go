// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"crypto/md5" //nolint:gosec // G501: the Digest scheme's algorithm, recomputed independently
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

func md5hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // G401: see the import
	return hex.EncodeToString(sum[:])
}

// TestDigest checks a Digest exchange: the challenge is answered once with
// a correct response (recomputed here from RFC 7616), only the final
// exchange is reported, an endless 401 ends after one answer, and the
// response is passed to Secret.
func TestDigest(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Digest ") && r.URL.Path == "/ok" {
			params := map[string]string{}
			for _, m := range regexp.MustCompile(`(\w+)=(?:"([^"]*)"|([^,]*))`).FindAllStringSubmatch(auth, -1) {
				params[m[1]] = m[2] + m[3]
			}
			ha1 := md5hex("user:realm:pw")
			ha2 := md5hex(r.Method + ":" + params["uri"])
			want := md5hex(ha1 + ":n0nce:" + params["nc"] + ":" + params["cnonce"] + ":auth:" + ha2)
			if params["response"] == want && params["uri"] == "/ok?q=1" && params["opaque"] == "op" {
				_, _ = io.WriteString(w, "authed")
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Digest realm="realm", nonce="n0nce", opaque="op", qop="auth"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	calls, err := c.Execute(t.Context(), &RequestSpec{Method: "POST", URL: srv.URL + "/ok?q=1", Body: Body{Kind: BodyText, Data: []byte("b")}},
		&Options{Digest: true, User: "user:pw"})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Response.Status != 200 || string(calls[0].Response.Body) != "authed" {
		t.Fatalf("calls %d, final %d %q", len(calls), calls[0].Response.Status, calls[0].Response.Body)
	}
	if auth, _ := exchange.Headers(calls[0].Request.Headers).Get("Authorization"); auth != "***" {
		t.Errorf("recorded Authorization not masked (%d bytes)", len(auth))
	}
	// Credentials in the URL are the scheme's, never Basic.
	calls, err = c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: strings.Replace(srv.URL, "://", "://user:pw@", 1) + "/ok?q=1"}, &Options{Digest: true})
	if err != nil || calls[0].Response.Status != 200 {
		t.Errorf("URL credentials: %v, status %d", err, calls[0].Response.Status)
	}
	mu.Lock()
	requests = 0
	mu.Unlock()
	calls, err = c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: srv.URL + "/never"}, &Options{Digest: true, User: "user:pw"})
	if err != nil || calls[0].Response.Status != 401 || requests != 2 {
		t.Errorf("endless 401: %v, status %d, %d requests", err, calls[0].Response.Status, requests)
	}
}

// TestNTLM checks that the NTLM exchange runs on one connection, that
// the authenticated connection is not reused afterwards, and that it
// works with HTTP/2 asked for (it forces HTTP/1.1).
func TestNTLM(t *testing.T) {
	var mu sync.Mutex
	var remotes []string
	challenge := "TlRMTVNTUAACAAAAAwAMADgAAAAzgoriASNFZ4mrze8AAAAAAAAAACQAJABEAAAABgBwFwAAAA9TAGUAcgB2AGUAcgACAAwARABvAG0AYQBpAG4AAQAMAFMAZQByAHYAZQByAAAAAAA="
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		remotes = append(remotes, r.RemoteAddr)
		mu.Unlock()
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "NTLM ")
		msg, _ := base64.StdEncoding.DecodeString(token)
		switch {
		case len(msg) > 8 && msg[8] == 1:
			w.Header().Set("WWW-Authenticate", "NTLM "+challenge)
			w.WriteHeader(http.StatusUnauthorized)
		case len(msg) > 8 && msg[8] == 3:
			_, _ = io.WriteString(w, "authed")
		default:
			w.Header().Set("WWW-Authenticate", "NTLM")
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	calls, err := c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: srv.URL}, &Options{NTLM: true, User: `domain\user:pw`, HTTPVersion: HTTP2})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || string(calls[0].Response.Body) != "authed" {
		t.Fatalf("calls %d, %q", len(calls), calls[0].Response.Body)
	}
	get(t, c, srv.URL, nil)
	mu.Lock()
	defer mu.Unlock()
	if len(remotes) != 3 || remotes[0] != remotes[1] || remotes[2] == remotes[1] {
		t.Errorf("connections %v: the exchange shares one, the next request gets another", remotes)
	}
}

// TestAWSSigV4 checks the signed request: its headers, the credential
// scope from the option, and the signature passed to Secret.
func TestAWSSigV4(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "POST", URL: srv.URL + "/aws-sigv4", Form: []Param{{Name: "test", Value: "test"}},
		Headers: []exchange.Header{{Name: "X-Amz-Security-Token", Value: "tok"}}}
	calls, err := c.Execute(t.Context(), spec, &Options{AWSSigV4: "aws:amz:eu-central-1:hurltest", User: "someAccessKeyId:someSecretKey"})
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^AWS4-HMAC-SHA256 Credential=someAccessKeyId/\d{8}/eu-central-1/hurltest/aws4_request, SignedHeaders=(\S+), Signature=[a-f0-9]{64}$`)
	m := re.FindStringSubmatch(got.Get("Authorization"))
	if m == nil {
		t.Fatalf("Authorization %q", got.Get("Authorization"))
	}
	if m[1] != "content-type;host;user-agent;x-amz-date;x-amz-security-token" {
		t.Errorf("signed headers %s", m[1])
	}
	if got.Get("X-Amz-Date") == "" || got.Get("X-Amz-Security-Token") != "tok" || len(got.Values("Authorization")) != 1 {
		t.Errorf("headers %v", got)
	}
	if auth, _ := exchange.Headers(calls[0].Request.Headers).Get("Authorization"); auth != "***" {
		t.Errorf("recorded Authorization not masked")
	}
	// A Host header of the entry is the one signed (once).
	spec.Headers = []exchange.Header{{Name: "Host", Value: "service.test"}}
	if _, err := c.Execute(t.Context(), spec, &Options{AWSSigV4: "aws:amz:eu-central-1:hurltest", User: "a:b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Get("Authorization"), "SignedHeaders=content-type;host;user-agent;x-amz-date,") {
		t.Errorf("signed headers with a Host: %s", got.Get("Authorization"))
	}
}

// TestSchemesStayWithTheOriginalHost checks that no scheme authenticates
// a redirect to another host.
func TestSchemesStayWithTheOriginalHost(t *testing.T) {
	var gotAuth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("WWW-Authenticate", `Digest realm="r", nonce="n"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, other.URL+"/x", http.StatusFound)
	}))
	defer first.Close()
	c := newTestClient(t, ClientConfig{})
	for name, opts := range map[string]Options{
		"digest":    {Digest: true, User: "u:p"},
		"aws-sigv4": {AWSSigV4: "aws:amz:r:s", User: "a:b"},
	} {
		gotAuth = "unset"
		opts.FollowLocation = true
		if _, err := c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: first.URL}, &opts); err != nil {
			t.Fatal(err)
		}
		if gotAuth != "" {
			t.Errorf("%s: other host got Authorization %s", name, fmt.Sprint(len(gotAuth))+" bytes")
		}
	}
}

// TestBoundAuthRefused checks that NTLM and Negotiate are refused where
// their connection could not be held: through a SOCKS proxy, for gRPC,
// and for the WebSocket handshake.
func TestBoundAuthRefused(t *testing.T) {
	c := newTestClient(t, ClientConfig{})
	for name, opts := range map[string]Options{
		"socks proxy": {NTLM: true, Proxy: "socks5://127.0.0.1:1"},
		"grpc":        {Negotiate: true, GRPC: true},
	} {
		_, err := c.Execute(t.Context(), &RequestSpec{Method: "GET", URL: "http://example.test/"}, &opts)
		var herr *Error
		if !asError(err, &herr) || herr.Kind != ErrUnsupported {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := c.Upgrade(t.Context(), &RequestSpec{Method: "GET", URL: "ws://example.test/"}, &Options{Digest: true}); err == nil {
		t.Error("digest WebSocket handshake accepted")
	}
}
