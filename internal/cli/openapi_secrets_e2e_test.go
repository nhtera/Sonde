// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2EOpenAPIRowSecretsInViolations runs rows whose secret lands in a
// violation message (an unexpected property named after it) and in an
// unmatched-request warning (a path holding it), and checks no output
// holds it.
func TestE2EOpenAPIRowSecretsInViolations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"`+r.Header.Get("X-Token")+`": 1}`) //nolint:gosec // G705: test server echoing its input
	}))
	t.Cleanup(srv.Close)
	spec := writeTemp(t, "spec.yaml", `openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /item:
    get:
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {type: object, additionalProperties: false}
`)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"/item\nX-Token: {{token}}\n\nGET "+srv.URL+"/unknown/{{token}}\nX-Token: {{token}}\n")
	data := writeTemp(t, "rows.csv", "token\nrow-secret-alpha\nrow-secret-bravo\n")
	dir := t.TempDir()
	for _, args := range [][]string{
		{"--test", "--continue-on-error", "--jobs", "2", "--error-format", "long", "--report-json", filepath.Join(dir, "json"),
			"--report-junit", filepath.Join(dir, "junit.xml"), "--report-html", filepath.Join(dir, "html"), "--report-tap", filepath.Join(dir, "r.tap")},
		{"--json", "--continue-on-error"},
		{"--very-verbose", "--continue-on-error"},
	} {
		args = append(args, "--openapi", spec, file, "--data", data, "--data-secret", "token")
		code, out, errOut := runArgs(t, args...)
		if code != ExitAssert {
			t.Fatalf("%v: exit %d\n%s", args[:2], code, errOut)
		}
		all := out + errOut
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(path) //nolint:gosec // G304: test temp dir
			all += string(b)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(all, "is unsupported") || !strings.Contains(all, "no operation of the OpenAPI spec matches") {
			t.Errorf("%v: the violation or the warning is missing:\n%s", args[:2], errOut)
		}
		for _, secret := range []string{"row-secret-alpha", "row-secret-bravo"} {
			if strings.Contains(all, secret) {
				t.Errorf("%v: output leaks %s", args[:2], secret)
			}
		}
	}
}
