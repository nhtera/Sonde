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

// TestE2EImportHTTPFixturesCheck imports both hand-authored .http fixtures
// and checks that every generated file passes "sonde check" and that the
// generated sonde.yaml loads (a run that fails only for a missing/unreachable
// server, never for a project-file or syntax error).
func TestE2EImportHTTPFixturesCheck(t *testing.T) {
	for _, name := range []string{"jetbrains", "restclient"} {
		t.Run(name, func(t *testing.T) {
			src, err := filepath.Abs(filepath.Join("../../testdata/convert/httpfile", name+".http"))
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "out")
			code, _, errOut := runArgs(t, "import", "http", src, "-o", dir)
			if code != ExitOK {
				t.Fatalf("import: exit %d\n%s", code, errOut)
			}
			files, err := filepath.Glob(filepath.Join(dir, "*.hurl"))
			if err != nil || len(files) != 1 {
				t.Fatalf("files = %v, %v", files, err)
			}
			if code, _, errOut := runArgs(t, append([]string{"check"}, files...)...); code != ExitOK {
				t.Fatalf("check: exit %d\n%s", code, errOut)
			}
			if _, err := os.Stat(filepath.Join(dir, "sonde.yaml")); err != nil {
				t.Fatalf("sonde.yaml not written: %v", err)
			}
			// The sonde.yaml default environment's variables point at
			// placeholder hosts, so a real run fails only reaching the
			// network, never on a malformed project file or entry syntax.
			code, _, errOut = runArgs(t, "--test", files[0])
			if code == ExitUsage {
				t.Fatalf("run failed before even reaching the network: exit %d\n%s", code, errOut)
			}
		})
	}
}

// TestE2EImportHTTPRunsGreen imports a small .http file whose sonde.yaml
// default environment holds a placeholder base URL, then runs the generated
// file against a real httptest server by overriding that variable, exactly
// as a user would after filling in their own sonde.yaml.
func TestE2EImportHTTPRunsGreen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/widgets" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"id": 1, "name": "gadget"}]`)
	}))
	t.Cleanup(srv.Close)

	src := writeTemp(t, "widgets.http", "@base = https://sonde-import-http-e2e.invalid\n\n"+
		"### List widgets\nGET {{base}}/widgets\nAccept: application/json\n")
	dir := filepath.Join(t.TempDir(), "out")
	code, _, errOut := runArgs(t, "import", "http", src, "-o", dir)
	if code != ExitOK {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
	file := filepath.Join(dir, "widgets.hurl")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("widgets.hurl not written: %v", err)
	}
	sondeYAML, err := os.ReadFile(filepath.Join(dir, "sonde.yaml")) //nolint:gosec // G304: test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sondeYAML), "base: https://sonde-import-http-e2e.invalid") {
		t.Errorf("sonde.yaml missing the file variable's default:\n%s", sondeYAML)
	}

	code, _, errOut = runArgs(t, "--test", "--variable", "base="+srv.URL, file)
	if code != ExitOK {
		t.Fatalf("run: exit %d\n%s", code, errOut)
	}

	// A second import without --force conflicts and writes nothing.
	if code, _, errOut := runArgs(t, "import", "http", src, "-o", dir); code != ExitUsage {
		t.Errorf("second import: exit %d\n%s", code, errOut)
	}
}
