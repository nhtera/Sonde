// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCookieJarSetAndSend(t *testing.T) {
	var mux http.ServeMux
	var hits int
	mux.HandleFunc("/set", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc123", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // plain http.testing server; Secure would stop the round trip this test checks
		w.WriteHeader(200)
	})
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		hits++
		c, err := r.Cookie("sid")
		if err != nil || c.Value != "abc123" {
			t.Errorf("cookie sid missing: %v", err)
		}
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	get(t, c, srv.URL+"/set", nil)
	get(t, c, srv.URL+"/get", nil)
	if hits != 1 {
		t.Fatalf("hits = %d", hits)
	}
	cookies := c.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "sid" || cookies[0].Value != "abc123" {
		t.Fatalf("Cookies() = %+v", cookies)
	}
}

func TestCookieJarSecureFlag(t *testing.T) {
	now := time.Now()
	j := &cookieJar{}
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "s", Value: "1", HTTPS: true})
	u, _ := url.Parse("http://example.com/")
	if got := j.forRequest(u, now); len(got) != 0 {
		t.Errorf("secure cookie sent over http: %+v", got)
	}
	u2, _ := url.Parse("https://example.com/")
	if got := j.forRequest(u2, now); len(got) != 1 {
		t.Errorf("secure cookie not sent over https: %+v", got)
	}
}

func TestCookieJarDomainMatch(t *testing.T) {
	now := time.Now()
	j := &cookieJar{}
	j.store(Cookie{Domain: "example.com", IncludeSubdomain: false, Path: "/", Name: "a", Value: "1"})
	j.store(Cookie{Domain: "example.com", IncludeSubdomain: true, Path: "/", Name: "b", Value: "2"})

	cases := []struct {
		host    string
		wantLen int
	}{
		{"example.com", 2},
		{"sub.example.com", 1}, // only the includeSubdomain cookie
		{"other.com", 0},
	}
	for _, tt := range cases {
		u, _ := url.Parse("http://" + tt.host + "/")
		got := j.forRequest(u, now)
		if len(got) != tt.wantLen {
			t.Errorf("%s: got %d cookies, want %d (%+v)", tt.host, len(got), tt.wantLen, got)
		}
	}
}

func TestCookieJarPathMatch(t *testing.T) {
	now := time.Now()
	j := &cookieJar{}
	j.store(Cookie{Domain: "example.com", Path: "/foo", Name: "a", Value: "1"})

	cases := []struct {
		path    string
		wantLen int
	}{
		{"/foo", 1},
		{"/foo/bar", 1},
		{"/foobar", 0},
		{"/", 0},
	}
	for _, tt := range cases {
		u, _ := url.Parse("http://example.com" + tt.path)
		got := j.forRequest(u, now)
		if len(got) != tt.wantLen {
			t.Errorf("%s: got %d, want %d", tt.path, len(got), tt.wantLen)
		}
	}
}

func TestCookieJarExpiry(t *testing.T) {
	now := time.Now()
	j := &cookieJar{}
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "session", Value: "1", Expires: 0})
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "expired", Value: "1", Expires: now.Add(-time.Hour).Unix()})
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "future", Value: "1", Expires: now.Add(time.Hour).Unix()})

	u, _ := url.Parse("http://example.com/")
	got := j.forRequest(u, now)
	names := map[string]bool{}
	for _, c := range got {
		names[c.Name] = true
	}
	if !names["session"] || !names["future"] || names["expired"] {
		t.Errorf("got = %+v", got)
	}
}

func TestCookieJarUpdateInPlace(t *testing.T) {
	j := &cookieJar{}
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "a", Value: "1"})
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "b", Value: "2"})
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "a", Value: "updated"})

	all := j.all()
	if len(all) != 2 {
		t.Fatalf("all = %+v", all)
	}
	if all[0].Name != "a" || all[0].Value != "updated" {
		t.Errorf("update did not keep position: %+v", all)
	}
}

