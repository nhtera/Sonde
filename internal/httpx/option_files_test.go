// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/internal/sandbox"
)

// Files named by request-file options (not command line paths) are read
// through the sandbox: outside the file root they are refused (a pinned
// key that cannot be read fails as a mismatch).
func TestOptionFilesOutsideRootDenied(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	outside := filepath.Join(t.TempDir(), "file.pem")
	if err := os.WriteFile(outside, certPEM(t, srv), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, ClientConfig{})
	for name, tc := range map[string]struct {
		opts *Options
		want ErrorKind
	}{
		"cacert":     {&Options{CACert: outside}, ErrFileAccess},
		"cert":       {&Options{Insecure: true, ClientCert: outside, ClientKey: outside}, ErrFileAccess},
		"pin":        {&Options{Insecure: true, PinnedPublicKey: outside}, ErrTLS},
		"netrc-file": {&Options{NetrcFile: outside}, ErrFileAccess},
	} {
		_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL}, tc.opts)
		var herr *Error
		if !asError(err, &herr) || herr.Kind != tc.want {
			t.Errorf("%s: err = %v, want kind %d", name, err, tc.want)
		}
	}
}

// Each netrc file is loaded on its own: a second file is not shadowed by
// the first one.
func TestNetrcCachedPerFile(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		u, _, _ := r.BasicAuth()
		got = append(got, u)
	}))
	defer srv.Close()
	host, _, _ := splitHostPortT(t, srv.URL)
	box, err := sandbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	for _, u := range []string{"a", "b"} {
		writeFile(t, box, u, []byte("machine "+host+" login "+u+" password p\n"))
	}
	c, err := NewClient(ClientConfig{Sandbox: box, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, u := range []string{"a", "b"} {
		get(t, c, srv.URL, &Options{NetrcFile: filepath.Join(box.Dir(), u)})
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("logins = %q, want [a b]", got)
	}
}
