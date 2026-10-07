// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// headersServer answers with the request's User-Agent and Accept headers
// (or "-" when absent) and a JSON body.
func headersServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, accept := "-", "-"
		if v, ok := r.Header["User-Agent"]; ok {
			ua = v[0]
		}
		if v, ok := r.Header["Accept"]; ok {
			accept = v[0]
		}
		w.Header().Set("X-UA", ua)
		w.Header().Set("X-Accept", accept)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"a":1}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeRequestFile(t *testing.T, content string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "t.hurl")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// TestFailWithBodyFlag checks --fail-with-body and HURL_FAIL_WITH_BODY,
// in test mode too: the body of the failed entry goes to stdout,
// followed by a newline.
func TestFailWithBodyFlag(t *testing.T) {
	srv := headersServer(t)
	file := writeRequestFile(t, "GET "+srv.URL+"\nHTTP 200\n[Asserts]\njsonpath \"$.a\" == 2\n")
	for _, args := range [][]string{{"--fail-with-body", file}, {"--test", "--fail-with-body", file}} {
		code, out, errOut := runArgs(t, args...)
		if code != ExitAssert || out != "{\"a\":1}\n" || !strings.Contains(errOut, "Assert failure") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
	t.Setenv("HURL_FAIL_WITH_BODY", "1")
	if code, out, _ := runArgs(t, file); code != ExitAssert || out != "{\"a\":1}\n" {
		t.Errorf("HURL_FAIL_WITH_BODY: exit %d, stdout %q", code, out)
	}
}

// TestNoHeaderFlag checks that --no-header and HURL_NO_HEADER remove the
// default headers, and that an empty name is a usage error.
func TestNoHeaderFlag(t *testing.T) {
	srv := headersServer(t)
	file := writeRequestFile(t, "GET "+srv.URL+"\nHTTP 200\n[Asserts]\nheader \"X-UA\" == \"-\"\nheader \"X-Accept\" == \"-\"\n")
	if code, _, errOut := runArgs(t, "--no-header", "user-agent", "--no-header", "Accept", file); code != ExitOK {
		t.Errorf("--no-header: exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := runArgs(t, "--no-header", "foo", "--no-header", "", file); code != ExitUsage || !strings.Contains(errOut, "Missing header name") {
		t.Errorf("empty --no-header: exit %d, stderr %q", code, errOut)
	}
	t.Setenv("HURL_NO_HEADER", "user-agent |Accept")
	if code, _, errOut := runArgs(t, file); code != ExitOK {
		t.Errorf("HURL_NO_HEADER: exit %d, stderr %q", code, errOut)
	}
	t.Setenv("HURL_NO_HEADER", "foo|")
	if code, _, errOut := runArgs(t, file); code != ExitUsage || !strings.Contains(errOut, "Missing header name (HURL_NO_HEADER environment variable)") {
		t.Errorf("empty HURL_NO_HEADER name: exit %d, stderr %q", code, errOut)
	}
}

// TestNoJSONPathCoercionFlag checks --no-jsonpath-coercion.
func TestNoJSONPathCoercionFlag(t *testing.T) {
	srv := headersServer(t)
	file := writeRequestFile(t, "GET "+srv.URL+"\nHTTP 200\n[Asserts]\njsonpath \"$.a\" count == 1\njsonpath \"$.b\" count == 0\n")
	if code, _, errOut := runArgs(t, "--no-jsonpath-coercion", file); code != ExitOK {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if code, _, _ := runArgs(t, file); code != ExitAssert {
		t.Errorf("with coercion: exit %d, want %d", code, ExitAssert)
	}
}

// TestProxyHeaderFlagValidation checks the message of a proxy header
// without a colon.
func TestProxyHeaderFlagValidation(t *testing.T) {
	file := writeRequestFile(t, "GET http://localhost:1\n")
	if code, _, errOut := runArgs(t, "--proxy-header", "nocolon", file); code != ExitUsage || !strings.Contains(errOut, "Invalid proxy header <nocolon>, missing `:`") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}