func TestCookieJarClear(t *testing.T) {
	j := &cookieJar{}
	j.store(Cookie{Domain: "example.com", Path: "/", Name: "a", Value: "1"})
	j.clear()
	if len(j.all()) != 0 {
		t.Error("clear did not empty jar")
	}
}

func TestClientAddCookieAndClear(t *testing.T) {
	c := newTestClient(t, ClientConfig{})
	if err := c.AddCookie("example.com\tFALSE\t/\tFALSE\t0\tfoo\tbar"); err != nil {
		t.Fatal(err)
	}
	cookies := c.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "foo" || cookies[0].Value != "bar" {
		t.Fatalf("cookies = %+v", cookies)
	}
	c.ClearCookies()
	if len(c.Cookies()) != 0 {
		t.Error("ClearCookies did not empty jar")
	}
}

func TestClientAddCookieInvalid(t *testing.T) {
	c := newTestClient(t, ClientConfig{})
	if err := c.AddCookie("not a cookie"); err == nil {
		t.Error("expected error for malformed cookie")
	}
}

func TestNetscapeRoundTrip(t *testing.T) {
	c := Cookie{
		Domain: "example.com", IncludeSubdomain: true, Path: "/a",
		HTTPS: true, Expires: 1893456000, Name: "n", Value: "v", HTTPOnly: true,
	}
	line := formatNetscapeCookie(c)
	got, ok := parseNetscapeCookie(line)
	if !ok {
		t.Fatalf("parse failed for %q", line)
	}
	if got != c {
		t.Errorf("round trip = %+v, want %+v", got, c)
	}
}

func TestCookieFileLoadedAtStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")
	content := "# Netscape HTTP Cookie File\nexample.com\tFALSE\t/\tFALSE\t0\tfoo\tbar\n#HttpOnly_example.com\tFALSE\t/\tFALSE\t0\tsecret\tvalue\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, ClientConfig{CookieFile: path})
	cookies := c.Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies = %+v", cookies)
	}
	var sawHTTPOnly bool
	for _, ck := range cookies {
		if ck.Name == "secret" {
			sawHTTPOnly = ck.HTTPOnly
		}
	}
	if !sawHTTPOnly {
		t.Error("HttpOnly prefix not parsed")
	}
}

func TestNoCookieStore(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/set", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // plain http.testing server; Secure would stop the round trip this test checks
		w.WriteHeader(200)
	})
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sid"); err == nil {
			t.Error("cookie sent despite NoCookieStore")
		}
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	c := newTestClient(t, ClientConfig{NoCookieStore: true})
	get(t, c, srv.URL+"/set", nil)
	get(t, c, srv.URL+"/get", nil)
	if len(c.Cookies()) != 0 {
		t.Error("cookie stored despite NoCookieStore")
	}
}

func TestCookieRequestSectionAddsToJar(t *testing.T) {
	var mux http.ServeMux
	var seen string
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Cookie")
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	c.jar.store(Cookie{Domain: "127.0.0.1", Path: "/", Name: "a", Value: "from-jar"})
	spec := &RequestSpec{Method: "GET", URL: srv.URL + "/", Cookies: []RequestCookie{{Name: "a", Value: "from-section"}, {Name: "b", Value: "new"}}}
	_, err := c.Execute(context.Background(), spec, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	if seen != "a=from-jar; a=from-section; b=new" {
		t.Errorf("Cookie header = %q", seen)
	}
}

// Cookies are sent in curl's order: longer path first, then the most
// recently stored.
func TestCookieOrder(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Cookie")
	}))
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	for _, ck := range []Cookie{{Name: "c1", Path: "/"}, {Name: "c2", Path: "/"}, {Name: "c3", Path: "/a"}, {Name: "c4", Path: "/"}} {
		ck.Domain, ck.Value = "127.0.0.1", "v"
		c.jar.store(ck)
	}
	if _, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL + "/a/b"}, &Options{}); err != nil {
		t.Fatal(err)
	}
	if seen != "c3=v; c4=v; c2=v; c1=v" {
		t.Errorf("Cookie header = %q", seen)
	}
}
