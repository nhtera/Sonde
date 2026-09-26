// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestE2EBufferedLogsFollowParallelNotJobCount checks code-reviewer finding
// #16: whether a `redact` capture is allowed alongside verbose logging
// depends on the reference CLI's parallel-vs-sequential runner choice
// (--test or --parallel), not on --jobs, since even `--jobs 1` still uses
// the buffered parallel runner once --test is given.
func TestE2EBufferedLogsFollowParallelNotJobCount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Token", "sekret-token-9f21")
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTemp(t, "redact.hurl",
		"GET "+srv.URL+"/token\nHTTP 200\n[Captures]\ntok: header \"X-Token\" redact\n")

	t.Run("sequential run rejects a redact capture in verbose mode, even at --jobs 1", func(t *testing.T) {
		code, _, errOut := runArgs(t, file, "--jobs", "1", "--very-verbose")
		if code != ExitRuntime {
			t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitRuntime, errOut)
		}
		if !strings.Contains(errOut, "Invalid redacted secret") {
			t.Errorf("stderr missing the redact rejection:\n%s", errOut)
		}
	})

	t.Run("--test allows a redact capture in verbose mode, even at --jobs 1", func(t *testing.T) {
		code, _, errOut := runArgs(t, file, "--jobs", "1", "--test", "--very-verbose")
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
		}
	})

	t.Run("--parallel (without --test) also allows it, even at --jobs 1", func(t *testing.T) {
		code, _, errOut := runArgs(t, file, "--jobs", "1", "--parallel", "--very-verbose")
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
		}
	})
}

// TestE2ETestJobsVerboseRedactCaptureFlushes is the regression test for
// code-reviewer finding #16 (2026-09-26): `--test --jobs 2 -v` with a
// `redact` capture must not only be accepted (not rejected with
// "Invalid redacted secret"), but the flushed stderr must have no raw
// captured value — redaction must be applied at flush time, not just at
// capture or storage.
func TestE2ETestJobsVerboseRedactCaptureFlushes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/secrets", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"api_key":"captured-secret-4a17x5","status":"ok"}`)) //nolint:gosec // G705: test server with fake secret
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTemp(t, "multi.hurl",
		"GET "+srv.URL+"/secrets\nHTTP 200\n"+
			"[Captures]\nkey: jsonpath \"$.api_key\" redact\n"+
			"\nGET "+srv.URL+"/secrets\nHTTP 200\n[Asserts]\nstatus == 200\n")

	code, _, stderr := runArgs(t, file, "--test", "--jobs", "2", "--verbose")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, stderr)
	}

	// Verify the test ran (flushed output exists)
	if !strings.Contains(stderr, "GET "+srv.URL+"/secrets") {
		t.Errorf("stderr missing request output; test may not have executed:\n%s", stderr)
	}

	// The critical check: the captured secret must never appear in clear
	if strings.Contains(stderr, "captured-secret-4a17x5") {
		t.Errorf("stderr leaks the redacted capture value:\n%s", stderr)
	}

	// Verify redaction marker is present (test exercises the redaction path)
	if !strings.Contains(stderr, "***") {
		t.Errorf("stderr has no redaction marker; the test may not exercise the redacted capture:\n%s", stderr)
	}
}
