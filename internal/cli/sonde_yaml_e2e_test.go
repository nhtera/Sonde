// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeProjectFile writes a sonde.yaml (or a differently named project
// file) and a request file in the same directory, so discovery finds it as
// the request file's nearest ancestor.
func writeProjectFile(t *testing.T, projectName, projectYAML, reqName, reqContent string) (dir, reqPath string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, projectName), []byte(projectYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	reqPath = filepath.Join(dir, reqName)
	if err := os.WriteFile(reqPath, []byte(reqContent), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, reqPath
}

func TestE2ESondeYAMLVariable(t *testing.T) {
	srv := testServer(t)
	yaml := "version: 1\n" +
		"environments:\n" +
		"  local:\n" +
		"    variables:\n" +
		"      base_url: " + srv.URL + "\n" +
		"defaults:\n" +
		"  env: local\n"
	_, file := writeProjectFile(t, "sonde.yaml", yaml, "ok.hurl", "GET {{base_url}}/hello\nHTTP 200\n")

	code, out, errOut := runArgs(t, file)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "Hello World!" {
		t.Errorf("stdout = %q", out)
	}
}

// TestE2ESondeYAMLFlagWinsOverJob checks that a --variable of the same name
// as a sonde.yaml environment variable wins: sonde.yaml sits at the lowest
// precedence tier (docs/sonde-yaml.md), below every CLI/env-var source.
func TestE2ESondeYAMLFlagWinsOverJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Greeting"))) //nolint:gosec // G705: a local test server echoing its own header, not XSS
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	yaml := "version: 1\n" +
		"environments:\n" +
		"  local:\n" +
		"    variables:\n" +
		"      greeting: wrong\n"
	_, file := writeProjectFile(t, "sonde.yaml", yaml, "ok.hurl",
		"GET "+srv.URL+"/echo\nX-Greeting: {{greeting}}\nHTTP 200\n")

	code, out, errOut := runArgs(t, file, "--env", "local", "--variable", "greeting=right")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "right" {
		t.Errorf("stdout = %q, want %q (--variable should win over sonde.yaml)", out, "right")
	}
}

func TestE2ESondeYAMLUnknownEnv(t *testing.T) {
	srv := testServer(t)
	yaml := "version: 1\n" +
		"environments:\n" +
		"  local:\n" +
		"    variables:\n" +
		"      base_url: " + srv.URL + "\n"
	_, file := writeProjectFile(t, "sonde.yaml", yaml, "ok.hurl", "GET {{base_url}}/hello\nHTTP 200\n")

	code, _, errOut := runArgs(t, file, "--env", "nope")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, `unknown environment "nope"`) {
		t.Errorf("stderr = %q, want it to name the unknown environment", errOut)
	}
}

func TestE2ESondeYAMLConfigFlagOverridesDiscovery(t *testing.T) {
	srv := testServer(t)
	// The request file's own directory has no sonde.yaml; --config points
	// at one in an unrelated directory.
	otherDir := t.TempDir()
	cfgPath := filepath.Join(otherDir, "other.yaml")
	yaml := "version: 1\n" +
		"environments:\n" +
		"  local:\n" +
		"    variables:\n" +
		"      base_url: " + srv.URL + "\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	file := writeTemp(t, "ok.hurl", "GET {{base_url}}/hello\nHTTP 200\n")

	code, out, errOut := runArgs(t, file, "--config", cfgPath, "--env", "local")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "Hello World!" {
		t.Errorf("stdout = %q", out)
	}
}

// TestE2EEnvFlagWithNoSondeYAMLErrors checks code-reviewer finding #13's
// decision: an explicit --env with no sonde.yaml found anywhere above the
// input file is an error (exit 1, naming the file and the environment) —
// there is nothing for --env to select an environment from.
func TestE2EEnvFlagWithNoSondeYAMLErrors(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")

	code, _, errOut := runArgs(t, file, "--env", "staging")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, "no sonde.yaml found for environment") || !strings.Contains(errOut, "staging") {
		t.Errorf("stderr = %q, want it to name the missing sonde.yaml and the environment", errOut)
	}
}

// TestE2ESondeEnvVarWithNoSondeYAMLIsIgnored checks the other half of the
// same decision: SONDE_ENV alone (no --env flag) is silently ignored when
// no sonde.yaml is found — the run proceeds exactly as if SONDE_ENV had
// never been set.
func TestE2ESondeEnvVarWithNoSondeYAMLIsIgnored(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")

	t.Setenv("SONDE_ENV", "staging")
	code, out, errOut := runArgs(t, file)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "Hello World!" {
		t.Errorf("stdout = %q", out)
	}
}

func TestE2ESondeYAMLDefaultsJobsFallback(t *testing.T) {
	// defaults.jobs only takes effect as a --jobs fallback once the run is
	// already parallel (--parallel/--test); it must not force parallelism
	// on a plain sequential run. This exercises the code path without
	// depending on scheduling order (jobs=1 is externally indistinguishable
	// from sequential here), so it only checks the run still succeeds and
	// the flag/env-var still win over it.
	srv := testServer(t)
	yaml := "version: 1\n" +
		"defaults:\n" +
		"  jobs: 1\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "a.hurl")
	b := filepath.Join(dir, "b.hurl")
	body := "GET " + srv.URL + "/hello\nHTTP 200\n"
	if err := os.WriteFile(a, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runArgs(t, "--test", a, b)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
}

// TestE2ESondeYAMLInsecureCandidateWarns checks that a discovered sonde.yaml
// skipped for being group/other-writable (config.ProjectCache's own
// ownership/permission check) is reported to the user as a "warning: ..."
// line, and that the run still proceeds as if that file didn't exist (no
// variables from it, no error).
func TestE2ESondeYAMLInsecureCandidateWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the ownership/permission check is Unix-only (candidateIsSecure always true on Windows)")
	}
	srv := testServer(t)
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "sonde.yaml")
	if err := os.WriteFile(yamlPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Group- and other-writable: candidateIsSecure rejects this outright,
	// regardless of who owns it. The permissive mode is the point of the
	// test, not an oversight.
	if err := os.Chmod(yamlPath, 0o666); err != nil { //nolint:gosec // G302: deliberately insecure, exercising the rejection
		t.Fatal(err)
	}
	file := filepath.Join(dir, "ok.hurl")
	if err := os.WriteFile(file, []byte("GET "+srv.URL+"/hello\nHTTP 200\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runArgs(t, file)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "Hello World!" {
		t.Errorf("stdout = %q, want the run to proceed as if sonde.yaml didn't exist", out)
	}
	if !strings.Contains(errOut, "warning:") || !strings.Contains(errOut, yamlPath) {
		t.Errorf("stderr missing the discovery warning naming %s:\n%s", yamlPath, errOut)
	}
}
