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
