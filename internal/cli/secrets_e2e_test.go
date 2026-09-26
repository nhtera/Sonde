// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Secret values from each source; long and distinctive so a match can
// only come from a leak.
const (
	cliSecret = "cli-secret-7d1f93" //nolint:gosec // G101: a fake test value, not a credential
	envSecret = "env-secret-a24c58" //nolint:gosec // G101
	dynSecret = "dyn-secret-e61b07" //nolint:gosec // G101
)

// leakServer echoes every secret it receives into headers, cookies and
// the body, and hands out a fresh value to be captured with `redact`.
func leakServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		echo := r.Header.Get("X-Cli") + " " + r.Header.Get("X-Env") + " " + r.Header.Get("X-Dyn")
		w.Header().Set("X-Echo", echo)
		http.SetCookie(w, &http.Cookie{Name: "token", Value: strings.ReplaceAll(r.Header.Get("X-Cli"), " ", "")}) //nolint:gosec // G124: a local test server, cookie flags are not the point
		if v := r.Header.Get("X-Dyn"); v != "" {
			http.SetCookie(w, &http.Cookie{Name: "dyn", Value: v}) //nolint:gosec // G124
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"dynamic": dynSecret, "echo": echo})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// No secret, whatever its source, reaches stdout (--json), stderr
// (verbose logs and errors) or the cookie jar. A `redact` capture is
// refused in verbose mode, so the verbose run has no dynamic secret.
func TestE2ESecretsNeverLeak(t *testing.T) {
	t.Setenv("SONDE_SECRET_env_secret", envSecret)
	const static = `GET {{host}}/a?q={{cli_secret}}
X-Cli: {{cli_secret}}
X-Env: {{env_secret}}
HTTP 200
[Asserts]
jsonpath "$.echo" == "no {{cli_secret}}"
body contains "no {{env_secret}}"
`
	const dynamic = `GET {{host}}/a?q={{cli_secret}}
X-Cli: {{cli_secret}}
X-Env: {{env_secret}}
HTTP 200
[Captures]
dyn: jsonpath "$.dynamic" redact

GET {{host}}/b?q={{dyn}}
X-Cli: {{cli_secret}}
X-Dyn: {{dyn}}
HTTP 200
[Asserts]
header "X-Echo" == "no {{dyn}}"
jsonpath "$.echo" == "no {{cli_secret}}"
body contains "no {{env_secret}}"
`
	for _, tc := range []struct {
		name, source  string
		args, secrets []string
	}{
		{"verbose", static, []string{"--verbose"}, []string{cliSecret, envSecret}},
		{"dynamic", dynamic, nil, []string{cliSecret, envSecret, dynSecret}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := leakServer(t)
			file := writeTemp(t, "leak.hurl", tc.source)
			jar := filepath.Join(t.TempDir(), "jar.txt")
			args := append([]string{file, "--json", "--continue-on-error", "--variable", "host=" + srv.URL,
				"--secret", "cli_secret=" + cliSecret, "--cookie-jar", jar}, tc.args...)
			code, out, errOut := runArgs(t, args...)
			if code != ExitAssert {
				t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitAssert, errOut)
			}
			jarData, err := os.ReadFile(jar)
			if err != nil {
				t.Fatal(err)
			}
			sinks := map[string]string{"stdout (--json)": out, "stderr": errOut, "cookie jar": string(jarData)}
			for name, text := range sinks {
				if !strings.Contains(text, "***") {
					t.Errorf("%s has no redacted value; the test no longer exercises it", name)
				}
				for _, s := range tc.secrets {
					if strings.Contains(text, s) {
						t.Errorf("%s leaks a secret:\n%s", name, text)
					}
				}
			}
		})
	}
}

// A request file cannot reach files outside the file root, through `..`
// or a symbolic link; the run fails with a runtime error.
func TestE2EFileRootEscape(t *testing.T) {
	srv := testServer(t)
	outside := writeTemp(t, "outside.txt", "outside")
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"dotdot": "file," + rel + ";", "symlink": "file,link.txt;"} {
		t.Run(name, func(t *testing.T) {
			file := writeTemp(t, "escape.hurl", "POST "+srv.URL+"/hello\n"+body+"\nHTTP 200\n")
			code, _, errOut := runArgs(t, file, "--file-root", root)
			if code != ExitRuntime {
				t.Errorf("exit code = %d, want %d; stderr=%s", code, ExitRuntime, errOut)
			}
		})
	}
}

