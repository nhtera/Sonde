// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2EErrorFormatLongRedacts checks that --error-format long redacts a
// secret that appears in the response header and body it prints (H4):
// unlike the upstream CLI, that sink must never carry a raw secret either.
func TestE2EErrorFormatLongRedacts(t *testing.T) {
	const secret = "errfmt-secret-9d21ab" //nolint:gosec // G101: a fake test value, not a credential
	mux := http.NewServeMux()
	mux.HandleFunc("/errfmt", func(w http.ResponseWriter, r *http.Request) {
		in := r.Header.Get("X-In")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Echo", in)
		_, _ = w.Write([]byte("body has " + in + " inside")) //nolint:gosec // G705: a local test server echoing its own header, not XSS
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTemp(t, "errfmt.hurl", "GET {{host}}/errfmt\nX-In: {{s}}\nHTTP 200\n[Asserts]\nstatus == 404\n")
	code, _, errOut := runArgs(t, file, "--error-format", "long",
		"--variable", "host="+srv.URL, "--secret", "s="+secret)
	if code != ExitAssert {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitAssert, errOut)
	}
	if strings.Contains(errOut, secret) {
		t.Errorf("--error-format long leaks the secret:\n%s", errOut)
	}
	if !strings.Contains(errOut, "***") {
		t.Errorf("stderr has no redaction marker; the test may not exercise the header/body it should:\n%s", errOut)
	}
}

// TestE2EJSONBytesCaptureRedacted checks that a `bytes` capture whose raw
// bytes are exactly a registered secret's own bytes comes out redacted in
// --json, not as a plain base64 string (L7).
func TestE2EJSONBytesCaptureRedacted(t *testing.T) {
	const secret = "bytes-secret-71fa02" //nolint:gosec // G101: a fake test value, not a credential
	mux := http.NewServeMux()
	mux.HandleFunc("/bytes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(secret))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTemp(t, "bytes.hurl", "GET {{host}}/bytes\nHTTP 200\n[Captures]\nraw: bytes\n")
	code, out, errOut := runArgs(t, file, "--json", "--variable", "host="+srv.URL, "--secret", "s="+secret)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if strings.Contains(out, secret) {
		t.Errorf("--json leaks the secret in a bytes capture:\n%s", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("--json has no redaction marker; the test may not exercise the bytes capture:\n%s", out)
	}
}

// TestE2ESetupErrorRedacted checks that a setup error (one RunSource
// itself returns, before any entry runs — here an unreadable --cookie
// file whose path contains a secret) is redacted the same way an entry
// error is (L8).
func TestE2ESetupErrorRedacted(t *testing.T) {
	const secret = "setup-secret-3c88f1"                               //nolint:gosec // G101: a fake test value, not a credential
	badCookiePath := filepath.Join(t.TempDir(), secret, "cookies.txt") // parent dir does not exist
	file := writeTemp(t, "setup.hurl", "GET http://127.0.0.1:1/hello\nHTTP 200\n")

	code, _, errOut := runArgs(t, file, "--cookie", badCookiePath, "--secret", "s="+secret)
	if code != ExitRuntime {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitRuntime, errOut)
	}
	if strings.Contains(errOut, secret) {
		t.Errorf("the setup error leaks the secret:\n%s", errOut)
	}
	if !strings.Contains(errOut, "***") {
		t.Errorf("stderr has no redaction marker; the test may not exercise the setup-error path:\n%s", errOut)
	}
}
