// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

// TestRedirectCredentialMatrix checks every kind of credential against
// every redirect path: they reach the original scheme, host and port
// only (a redirect back to it sends them again), unless location-trusted
// sends them everywhere. The server is 127.0.0.1; the second host is the
// same server named localhost, as in the reference's tests.
func TestRedirectCredentialMatrix(t *testing.T) {
	type seen struct{ auth, cookie string }
	var mu sync.Mutex
	got := map[string]seen{}
	var srv *httptest.Server
	other := func() string { return strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) }
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got[r.URL.Path] = seen{r.Header.Get("Authorization"), r.Header.Get("Cookie")}
		mu.Unlock()
		switch r.URL.Path {
		case "/same":
			w.Header().Set("Location", "/same-end")
		case "/cross":
			w.Header().Set("Location", other()+"/cross-end")
		case "/a1":
			w.Header().Set("Location", other()+"/b2")
		case "/b2":
			w.Header().Set("Location", srv.URL+"/a3")
		default:
			return
		}
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	if !strings.Contains(srv.URL, "127.0.0.1") {
		t.Skip("the test server is not on 127.0.0.1, so localhost is no second host name")
	}

	credentials := []struct {
		name       string
		spec       func(*RequestSpec)
		opts       func(*Options)
		auth, cook bool
	}{
		{"Authorization header", func(s *RequestSpec) { s.Headers = []exchange.Header{{Name: "Authorization", Value: "Bearer e"}} }, nil, true, false},
		{"Cookie header", nil, func(o *Options) { o.Headers = []exchange.Header{{Name: "Cookie", Value: "h=1"}} }, false, true},
		{"[Cookies]", func(s *RequestSpec) { s.Cookies = []RequestCookie{{Name: "c", Value: "1"}} }, nil, false, true},
		{"--user", nil, func(o *Options) { o.User = "alice:pw" }, true, false},
		{"URL userinfo", func(s *RequestSpec) { s.URL = strings.Replace(s.URL, "://", "://alice:pw@", 1) }, nil, true, false},
	}
	paths := []struct {
		start string
		// sent and stripped list the paths that must, and must not, get
		// the credential (untrusted).
		sent, stripped []string
	}{
		{"/same", []string{"/same", "/same-end"}, nil},
		{"/cross", []string{"/cross"}, []string{"/cross-end"}},
		{"/a1", []string{"/a1", "/a3"}, []string{"/b2"}},
	}
	c := newTestClient(t, ClientConfig{})
	for _, cred := range credentials {
		for _, p := range paths {
			for _, trusted := range []bool{false, true} {
				spec := &RequestSpec{Method: "GET", URL: srv.URL + p.start}
				opts := &Options{FollowLocation: true, LocationTrusted: trusted}
				if cred.spec != nil {
					cred.spec(spec)
				}
				if cred.opts != nil {
					cred.opts(opts)
				}
				mu.Lock()
				clear(got)
				mu.Unlock()
				if _, err := c.Execute(t.Context(), spec, opts); err != nil {
					t.Fatal(err)
				}
				has := func(path string) bool {
					mu.Lock()
					defer mu.Unlock()
					s, ok := got[path]
					if !ok {
						t.Fatalf("%s %s: %s not requested", cred.name, p.start, path)
					}
					return (cred.auth && s.auth != "") || (cred.cook && s.cookie != "")
				}
				for _, path := range p.sent {
					if !has(path) {
						t.Errorf("%s, from %s, trusted=%v: not sent to %s", cred.name, p.start, trusted, path)
					}
				}
				for _, path := range p.stripped {
					if has(path) != trusted {
						t.Errorf("%s, from %s, trusted=%v: sent to %s = %v", cred.name, p.start, trusted, path, !trusted)
					}
				}
			}
		}
	}
}
