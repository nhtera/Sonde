// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2ECurlSinkRedactsEncodedForms is C2 (phase 8 review) for the run
// --curl FILE sink (engine/curl.go, unchanged by this phase): a secret
// containing "_-.~'\" in the query, a header and --user, and the full
// set including a newline and a tab in the body (the only one of these
// that can safely carry raw control bytes over the wire), must stay
// masked in every encoding it renders that value in — exactly like
// sonde export curl (see TestE2EExportCurlRedactsEncodedForms); the same
// internal/redact fix covers both sinks, since both call Runner.Redact.
func TestE2ECurlSinkRedactsEncodedForms(t *testing.T) {
	srv := curl200Server(t)
	const secret = "sk_live-51.Habc~'\\"         //nolint:gosec // G101: a fake test value, not a credential
	const bodySecret = "sk_live-51.Habc~'\\\n\t" //nolint:gosec // G101: a fake test value, not a credential
	file := writeTemp(t, "a.hurl",
		"POST "+srv.URL+"/x\nX-Pw: {{s}}\n[Query]\napi_key: {{s}}\n[BasicAuth]\nuser: {{s}}\n`{{b}}`\nHTTP 200\n")
	curlOut := filepath.Join(t.TempDir(), "curl.txt")

	code, _, errOut := runArgs(t, file, "--secret", "s="+secret, "--secret", "b="+bodySecret, "--curl", curlOut)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	data, err := os.ReadFile(curlOut) //nolint:gosec // G304: test temp file
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if strings.Contains(out, secret) || strings.Contains(out, bodySecret) {
		t.Errorf("--curl sink leaks a raw secret:\n%s", out)
	}
	for _, marker := range []string{"live", "Habc"} {
		if strings.Contains(out, marker) {
			t.Errorf("--curl sink leaks a recognizable fragment of the secret (%q):\n%s", marker, out)
		}
	}
	if !strings.Contains(out, "***") {
		t.Errorf("--curl sink has no redaction marker:\n%s", out)
	}
}
