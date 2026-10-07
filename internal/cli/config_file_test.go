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

// useConfigFile writes content as the user config file of the test and
// returns a request file for a JSON server.
func useConfigFile(t *testing.T, content string) string {
	t.Helper()
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "hurl"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "hurl", "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"a":1}`)
	}))
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "get.hurl")
	if err := os.WriteFile(file, []byte("GET "+srv.URL+"\nHTTP 200\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// TestConfigFileErrorFormatFirst checks that a bad --error-format is
// still reported before another setting's error.
func TestConfigFileErrorFormatFirst(t *testing.T) {
	file := useConfigFile(t, "")
	code, _, errOut := runArgs(t, "--error-format", "bogus", "--delay", "x", file)
	if code != ExitUsage || !strings.Contains(errOut, "error-format") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

// TestConfigFileOutputSettings checks the config file's output options
// and the environment variables that replace them, as upstream resolves
// them: HURL_NO_OUTPUT=false keeps --no-output, HURL_PRETTY=false turns
// --pretty off.
func TestConfigFileOutputSettings(t *testing.T) {
	tests := []struct {
		config, envName, envValue, want string
	}{
		{"--no-output\n", "", "", ""},
		{"--no-output\n", "HURL_NO_OUTPUT", "false", ""},
		{"--pretty\n", "", "", "{\n  \"a\": 1\n}\n"},
		{"--pretty\n", "HURL_PRETTY", "false", `{"a":1}`},
		{"--no-pretty\n", "HURL_NO_PRETTY", "false", "{\n  \"a\": 1\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.config+tt.envName, func(t *testing.T) {
			file := useConfigFile(t, tt.config)
			if tt.envName != "" {
				t.Setenv(tt.envName, tt.envValue)
			}
			code, out, errOut := runArgs(t, file)
			if code != ExitOK || out != tt.want {
				t.Errorf("exit %d, stdout %q, want %q (stderr %q)", code, out, tt.want, errOut)
			}
		})
	}
}
