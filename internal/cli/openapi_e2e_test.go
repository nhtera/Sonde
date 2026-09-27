// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/mock"
	"github.com/nhtera/sonde/internal/openapi"
)

// petServer serves the petstore of testdata/openapi under /v1, with
// deliberate contract breaks: pet 2 has a string id, pet 3 no name,
// /v1/pets/418 an undocumented status.
func petServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/pets":
			w.Header().Set("X-Rate-Limit", "10")
			_, _ = io.WriteString(w, `[{"id": 1, "name": "Rex", "kind": "pet"}]`)
		case "/v1/pets/1":
			_, _ = io.WriteString(w, `{"id": 1, "name": "Rex", "kind": "pet"}`)
		case "/v1/pets/2":
			_, _ = io.WriteString(w, `{"id": "2", "name": "Milo", "kind": "pet"}`)
		case "/v1/pets/3":
			_, _ = io.WriteString(w, `{"id": 3, "kind": "pet", "secret": "`+r.Header.Get("X-Token")+`"}`) //nolint:gosec // G705: test server echoing its input
		case "/v1/pets/418":
			w.WriteHeader(http.StatusTeapot)
			_, _ = io.WriteString(w, `{}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func petSpec(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/openapi/petstore-3.1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestE2EOpenAPI(t *testing.T) {
	srv := petServer(t)
	spec := petSpec(t)
	ok := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/v1/pets\nHTTP 200\n\nGET "+srv.URL+"/v1/pets/1\n")
	code, _, errOut := runArgs(t, "--test", "--openapi", spec, ok)
	if code != ExitOK {
		t.Fatalf("conforming file: exit %d\n%s", code, errOut)
	}

	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/v1/pets/2", []string{"Contract violation", "value must be an integer (type)", "at: /id", "spec: #/paths/~1pets~1{petId}/get/responses/200/content/application~1json/schema"}},
		{"/v1/pets/3", []string{"property \"name\" is missing (required)", "at: /name"}},
		{"/v1/pets/418", []string{"status 418 is not documented"}},
	} {
		file := writeTemp(t, "bad.hurl", "GET "+srv.URL+tc.path+"\nHTTP *\n")
		code, _, errOut := runArgs(t, "--test", "--openapi", spec, file)
		if code != ExitAssert {
			t.Errorf("%s: exit %d, want %d\n%s", tc.path, code, ExitAssert, errOut)
		}
		for _, w := range tc.want {
			if !strings.Contains(errOut, w) {
				t.Errorf("%s: stderr lacks %q:\n%s", tc.path, w, errOut)
			}
		}
		// --no-assert skips the contract too.
		if code, _, errOut := runArgs(t, "--test", "--no-assert", "--openapi", spec, file); code != ExitOK {
			t.Errorf("%s --no-assert: exit %d\n%s", tc.path, code, errOut)
		}
	}
}

func TestE2EOpenAPIUnmatched(t *testing.T) {
	srv := petServer(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"/v1/unknown\n")
	code, _, errOut := runArgs(t, "--test", "--openapi", petSpec(t), file)
	if code != ExitOK || !strings.Contains(errOut, "warning: no operation of the OpenAPI spec matches GET /v1/unknown") {
		t.Errorf("default: exit %d\n%s", code, errOut)
	}
	code, _, errOut = runArgs(t, "--test", "--openapi", petSpec(t), "--openapi-strict", file)
	if code != ExitAssert || !strings.Contains(errOut, "no operation of the OpenAPI spec matches") {
		t.Errorf("strict: exit %d\n%s", code, errOut)
	}
	// --openapi-server replaces the spec's /v1 base path.
	root := writeTemp(t, "b.hurl", "GET "+srv.URL+"/v1/pets/2\n")
	code, _, errOut = runArgs(t, "--test", "--openapi", petSpec(t), "--openapi-server", srv.URL+"/v1/pets", "--openapi-strict", root)
	if code != ExitAssert || !strings.Contains(errOut, "no operation of the OpenAPI spec matches GET /v1/pets/2") {
		t.Errorf("server: exit %d\n%s", code, errOut)
	}
}

func TestE2EOpenAPIErrors(t *testing.T) {
	file := writeTemp(t, "a.hurl", "GET http://localhost:1\n")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--openapi", "missing.yaml"}, "--openapi: "},
		{[]string{"--openapi", "https://example.invalid/openapi.yaml"}, "use --openapi-allow-remote"},
		{[]string{"--openapi", writeTemp(t, "x.yaml", "a: 1\n")}, "not an OpenAPI document"},
	} {
		code, _, errOut := runArgs(t, append(tc.args, file)...)
		if code != ExitUsage || !strings.Contains(errOut, tc.want) {
			t.Errorf("%v: exit %d, stderr %q", tc.args, code, errOut)
		}
	}
}

func TestE2EOpenAPISondeYAML(t *testing.T) {
	srv := petServer(t)
	spec, err := os.ReadFile(petSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	dir, bad := writeProjectFile(t, "sonde.yaml",
		"version: 1\nopenapi:\n  spec: api/openapi.yaml\n  exclude_operations: [\"GET /pets\"]\n  exclude_files: [\"legacy/**\"]\n",
		"bad.hurl", "GET "+srv.URL+"/v1/pets/2\n")
	for name, content := range map[string]string{
		"api/openapi.yaml":    string(spec),
		"legacy/old/bad.hurl": "GET " + srv.URL + "/v1/pets/2\n",
		"excluded-op.hurl":    "GET " + srv.URL + "/v1/unknown-but-excluded\nGET " + srv.URL + "/v1/pets\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, errOut := runArgs(t, "--test", bad); code != ExitAssert || !strings.Contains(errOut, "at: /id") {
		t.Errorf("sonde.yaml contract: exit %d\n%s", code, errOut)
	}
	legacy := filepath.Join(dir, "legacy/old/bad.hurl")
	if code, _, errOut := runArgs(t, "--test", legacy); code != ExitOK {
		t.Errorf("excluded file: exit %d\n%s", code, errOut)
	}
	if code, _, errOut := runArgs(t, "--test", filepath.Join(dir, "excluded-op.hurl")); code != ExitOK {
		t.Errorf("excluded operation: exit %d\n%s", code, errOut)
	}
	// Flags win over sonde.yaml: strict turns the unmatched warning into a failure.
	if code, _, errOut := runArgs(t, "--test", "--openapi-strict", filepath.Join(dir, "excluded-op.hurl")); code != ExitAssert {
		t.Errorf("--openapi-strict over sonde.yaml: exit %d\n%s", code, errOut)
	}
}

func TestE2EOpenAPISecretsAndReport(t *testing.T) {
	srv := petServer(t)
	dir := t.TempDir()
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"/v1/pets/3\nX-Token: {{token}}\n")
	data := writeTemp(t, "rows.csv", "token\nrow-secret-value\n")
	code, _, errOut := runArgs(t, "--test", "--openapi", petSpec(t), file, "--data", data, "--data-secret", "token",
		"--report-json", filepath.Join(dir, "json"), "--error-format", "long")
	if code != ExitAssert {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	report, err := os.ReadFile(filepath.Join(dir, "json", "report.json")) //nolint:gosec // G304: test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), `"sonde":{"contract":{"violations":[{"instance_path":"/name"`) {
		t.Errorf("report lacks the violation:\n%s", report)
	}
	if strings.Contains(string(report)+errOut, "row-secret-value") {
		t.Error("a row secret leaks")
	}
}

// specServer serves a mock of spec (sonde mock's handler).
func specServer(t *testing.T, spec *openapi.Spec) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mock.Handler(spec.Mock(""), mock.Options{ValidateRequests: true}))
	t.Cleanup(srv.Close)
	return srv
}

// TestE2EImportOpenAPIRunsGreen imports a spec, checks the files and runs
// them against a mock of the spec (sonde mock), validated by the same
// spec.
func TestE2EImportOpenAPIRunsGreen(t *testing.T) {
	specPath := petSpec(t)
	spec, err := openapi.Load(context.Background(), specPath, openapi.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	srv := specServer(t, spec)
	dir := filepath.Join(t.TempDir(), "out")
	code, _, errOut := runArgs(t, "import", "openapi", specPath, "-o", dir)
	if code != ExitOK || !strings.Contains(errOut, "wrote 5 file(s)") {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.hurl"))
	if err != nil || len(files) != 5 {
		t.Fatalf("files = %v, %v", files, err)
	}
	if code, _, errOut := runArgs(t, append([]string{"check"}, files...)...); code != ExitOK {
		t.Fatalf("check: exit %d\n%s", code, errOut)
	}
	args := append([]string{"--test", "--openapi", specPath, "--openapi-strict", "--variable", "base_url=" + srv.URL + "/v1",
		"--variable", "username=u", "--secret", "password=p4ssw0rd"}, files...)
	if code, _, errOut := runArgs(t, args...); code != ExitOK {
		t.Fatalf("run: exit %d\n%s", code, errOut)
	}
	// A second import without --force conflicts and writes nothing.
	if code, _, errOut := runArgs(t, "import", "openapi", specPath, "-o", dir); code != ExitUsage {
		t.Errorf("second import: exit %d\n%s", code, errOut)
	}
}

func TestE2EOpenAPIEdgeCases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle HEAD requests
		if r.Method == "HEAD" {
			w.Header().Set("Content-Type", "application/json")
			return
		}
		// Handle 204 No Content
		if r.URL.Path == "/v1/pets/1" && r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Handle trailing slash - return same response as /v1/pets but with header
		if r.URL.Path == "/v1/pets/" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Rate-Limit", "10")
			_, _ = io.WriteString(w, `[{"id": 1, "name": "Rex", "kind": "pet"}]`)
			return
		}
		// Handle default for /v1/pets
		if r.URL.Path == "/v1/pets" || r.URL.Path == "/v1/pets/" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Rate-Limit", "10")
			_, _ = io.WriteString(w, `[{"id": 1, "name": "Rex", "kind": "pet"}]`)
			return
		}
		// Handle individual pets
		if r.URL.Path == "/v1/pets/1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id": 1, "name": "Rex", "kind": "pet"}`)
			return
		}
		// Default response
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id": 1, "name": "Rex", "kind": "pet"}`)
	}))
	t.Cleanup(srv.Close)
	spec := petSpec(t)

	// Test 204 No Content (DELETE should work without a body)
	file := writeTemp(t, "delete.hurl", "DELETE "+srv.URL+"/v1/pets/1\nHTTP 204\n")
	if code, _, errOut := runArgs(t, "--test", "--openapi", spec, file); code != ExitOK {
		t.Errorf("DELETE 204: exit %d\n%s", code, errOut)
	}

	// Test trailing slash (should match /pets by stripping slash)
	file = writeTemp(t, "slash.hurl", "GET "+srv.URL+"/v1/pets/\nHTTP 200\n")
	if code, _, errOut := runArgs(t, "--test", "--openapi", spec, file); code != ExitOK {
		t.Errorf("trailing slash: exit %d\n%s", code, errOut)
	}
}

