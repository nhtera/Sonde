// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/netpolicy"
)

// TestRunHosts covers a runner with a host allowlist: allowed and denied
// URLs, and the entry options refused before any connection is made.
func TestRunHosts(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	policy, err := netpolicy.Parse([]string{host})
	if err != nil {
		t.Fatal(err)
	}
	runSrc := func(t *testing.T, src string) *UnitResult {
		t.Helper()
		r := NewRunner(Options{Variables: map[string]any{"base": srv.URL}})
		enginex.SetHosts(r, policy)
		res, err := r.RunSource(context.Background(), filepath.Join(t.TempDir(), "t.hurl"), []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	if res := runSrc(t, "GET {{base}}/\nHTTP 200\n"); !res.Success {
		t.Fatalf("allowed host failed: %v", res.Errors())
	}
	res := runSrc(t, "GET http://denied.test/\nHTTP 200\n")
	if res.Success || !strings.Contains(res.Errors()[0].Error(), "not in the host allowlist") {
		t.Fatalf("denied host: %v", res.Errors())
	}

	for _, opt := range []string{
		"proxy: " + host, "connect-to: a:1:b:2", "resolve: a:1:127.0.0.1", "unix-socket: s",
		"netrc: true", "netrc-file: n", "netrc-optional: true", "output: out.txt", "output: -",
	} {
		before := hits.Load()
		res := runSrc(t, "GET {{base}}/\n[Options]\n"+opt+"\nHTTP 200\n")
		errs := res.Errors()
		if res.Success || len(errs) != 1 || !strings.Contains(errs[0].Error(), "is not available with a host allowlist") {
			t.Errorf("%s: success=%v errors=%v", opt, res.Success, errs)
		}
		if hits.Load() != before {
			t.Errorf("%s: the request was sent", opt)
		}
	}
}