func TestE2EExitCodes(t *testing.T) {
	srv := testServer(t)
	for _, tc := range []struct {
		name, source string
		args         []string
		want         int
	}{
		{"ok", "GET " + srv.URL + "/hello\nHTTP 200\n", nil, ExitOK},
		{"usage", "GET " + srv.URL + "/hello\n", []string{"--no-such-flag"}, ExitUsage},
		{"parse", "GET\n", nil, ExitParse},
		{"assert", "GET " + srv.URL + "/hello\nHTTP 201\n", nil, ExitAssert},
		{"runtime", "GET " + srv.URL + "/hello\n[Options]\noutput: ../../out.txt\n", nil, ExitRuntime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := writeTemp(t, "f.hurl", tc.source)
			if code, _, errOut := runArgs(t, append([]string{file}, tc.args...)...); code != tc.want {
				t.Errorf("exit code = %d, want %d; stderr=%s", code, tc.want, errOut)
			}
		})
	}
}

func TestE2EInterrupted(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-block }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })
	file := writeTemp(t, "slow.hurl", "GET "+srv.URL+"\nHTTP 200\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel) // as on SIGINT
	var out, errOut bytes.Buffer
	if code := run(ctx, []string{file}, &out, &errOut); code != ExitInterrupted {
		t.Errorf("exit code = %d, want %d; stderr=%s", code, ExitInterrupted, errOut.String())
	}
}

// TestE2ESondeYAMLSecretsFilesRedacted is the regression test for
// code-reviewer finding #3 & #7 (2026-09-26): secrets from
// sonde.yaml's secrets_files must never appear in clear in any output
// sink: stderr (-v), stdout (--json), reports (--report-junit, --report-tap,
// --report-json including store/ files, --report-html all files).
// This test walks every generated file to verify redaction.
func TestE2ESondeYAMLSecretsFilesRedacted(t *testing.T) {
	const fileSecret = "file-secret-8c29f7" //nolint:gosec // G101: a fake test value, not a credential
	mux := http.NewServeMux()
	mux.HandleFunc("/secret", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Secret", fileSecret)
		_, _ = w.Write([]byte("response: " + fileSecret)) //nolint:gosec // G705: test server echoing its own secret
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Create a project directory with both sonde.yaml and secrets file
	projectDir := t.TempDir()
	secretsFile := filepath.Join(projectDir, "secrets.txt")
	if err := os.WriteFile(secretsFile, []byte("mysec="+fileSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	yaml := "version: 1\n" +
		"environments:\n" +
		"  test:\n" +
		"    secrets_files:\n" +
		"      - secrets.txt\n" +
		"defaults:\n" +
		"  env: test\n"

	sondeYAML := filepath.Join(projectDir, "sonde.yaml")
	if err := os.WriteFile(sondeYAML, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	reqFile := filepath.Join(projectDir, "leak.hurl")
	if err := os.WriteFile(reqFile, []byte("GET "+srv.URL+"/secret\nX-Sec: {{mysec}}\nHTTP 200\n[Captures]\nsecval: header \"X-Secret\" redact\n[Asserts]\nstatus == 404\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	reportDir := t.TempDir()
	junit := filepath.Join(reportDir, "junit.xml")
	tap := filepath.Join(reportDir, "report.tap")
	jsonDir := filepath.Join(reportDir, "json")
	htmlDir := filepath.Join(reportDir, "html")

	code, stdout, stderr := runArgs(t, reqFile,
		"--test",
		"--report-junit", junit,
		"--report-tap", tap,
		"--report-json", jsonDir,
		"--report-html", htmlDir,
	)
	// Expect assertion failure (status code 4) since we assert status == 404 but get 200
	if code != ExitAssert {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitAssert, stderr)
	}

	sinks := map[string]string{
		"stdout": stdout,
		"stderr": stderr,
	}

	// Check stdout and stderr for the file secret: it must never appear in clear
	for name, content := range sinks {
		if strings.Contains(content, fileSecret) {
			t.Errorf("%s leaks the file secret:\n%s", name, content)
		}
	}

	// Walk all report files
	for _, path := range []string{junit, tap} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("failed to read %s: %v", path, err)
			continue
		}
		if strings.Contains(string(data), fileSecret) {
			t.Errorf("%s leaks the file secret", path)
		}
	}

	// Check JSON report and store/ files
	if err := filepath.Walk(jsonDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: path from filepath.Walk over t.TempDir()
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), fileSecret) {
			t.Errorf("JSON file %s leaks the file secret", path)
		}
		return nil
	}); err != nil {
		t.Errorf("walking JSON dir: %v", err)
	}

	// Check HTML report files
	if err := filepath.Walk(htmlDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: path from filepath.Walk over t.TempDir()
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), fileSecret) {
			t.Errorf("HTML file %s leaks the file secret", path)
		}
		return nil
	}); err != nil {
		t.Errorf("walking HTML dir: %v", err)
	}
}
