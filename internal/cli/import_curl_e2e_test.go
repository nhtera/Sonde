// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// curl200Server answers every request with 200 OK, recording nothing: it is
// enough to prove an imported file actually runs, not what it sent (the
// curl package's own tests cover the semantics of the conversion).
func curl200Server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestE2EImportCurlRunsGreen imports a multi-command curl script, then
// checks the generated file and runs it against a real server: both must
// succeed, proving the round trip from shell text to a runnable request
// file (Success Criteria: "curl/http fixtures run green").
func TestE2EImportCurlRunsGreen(t *testing.T) {
	srv := curl200Server(t)
	script := "curl -X POST " + srv.URL + "/widgets \\\n" +
		"  -H 'Content-Type: application/json' \\\n" +
		"  -d '{\"name\":\"gizmo\"}'\n" +
		"curl -F 'field=value' " + srv.URL + "/upload\n" +
		"curl -u alice:s3cret " + srv.URL + "/secure\n"
	input := writeTemp(t, "script.sh", script)
	dir := t.TempDir()

	code, _, errOut := runArgs(t, "import", "curl", input, "-o", dir)
	if code != ExitOK {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
	generated := filepath.Join(dir, "script.hurl")
	if _, err := os.Stat(generated); err != nil {
		t.Fatalf("generated file missing: %v", err)
	}

	if code, _, errOut := runArgs(t, "check", generated); code != ExitOK {
		t.Fatalf("check: exit %d\n%s", code, errOut)
	}
	if code, _, errOut := runArgs(t, "--test", generated); code != ExitOK {
		t.Fatalf("run: exit %d\n%s", code, errOut)
	}
}

// TestE2EImportCurlStdin imports from standard input ("-"), which names
// the generated file "curl" rather than an input file's stem.
func TestE2EImportCurlStdin(t *testing.T) {
	srv := curl200Server(t)
	dir := t.TempDir()
	var out, errOut strings.Builder
	root := newRootCmd(&out, &errOut)
	root.SetIn(strings.NewReader("curl " + srv.URL + "/ping\n"))
	root.SetArgs([]string{"import", "curl", "-", "-o", dir})
	if err := root.Execute(); err != nil {
		t.Fatalf("import from stdin: %v\nstderr=%s", err, errOut.String())
	}
	generated := filepath.Join(dir, "curl.hurl")
	if _, err := os.Stat(generated); err != nil {
		t.Fatalf("generated file missing: %v (stderr=%s)", err, errOut.String())
	}
}

// TestE2EImportCurlNoCommand reports a usage error, and writes nothing,
// when the input holds no curl command at all.
func TestE2EImportCurlNoCommand(t *testing.T) {
	input := writeTemp(t, "notacurl.sh", "echo hello\n")
	dir := t.TempDir()
	code, _, errOut := runArgs(t, "import", "curl", input, "-o", dir)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		t.Errorf("no file should have been written, found %v", entries)
	}
}

// TestE2EImportCurlUnreadable reports a usage error for a missing input
// file, matching every other importer.
func TestE2EImportCurlUnreadable(t *testing.T) {
	dir := t.TempDir()
	code, _, errOut := runArgs(t, "import", "curl", filepath.Join(dir, "missing.sh"), "-o", dir)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
}

// TestE2EImportCurlScriptsNeverRun proves that importing never executes
// anything the shell script does (Success Criteria).
func TestE2EImportCurlScriptsNeverRun(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	script := "curl https://example.com/ok\ntouch " + marker + "\n"
	input := writeTemp(t, "script.sh", script)
	out := filepath.Join(dir, "out")
	if code, _, errOut := runArgs(t, "import", "curl", input, "-o", out); code != ExitOK {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the script's side effect ran: the importer executed shell content")
	}
}
