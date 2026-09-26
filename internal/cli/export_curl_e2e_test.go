// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestE2EExportCurl exports a two-entry file and checks the printed
// commands: method, headers and body render as expected, one line per
// entry, in file order.
func TestE2EExportCurl(t *testing.T) {
	srv := curl200Server(t)
	file := writeTemp(t, "a.hurl",
		"GET "+srv.URL+"/ping\n\n"+
			"POST "+srv.URL+"/widgets\nContent-Type: application/json\n{\"a\":1}\n")

	code, out, errOut := runArgs(t, "export", "curl", file)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "'"+srv.URL+"/ping'") {
		t.Errorf("line 1 = %q, want the GET /ping URL", lines[0])
	}
	if !strings.Contains(lines[1], "--data") || !strings.Contains(lines[1], `{"a":1}`) || !strings.Contains(lines[1], srv.URL+"/widgets") {
		t.Errorf("line 2 = %q, want a POST with the JSON body", lines[1])
	}
}

// TestE2EExportCurlRedactsSecrets proves no flag reveals a secret: the
// value passed with --secret must never appear verbatim in the exported
// command, even embedded in a Basic auth header's base64 form.
func TestE2EExportCurlRedactsSecrets(t *testing.T) {
	srv := curl200Server(t)
	const secret = "export-secret-7f2c91" //nolint:gosec // G101: a fake test value, not a credential
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"/x\nAuthorization: Bearer {{tok}}\n")

	code, out, errOut := runArgs(t, "export", "curl", file, "--secret", "tok="+secret)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if strings.Contains(out, secret) {
		t.Errorf("exported command leaks the secret:\n%s", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("exported command has no redaction marker:\n%s", out)
	}
}

// TestE2EExportCurlUndefinedVariable warns on stderr, once per variable,
// and keeps the placeholder literal in the command, for a variable no
// source defines (typically one only a capture would set).
func TestE2EExportCurlUndefinedVariable(t *testing.T) {
	srv := curl200Server(t)
	file := writeTemp(t, "a.hurl",
		"GET "+srv.URL+"/login\nHTTP 200\n[Captures]\ntoken: jsonpath \"$.token\"\n\n"+
			"GET "+srv.URL+"/profile\nAuthorization: Bearer {{token}}\n")

	code, out, errOut := runArgs(t, "export", "curl", file)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "{{token}} is undefined") {
		t.Errorf("stderr missing undefined-variable warning:\n%s", errOut)
	}
	if !strings.Contains(out, "Bearer {{token}}") {
		t.Errorf("stdout missing the literal placeholder:\n%s", out)
	}
}

// TestE2EExportCurlEntryFlag exports only the given entry, 1-based, and
// reports a usage error for an out-of-range one.
func TestE2EExportCurlEntryFlag(t *testing.T) {
	srv := curl200Server(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"/one\n\nGET "+srv.URL+"/two\n")

	code, out, errOut := runArgs(t, "export", "curl", file, "--entry", "2")
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if !strings.Contains(out, "/two") || strings.Contains(out, "/one") {
		t.Errorf("--entry 2 output = %q, want only /two", out)
	}

	code, _, errOut = runArgs(t, "export", "curl", file, "--entry", "5")
	if code != ExitUsage {
		t.Fatalf("out-of-range --entry: exit %d, want %d\n%s", code, ExitUsage, errOut)
	}
}

// TestE2EExportCurlVariable substitutes an explicit --variable into the
// rendered command.
func TestE2EExportCurlVariable(t *testing.T) {
	srv := curl200Server(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"/items/{{id}}\n")

	code, out, errOut := runArgs(t, "export", "curl", file, "--variable", "id=42")
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if !strings.Contains(out, "/items/42") {
		t.Errorf("output = %q, want the substituted id", out)
	}
}

// TestE2EExportCurlMultipleFiles exports each FILE in turn, in argument
// order.
func TestE2EExportCurlMultipleFiles(t *testing.T) {
	srv := curl200Server(t)
	a := writeTemp(t, "a.hurl", "GET "+srv.URL+"/a\n")
	b := writeTemp(t, "b.hurl", "GET "+srv.URL+"/b\n")

	code, out, errOut := runArgs(t, "export", "curl", a, b)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "/a") || !strings.Contains(lines[1], "/b") {
		t.Errorf("output = %q, want /a then /b", out)
	}
}

// TestE2EExportCurlUnreadable reports a usage error for a missing file,
// matching import.
func TestE2EExportCurlUnreadable(t *testing.T) {
	code, _, errOut := runArgs(t, "export", "curl", "missing.hurl")
	if code != ExitParse {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitParse, errOut)
	}
}

// TestE2EExportCurlNeverSends proves RenderCurl never sends anything: a
// server that would fail the test if it received a request stays silent.
func TestE2EExportCurlNeverSends(t *testing.T) {
	var hit atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit.Store(true) }))
	t.Cleanup(srv.Close)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"/must-not-be-called\n")

	if code, _, errOut := runArgs(t, "export", "curl", file); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if hit.Load() {
		t.Fatal("export sent a real request")
	}
}

// TestE2EExportCurlRedactsEncodedForms is C2 (phase 8 review): a secret
// containing "_-.~'\", a newline and a tab must stay masked in every
// encoding engine/curl.go actually renders it in — percent-encoded in
// the URL query, backslash-escaped inside a header's $'...' form, in the
// body, and in --user.
func TestE2EExportCurlRedactsEncodedForms(t *testing.T) {
	srv := curl200Server(t)
	const secret = "sk_live-51.Habc~'\\\n\t" //nolint:gosec // G101: a fake test value, not a credential
	file := writeTemp(t, "a.hurl",
		"POST "+srv.URL+"/x?api_key={{s}}\nX-Pw: {{s}}\n[BasicAuth]\nuser: {{s}}\n`{{s}}`\n")

	code, out, errOut := runArgs(t, "export", "curl", file, "--secret", "s="+secret)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if strings.Contains(out, secret) {
		t.Errorf("export leaks the raw secret:\n%s", out)
	}
	// The four characters the plain '...'/base64/JSON/URL variants alone
	// don't cover (an underscore or dash survives QueryEscape/PathEscape,
	// a quote/backslash/newline/tab survives outside $'...').
	for _, marker := range []string{"live", "Habc"} {
		if strings.Contains(out, marker) {
			t.Errorf("export leaks a recognizable fragment of the secret (%q):\n%s", marker, out)
		}
	}
	if !strings.Contains(out, "***") {
		t.Errorf("export has no redaction marker:\n%s", out)
	}
}
