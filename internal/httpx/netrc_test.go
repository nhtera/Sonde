// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNetrc(t *testing.T) {
	data := "machine example.com login alice password secret\n" +
		"machine other.com login bob password hunter2\n" +
		"default login guest password guestpw\n"
	f := parseNetrc(data)
	login, pass, ok := f.lookup("example.com")
	if !ok || login != "alice" || pass != "secret" {
		t.Errorf("example.com = %q %q %v", login, pass, ok)
	}
	login, pass, ok = f.lookup("unknown.com")
	if !ok || login != "guest" || pass != "guestpw" {
		t.Errorf("default fallback = %q %q %v", login, pass, ok)
	}
}

func TestNetrcAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "alice" || p != "secret" {
			t.Errorf("BasicAuth = %q %q %v", u, p, ok)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	host, _, _ := splitHostPortT(t, srv.URL)
	dir := t.TempDir()
	path := filepath.Join(dir, "netrc")
	content := "machine " + host + " login alice password secret\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, ClientConfig{})
	get(t, c, srv.URL, &Options{Netrc: true, NetrcFile: path, LocalFiles: map[string]bool{path: true}})
}

func TestNetrcOptionalMissingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			t.Error("unexpected BasicAuth with no netrc file")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL},
		&Options{NetrcOptional: true, NetrcFile: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("NetrcOptional should not fail on missing file: %v", err)
	}
}

func TestNetrcRerouteGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			t.Error("netrc credentials sent despite reroute without NetrcAllowReroute")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	_, port, _ := splitHostPortT(t, srv.URL)

	dir := t.TempDir()
	path := filepath.Join(dir, "netrc")
	content := "machine fake.invalid login alice password secret\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var warnings []string
	c := newTestClient(t, ClientConfig{Warn: func(msg string) { warnings = append(warnings, msg) }})
	defer func() {
		if len(warnings) != 1 || !strings.Contains(warnings[0], "--netrc-allow-reroute") {
			t.Errorf("warnings = %q, want one about --netrc-allow-reroute", warnings)
		}
	}()
	get(t, c, "http://fake.invalid:"+port+"/", &Options{
		Netrc: true, NetrcFile: path, LocalFiles: map[string]bool{path: true},
		Resolve: []string{"fake.invalid:" + port + ":127.0.0.1"},
	})
}

func splitHostPortT(t *testing.T, rawURL string) (host, port string, ok bool) {
	t.Helper()
	// srv.URL is like "http://127.0.0.1:PORT"; strip the scheme.
	u := rawURL
	for _, p := range []string{"http://", "https://"} {
		if len(u) > len(p) && u[:len(p)] == p {
			u = u[len(p):]
			break
		}
	}
	for i := len(u) - 1; i >= 0; i-- {
		if u[i] == ':' {
			return u[:i], u[i+1:], true
		}
	}
	return u, "", false
}
