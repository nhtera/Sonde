// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// permissiveServer answers every request 200 OK: it exists to let a run
// exercise sonde.yaml discovery, variable/secret resolution and every
// header/body/auth Sonde builds, without the test having to reproduce each
// fixture's exact expectations.
func permissiveServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestE2EImportOpenCollectionFixturesCheck imports every hand-authored
// OpenCollection fixture (single file and directory,
// docs/decisions/0002-opencollection-mapping.md), checks that every
// generated file passes `sonde check`, that sonde.yaml loads, and that
// every generated request — overriding base_url and the secrets/variables
// the fixtures use — runs successfully against a permissive server.
func TestE2EImportOpenCollectionFixturesCheck(t *testing.T) {
	srv := permissiveServer(t)
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"single-file", "../../testdata/convert/opencollection/single-file/collection.yml"},
		{"directory", "../../testdata/convert/opencollection/directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, err := filepath.Abs(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "out")
			code, _, errOut := runArgs(t, "import", "opencollection", input, "-o", dir)
			if code != ExitOK {
				t.Fatalf("import: exit %d\n%s", code, errOut)
			}
			// filepath.Glob has no recursive "**"; both fixtures nest at
			// most one folder deep (a top-level request, or one under
			// "Users/"), so a flat and a one-level pattern cover them.
			flat, err := filepath.Glob(filepath.Join(dir, "*.hurl"))
			if err != nil {
				t.Fatal(err)
			}
			nested, err := filepath.Glob(filepath.Join(dir, "*", "*.hurl"))
			if err != nil {
				t.Fatal(err)
			}
			files := append(flat, nested...)
			if len(files) == 0 {
				t.Fatalf("no .hurl files written under %s", dir)
			}
			if code, _, errOut := runArgs(t, append([]string{"check"}, files...)...); code != ExitOK {
				t.Fatalf("check: exit %d\n%s", code, errOut)
			}
			if _, err := os.Stat(filepath.Join(dir, "sonde.yaml")); err != nil {
				t.Fatalf("sonde.yaml not written: %v", err)
			}
			// The single-file fixture's multipart body references
			// "./avatar.png" (a file field, mapping doc "Request fields");
			// give it something real to read, next to the generated file.
			if err := os.WriteFile(filepath.Join(dir, "avatar.png"), []byte("not a real png, just bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"--test",
				"--variable", "base_url=" + srv.URL,
				"--variable", "name=widget",
				"--secret", "token=secret-token",
				"--secret", "admin_password=secret-pass",
				"--secret", "api_key=secret-api-key",
			}, files...)
			if code, _, errOut := runArgs(t, args...); code != ExitOK {
				t.Fatalf("run: exit %d\n%s", code, errOut)
			}
		})
	}
}

// TestE2EImportOpenCollectionRunsGreen imports a small OpenCollection
// single-file collection and runs the generated request against an
// httptest server, overriding base_url and the auth/body variables it
// needs (phase-08-importers-exporters.md "one fixture runs green").
func TestE2EImportOpenCollectionRunsGreen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/widgets" || r.URL.Query().Get("active") != "true" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	collection := writeTemp(t, "green.yml", `opencollection: "1.0.0"
request:
  auth:
    type: bearer
    token: "{{token}}"
items:
  - info:
      name: List widgets
      type: http
    http:
      method: GET
      url: "{{base_url}}/widgets"
      params:
        - name: active
          value: "true"
          type: query
    runtime:
      assertions:
        - expression: res.status
          operator: eq
          value: "200"
`)
	dir := filepath.Join(t.TempDir(), "out")
	code, _, errOut := runArgs(t, "import", "opencollection", collection, "-o", dir)
	if code != ExitOK {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
	file := filepath.Join(dir, "list-widgets.hurl")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("expected %s: %v", file, err)
	}
	code, _, errOut = runArgs(t, "--test", "--variable", "base_url="+srv.URL, "--secret", "token=secret-token", file)
	if code != ExitOK {
		t.Fatalf("run: exit %d\n%s", code, errOut)
	}
}
