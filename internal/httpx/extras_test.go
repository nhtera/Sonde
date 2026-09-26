// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestErrorMethods(t *testing.T) {
	inner := errors.New("boom")
	e := &Error{Kind: ErrOther, Description: "HTTP connection", Msg: "boom", Err: inner}
	if e.Error() != "HTTP connection: boom" {
		t.Errorf("Error() = %q", e.Error())
	}
	if !errors.Is(e, inner) {
		t.Error("Unwrap does not expose inner error")
	}
}

func TestErrorConstructors(t *testing.T) {
	if err := resolveError("h", errors.New("x")); err.Kind != ErrResolve {
		t.Errorf("resolveError kind = %v", err.Kind)
	}
	if err := fileAccessError("p", errors.New("x")); err.Kind != ErrFileAccess {
		t.Errorf("fileAccessError kind = %v", err.Kind)
	}
	if err := otherError("msg", errors.New("x")); err.Kind != ErrOther || err.Msg != "msg" {
		t.Errorf("otherError = %+v", err)
	}
}

func TestDefaultCookiePath(t *testing.T) {
	cases := map[string]string{
		"":        "/",
		"/":       "/",
		"/a":      "/",
		"/a/b":    "/a",
		"/a/b/":   "/a/b",
		"noslash": "/",
	}
	for in, want := range cases {
		if got := defaultCookiePath(in); got != want {
			t.Errorf("defaultCookiePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCookieDate(t *testing.T) {
	if _, ok := parseCookieDate("Wed, 09 Jun 2027 10:18:14 GMT"); !ok {
		t.Error("RFC1123 date not parsed")
	}
	if _, ok := parseCookieDate("not a date"); ok {
		t.Error("garbage date parsed")
	}
}

func TestCookieJarRemove(t *testing.T) {
	j := &cookieJar{}
	j.store(Cookie{Domain: "e.com", Path: "/", Name: "a", Value: "1"})
	j.remove("e.com", "/", "a")
	if len(j.all()) != 0 {
		t.Error("remove did not delete cookie")
	}
	j.remove("e.com", "/", "missing") // no-op, must not panic
}

func setCookieServer(t *testing.T, setCookie string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", setCookie)
		w.WriteHeader(200)
	}))
}

func hostOnly(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname()
}

func TestCookieMaxAgeZeroRemoves(t *testing.T) {
	srv := setCookieServer(t, "a=1; Max-Age=0")
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	c.jar.store(Cookie{Domain: hostOnly(t, srv.URL), Path: "/", Name: "a", Value: "old"})
	get(t, c, srv.URL, nil)
	if len(c.Cookies()) != 0 {
		t.Errorf("Max-Age=0 did not remove cookie: %+v", c.Cookies())
	}
}

func TestCookieExpiresPastRemoves(t *testing.T) {
	srv := setCookieServer(t, "a=1; Expires=Thu, 01 Jan 1970 00:00:00 GMT")
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	c.jar.store(Cookie{Domain: hostOnly(t, srv.URL), Path: "/", Name: "a", Value: "old"})
	get(t, c, srv.URL, nil)
	if len(c.Cookies()) != 0 {
		t.Errorf("past Expires did not remove cookie: %+v", c.Cookies())
	}
}

func TestCookiePublicSuffixRejected(t *testing.T) {
	srv := setCookieServer(t, "a=1; Domain=com")
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	get(t, c, srv.URL, nil)
	if len(c.Cookies()) != 0 {
		t.Errorf("cookie scoped to public suffix accepted: %+v", c.Cookies())
	}
}

func TestCookieDomainMismatchRejected(t *testing.T) {
	srv := setCookieServer(t, "a=1; Domain=totally-different.example")
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	get(t, c, srv.URL, nil)
	if len(c.Cookies()) != 0 {
		t.Errorf("cookie for unrelated domain accepted: %+v", c.Cookies())
	}
}

func TestTLSVersionName(t *testing.T) {
	cases := map[uint16]string{
		0x0304: "TLSv1.3",
		0x0303: "TLSv1.2",
		0x0302: "TLSv1.1",
		0x0301: "TLSv1.0",
		0x9999: "TLS",
	}
	for v, want := range cases {
		if got := tlsVersionName(v); got != want {
			t.Errorf("tlsVersionName(%x) = %q, want %q", v, got, want)
		}
	}
}

func TestDefaultNetrcPath(t *testing.T) {
	t.Setenv("NETRC", "/custom/netrc")
	if got := defaultNetrcPath(); got != "/custom/netrc" {
		t.Errorf("defaultNetrcPath() = %q", got)
	}
}

func TestSpkiHashFromPEMCert(t *testing.T) {
	cert, _, certPEMBytes := genCert(t, "spki-test", true, nil, nil)
	h, err := spkiHashFromFile(certPEMBytes)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	if h != want {
		t.Error("SPKI hash mismatch")
	}
}

func TestSpkiHashFromInvalidFile(t *testing.T) {
	if _, err := spkiHashFromFile([]byte("not a cert")); err == nil {
		t.Error("expected error for garbage input")
	}
}

func TestRateLimitedReaderClose(t *testing.T) {
	rc := io.NopCloser(strings.NewReader("hello"))
	lr := newRateLimitedReader(context.Background(), rc, 1000).(*rateLimitedReader)
	if err := lr.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestEffectivePort(t *testing.T) {
	u, _ := url.Parse("https://example.com/")
	if effectivePort(u) != "443" {
		t.Errorf("effectivePort(https) = %q", effectivePort(u))
	}
	u2, _ := url.Parse("http://example.com:8080/")
	if effectivePort(u2) != "8080" {
		t.Errorf("effectivePort(explicit) = %q", effectivePort(u2))
	}
}

func TestRemoveDotSegments(t *testing.T) {
	cases := map[string]string{
		"/a/./b":  "/a/b",
		"/a/../b": "/b",
		"/a/b/..": "/a/",
		"/../a":   "/a",
		"":        "",
		"/a/b/./": "/a/b/",
	}
	for in, want := range cases {
		if got := removeDotSegments(in); got != want {
			t.Errorf("removeDotSegments(%q) = %q, want %q", in, got, want)
		}
	}
}
