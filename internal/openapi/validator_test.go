// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

func load(t *testing.T, name string, opt LoadOptions) *Spec {
	t.Helper()
	s, err := Load(context.Background(), "../../testdata/openapi/"+name, opt)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// check validates a response and returns its violations, one line each:
// "instance|message|pointer", "warning: message" for warnings.
func check(v *Validator, method, rawURL string, status int, body string, headers ...string) []string {
	var hs exchange.Headers
	if body != "" {
		hs = append(hs, exchange.Header{Name: "Content-Type", Value: "application/json; charset=utf-8"})
	}
	for i := 0; i+1 < len(headers); i += 2 {
		hs = append(hs, exchange.Header{Name: headers[i], Value: headers[i+1]})
	}
	var out []string
	for _, vi := range v.ValidateResponse(context.Background(), &exchange.Request{Method: method, URL: rawURL},
		&exchange.Response{Status: status, Headers: hs, Body: []byte(body)}) {
		if vi.Warning {
			out = append(out, "warning: "+vi.Message)
			continue
		}
		out = append(out, vi.InstancePath+"|"+vi.Message+"|"+vi.SpecPointer)
	}
	return out
}

func expect(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("violations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

const petSchema = "#/paths/~1pets~1{petId}/get/responses/200/content/application~1json/schema"

func TestValidatePetstore(t *testing.T) {
	for _, name := range []string{"petstore-3.0.yaml", "petstore-3.1.yaml"} {
		t.Run(name, func(t *testing.T) {
			v := load(t, name, LoadOptions{}).Validator(Options{})
			pets := `[{"id": 1, "name": "Rex", "kind": "pet", "tag": null, "status": "available"}]`
			expect(t, check(v, "GET", "https://api.example.com/v1/pets?limit=1", 200, pets, "X-Rate-Limit", "10"))
			expect(t, check(v, "GET", "http://localhost/v1/pets/1", 200, `{"id": "1", "name": "Rex", "kind": "pet"}`),
				"/id|value must be an integer (type)|"+petSchema)
			expect(t, check(v, "GET", "http://localhost/v1/pets/1", 200, `{"id": 1, "kind": "pet"}`),
				"/name|property \"name\" is missing (required)|"+petSchema)
			expect(t, check(v, "GET", "http://localhost/v1/pets", 200, `[]`),
				`header X-Rate-Limit|response header "X-Rate-Limit" missing|#/paths/~1pets/get/responses/200/headers/X-Rate-Limit`)
			expect(t, check(v, "GET", "http://localhost/v1/pets/1", 418, `{}`),
				"|status 418 is not documented|#/paths/~1pets~1{petId}/get/responses")
			// The default response documents any other status.
			expect(t, check(v, "GET", "http://localhost/v1/pets", 500, `{"code": 500, "message": "boom"}`))
			expect(t, check(v, "DELETE", "http://localhost/v1/pets/1", 204, ""))
			expect(t, check(v, "GET", "http://localhost/v1/pets/mine", 200, `[]`))
			expect(t, check(v, "GET", "http://localhost/v1/unknown", 200, `{}`),
				"warning: no operation of the OpenAPI spec matches GET /v1/unknown")
			expect(t, check(v, "PUT", "http://localhost/v1/pets", 200, `{}`),
				"warning: the OpenAPI spec has no PUT /pets operation")
		})
	}
}

func TestValidate31(t *testing.T) {
	v := load(t, "petstore-3.1.yaml", LoadOptions{}).Validator(Options{})
	expect(t, check(v, "GET", "http://localhost/v1/pets/1", 200, `{"id": 1, "name": "Rex", "kind": "cat", "tag": 3}`),
		"/kind|value must be \"pet\" (const)|"+petSchema,
		"/tag|value must be one of string, null (type)|"+petSchema)
}

func TestValidateContentType(t *testing.T) {
	v := load(t, "petstore-3.0.yaml", LoadOptions{}).Validator(Options{})
	got := v.ValidateResponse(context.Background(), &exchange.Request{Method: "GET", URL: "http://x/v1/pets/1"},
		&exchange.Response{Status: 200, Headers: exchange.Headers{{Name: "Content-Type", Value: "text/plain"}}, Body: []byte("hi")})
	if len(got) != 1 || !strings.Contains(got[0].Message, "text/plain") {
		t.Errorf("violations = %+v", got)
	}
}

func TestValidatorOptions(t *testing.T) {
	s := load(t, "petstore-3.0.yaml", LoadOptions{})
	strict := s.Validator(Options{Strict: true, Server: "http://localhost:3000/", ExcludeOperations: []string{"GET /pets/{petId}"}})
	expect(t, check(strict, "GET", "http://localhost:3000/unknown", 200, `{}`),
		"|no operation of the OpenAPI spec matches GET /unknown|")
	// The server replaces the spec's: /v1 is now part of the path.
	expect(t, check(strict, "GET", "http://localhost:3000/v1/pets", 200, `[]`),
		"|no operation of the OpenAPI spec matches GET /v1/pets|")
	expect(t, check(strict, "GET", "http://localhost:3000/pets/1", 200, `{"id": "x"}`))
	if ops := s.Operations(); len(ops) != 5 || ops[0] != "DELETE /pets/{petId}" {
		t.Errorf("Operations() = %v", ops)
	}
}

func TestValidateConcurrent(t *testing.T) {
	v := load(t, "petstore-3.1.yaml", LoadOptions{}).Validator(Options{})
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 50 {
				got := check(v, "GET", fmt.Sprintf("http://x/v1/pets/%d", i*j), 200, `{"id": "x", "name": "Rex", "kind": "pet"}`)
				if len(got) != 1 {
					t.Errorf("violations = %v", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestLoadRefs(t *testing.T) {
	v := load(t, "refs/api.yaml", LoadOptions{}).Validator(Options{})
	expect(t, check(v, "GET", "http://x/items/a", 200, `{"id": 1}`),
		"/id|value must be a string (type)|#/paths/~1items~1{id}/get/responses/200/content/application~1json/schema")
	for name, want := range map[string]string{
		"refs/escape.yaml":        "must stay inside the directory of the spec",
		"refs/remote.yaml":        "use --openapi-allow-remote",
		"refs/schemas/item.yaml":  "not an OpenAPI document",
		"missing.yaml":            "",
		"../../../../etc/passwd0": "",
	} {
		_, err := Load(context.Background(), "../../testdata/openapi/"+name, LoadOptions{})
		if want == "" {
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s: error %v, want a missing file", name, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %v, want %q", name, err, want)
		}
	}
}

func TestLoadRemote(t *testing.T) {
	spec := "openapi: 3.0.3\ninfo: {title: t, version: '1'}\npaths:\n  /a:\n    get:\n      responses:\n        '200':\n          description: ok\n          content:\n            application/json:\n              schema: {$ref: 'schema.yaml'}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/schema.yaml" {
			_, _ = w.Write([]byte("type: integer\n"))
			return
		}
		_, _ = w.Write([]byte(spec))
	}))
	defer srv.Close()
	if _, err := Load(context.Background(), srv.URL+"/api.yaml", LoadOptions{}); err == nil || !strings.Contains(err.Error(), "--openapi-allow-remote") {
		t.Errorf("remote spec without opt-in: %v", err)
	}
	s, err := Load(context.Background(), srv.URL+"/api.yaml", LoadOptions{AllowRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, check(s.Validator(Options{}), "GET", "http://x/a", 200, `"x"`),
		"|value must be an integer (type)|#/paths/~1a/get/responses/200/content/application~1json/schema")
}

func TestLoadSwagger2(t *testing.T) {
	v := load(t, "swagger-2.0.yaml", LoadOptions{}).Validator(Options{})
	expect(t, check(v, "GET", "http://x/api/pets/1", 200, `{"id": 1}`))
	if got := check(v, "GET", "http://x/api/pets/1", 200, `{}`); len(got) != 1 {
		t.Errorf("violations = %v", got)
	}
}

var _ engine.ResponseValidator = (*Validator)(nil)

func TestValidateEdgeCases(t *testing.T) {
	v := load(t, "edge-cases-3.1.yaml", LoadOptions{}).Validator(Options{})
	repo := `{"id": "00000000-0000-4000-8000-000000000000", "name": "sonde", "owner": "a@example.com", "visibility": "public", "createdAt": "2026-01-01T00:00:00Z"`
	expect(t, check(v, "GET", "https://api.example.com/v2/orgs/acme/repos/sonde", 200, repo+`}`))
	// Two path parameters, a server variable default (v2) and allOf.
	got := check(v, "GET", "https://api.example.com/v2/orgs/acme/repos/sonde", 200, `{"name": "sonde"}`)
	if len(got) == 0 {
		t.Error("a repository without its required properties conforms")
	}
}

func TestLoadRefEdgeCases(t *testing.T) {
	// Test absolute path $ref escaping the spec directory is blocked
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "spec")
	if err := os.MkdirAll(specDir, 0o750); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(tmpDir, "outside.yaml")
	if err := os.WriteFile(outsidePath, []byte("type: integer\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Use an absolute path to a file outside the spec directory
	specWithAbsRef := `openapi: 3.0.0
info: {title: t, version: "1"}
paths:
  /a:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '` + outsidePath + `'}
`
	specFile := filepath.Join(specDir, "spec.yaml")
	if err := os.WriteFile(specFile, []byte(specWithAbsRef), 0o600); err != nil {
		t.Fatal(err)
	}
	// On Windows, the drive letter reads as a URL scheme: the reference is
	// rejected as unsupported.
	_, err := Load(context.Background(), specFile, LoadOptions{})
	driveAsScheme := runtime.GOOS == "windows" && err != nil && strings.Contains(err.Error(), "unsupported reference")
	if err == nil || !strings.Contains(err.Error(), "must stay inside") && !driveAsScheme {
		t.Errorf("absolute path $ref outside spec dir should be blocked: %v", err)
	}

	// Test file:// $ref is blocked (points to /etc/passwd)
	fileRefSpec := `openapi: 3.0.0
info: {title: t, version: "1"}
paths:
  /a:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: 'file:///etc/passwd'}
`
	fileRefFile := filepath.Join(specDir, "file-ref.yaml")
	if err := os.WriteFile(fileRefFile, []byte(fileRefSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	// On Windows, a path without a drive is resolved inside the directory
	// of the spec, where etc\passwd does not exist.
	if _, err := Load(context.Background(), fileRefFile, LoadOptions{}); err == nil ||
		!strings.Contains(err.Error(), "must stay inside") && runtime.GOOS != "windows" {
		t.Errorf("file:// $ref to /etc/passwd should be blocked: %v", err)
	}
}

func TestValidateResponseContentType(t *testing.T) {
	v := load(t, "petstore-3.0.yaml", LoadOptions{}).Validator(Options{})

	// Test with charset in content type
	expect(t, check(v, "GET", "http://x/v1/pets/1", 200, `{"id": 1, "name": "Rex", "kind": "pet"}`, "Content-Type", "application/json; charset=utf-8"))

	// Test with different charset should still pass
	expect(t, check(v, "GET", "http://x/v1/pets/1", 200, `{"id": 1, "name": "Rex", "kind": "pet"}`, "Content-Type", "application/json; charset=iso-8859-1"))

	// Test wrong content type header for JSON response
	got := v.ValidateResponse(context.Background(), &exchange.Request{Method: "GET", URL: "http://x/v1/pets/1"},
		&exchange.Response{Status: 200, Headers: exchange.Headers{{Name: "Content-Type", Value: "text/plain"}}, Body: []byte(`{"id": 1, "name": "Rex", "kind": "pet"}`)})
	if len(got) == 0 || !strings.Contains(got[0].Message, "text/plain") {
		t.Errorf("wrong content type should be flagged: %v", got)
	}
}

func TestValidateReviewFixes(t *testing.T) {
	dir := t.TempDir()
	spec := `openapi: 3.1.0
info: {title: t, version: "1"}
paths:
  /pets/mine:
    get:
      responses:
        "200": {description: ok}
  /pets/{id}:
    delete:
      responses:
        "204": {description: deleted}
  /things:
    get:
      responses:
        "200":
          description: ok
          headers:
            X-N: {required: true, schema: {type: integer, minimum: 5}}
          content:
            application/xml: {schema: {type: string, maxLength: 1}}
            application/vnd.acme+json: {schema: {type: object, required: [id]}}
            text/plain: {schema: {type: string, maxLength: 1}}
`
	path := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	v := s.Validator(Options{Strict: true})
	do := func(method, ct, body string, headers ...string) []engine.Violation {
		hs := exchange.Headers{{Name: "Content-Type", Value: ct}}
		for i := 0; i+1 < len(headers); i += 2 {
			hs = append(hs, exchange.Header{Name: headers[i], Value: headers[i+1]})
		}
		status := 200
		if method == "DELETE" {
			status = 204
		}
		return v.ValidateResponse(context.Background(), &exchange.Request{Method: method, URL: "http://x/" + map[bool]string{true: "pets/mine", false: "things"}[method == "DELETE"]},
			&exchange.Response{Status: status, Headers: hs, Body: []byte(body)})
	}
	// A method missing from the literal template falls back to /pets/{id}.
	if got := do("DELETE", "", ""); len(got) != 0 {
		t.Errorf("DELETE /pets/mine: %+v", got)
	}
	// Non-JSON bodies are checked for their content type only.
	for _, ct := range []string{"application/xml", "text/plain; charset=utf-8"} {
		if got := do("GET", ct, "long text", "X-N", "7"); len(got) != 0 {
			t.Errorf("%s: %+v", ct, got)
		}
	}
	// A vendor JSON type is schema-checked.
	got := do("GET", "application/vnd.acme+json", `{}`, "X-N", "7")
	if len(got) != 1 || got[0].Kind != engine.ViolationBody || got[0].InstancePath != "/id" ||
		got[0].SpecPointer != "#/paths/~1things/get/responses/200/content/application~1vnd.acme+json/schema" {
		t.Errorf("vendor JSON: %+v", got)
	}
	// A header violation is located at the header.
	got = do("GET", "text/plain", "x", "X-N", "1")
	if len(got) != 1 || got[0].Kind != engine.ViolationHeader || got[0].InstancePath != "header X-N" ||
		got[0].SpecPointer != "#/paths/~1things/get/responses/200/headers/X-N/schema" || !strings.Contains(got[0].Message, "5") {
		t.Errorf("header: %+v", got)
	}
	got = do("GET", "image/png", "x", "X-N", "7")
	if len(got) != 1 || got[0].Kind != engine.ViolationContentType || got[0].SpecPointer != "#/paths/~1things/get/responses/200/content" {
		t.Errorf("content type: %+v", got)
	}
	// HEAD falls back to GET, without its body.
	if got := do("HEAD", "application/vnd.acme+json", "", "X-N", "7"); len(got) != 0 {
		t.Errorf("HEAD: %+v", got)
	}
}

// TestValidate31IgnoresFileReferences checks that a 3.1 schema's
// $dynamicRef to a local file is never followed: the file's schema would
// require a property the body lacks.
func TestValidate31IgnoresFileReferences(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.json")
	if err := os.WriteFile(target, []byte(`{"type": "object", "required": ["stolen"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := (&url.URL{Scheme: "file", Path: filepath.ToSlash(target)}).String()
	spec := "openapi: 3.1.0\ninfo: {title: t, version: '1'}\njsonSchemaDialect: https://json-schema.org/draft/2020-12/schema\npaths:\n  /a:\n    get:\n      responses:\n        '200':\n          description: ok\n" +
		"          headers:\n            X-A: {schema: {type: integer, $dynamicRef: '" + ref + "'}}\n" +
		"          content:\n            application/json:\n              schema: {type: object, $dynamicRef: '" + ref + "'}\n"
	path := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, check(s.Validator(Options{}), "GET", "http://x/a", 200, `{}`, "X-A", "1"))
}