func TestE2EOpenAPIRetryWithContractViolation(t *testing.T) {
	srv := petServer(t)
	file := writeTemp(t, "bad.hurl", "GET "+srv.URL+"/v1/pets/2\nHTTP *\n")
	// With --retry, a contract violation should still fail after retries
	code, _, errOut := runArgs(t, "--test", "--openapi", petSpec(t), "--retry", "1", file)
	if code != ExitAssert {
		t.Errorf("exit %d, want %d\n%s", code, ExitAssert, errOut)
	}
	if !strings.Contains(errOut, "Contract violation") {
		t.Errorf("stderr lacks 'Contract violation':\n%s", errOut)
	}
}

func TestE2EOpenAPIImportWithFlags(t *testing.T) {
	spec := petSpec(t)
	dir := t.TempDir()

	// Test --dry-run
	code, _, errOut := runArgs(t, "import", "openapi", spec, "-o", dir, "--dry-run")
	if code != ExitOK || !strings.Contains(errOut, "would write") {
		t.Errorf("dry-run: exit %d\n%s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "list-pets.hurl")); err == nil {
		t.Error("dry-run should not write files")
	}

	// Test --group path
	dir2 := t.TempDir()
	code, _, errOut = runArgs(t, "import", "openapi", spec, "-o", dir2, "--group", "path")
	if code != ExitOK {
		t.Errorf("group path: exit %d\n%s", code, errOut)
	}
	// Should have pets/ and pets/delete-pet.hurl or similar
	if _, err := os.Stat(filepath.Join(dir2, "pets")); err != nil {
		t.Errorf("group path should create pets dir: %v", err)
	}

	// Test --group flat
	dir3 := t.TempDir()
	code, _, errOut = runArgs(t, "import", "openapi", spec, "-o", dir3, "--group", "flat")
	if code != ExitOK {
		t.Errorf("group flat: exit %d\n%s", code, errOut)
	}
	files, err := filepath.Glob(filepath.Join(dir3, "*.hurl"))
	if err != nil || len(files) == 0 {
		t.Errorf("group flat should create .hurl files at top level: %v", err)
	}
}

func TestE2EOpenAPIImportSwagger2(t *testing.T) {
	spec, err := filepath.Abs("../../testdata/openapi/swagger-2.0.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	code, _, errOut := runArgs(t, "import", "openapi", spec, "-o", dir)
	if code != ExitOK {
		t.Fatalf("swagger 2.0 import: exit %d\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "wrote") {
		t.Errorf("swagger 2.0 import should succeed: %s", errOut)
	}
}
