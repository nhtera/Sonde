// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// examplesSpec documents named examples, headers, several media types and
// readOnly/writeOnly properties.
const examplesSpec = `openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /users/{id}:
    get:
      parameters:
        - {name: id, in: path, required: true, schema: {type: integer}}
      responses:
        "200":
          description: ok
          headers:
            X-Request-Id: {schema: {type: string, format: uuid}}
            X-Version: {example: "7", schema: {type: string}}
            X-Tags: {example: [a, b], schema: {type: array, items: {type: string}}}
            Content-Type: {schema: {type: string}}
          content:
            application/json:
              schema: {$ref: "#/components/schemas/User"}
              examples:
                bob: {value: {id: 2, name: bob}}
                alice: {value: {id: 1, name: alice}}
            text/plain:
              example: a user
            application/xml:
              schema: {$ref: "#/components/schemas/User"}
        "4XX":
          description: client error
          content:
            application/problem+json:
              schema: {type: object, required: [title], properties: {title: {type: string}}}
    head:
      responses:
        "200": {description: ok}
    put:
      parameters:
        - {name: id, in: path, required: true, schema: {type: integer}}
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: "#/components/schemas/User"}
      responses:
        default: {description: any}
components:
  schemas:
    User:
      type: object
      required: [id, name, password]
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string}
        password: {type: string, writeOnly: true}
`

func loadText(t *testing.T, text string) *Spec {
	t.Helper()
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), p, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResponseExampleDirection(t *testing.T) {
	s := loadText(t, examplesSpec)
	user := s.doc.Components.Schemas["User"]
	if got, want := Example(user), map[string]any{"name": "string", "password": "string"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Example = %v, want %v", got, want)
	}
	if got, want := ResponseExample(user), map[string]any{"id": int64(1), "name": "string"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ResponseExample = %v, want %v", got, want)
	}
}

func TestMockMatch(t *testing.T) {
	m := load(t, "petstore-3.0.yaml", LoadOptions{}).Mock("")
	for _, tc := range []struct {
		method, path, name string
		params             map[string]string
	}{
		{"GET", "/v1/pets", "GET /pets", nil},
		{"get", "/v1/pets/", "GET /pets", nil},
		{"GET", "/v1/pets/mine", "GET /pets/mine", nil},
		{"GET", "/v1/pets/7", "GET /pets/{petId}", map[string]string{"petId": "7"}},
		{"HEAD", "/v1/pets/7", "GET /pets/{petId}", map[string]string{"petId": "7"}},
		{"DELETE", "/v1/pets/7", "DELETE /pets/{petId}", map[string]string{"petId": "7"}},
	} {
		op, p := m.Match(tc.method, tc.path)
		if p != nil {
			t.Errorf("%s %s: %v", tc.method, tc.path, p)
			continue
		}
		if op.Name() != tc.name || !reflect.DeepEqual(op.Params, tc.params) {
			t.Errorf("%s %s = %s %v, want %s %v", tc.method, tc.path, op.Name(), op.Params, tc.name, tc.params)
		}
	}
	if _, p := m.Match("GET", "/v2/pets"); p == nil || p.Status != http.StatusNotFound {
		t.Errorf("unknown path: %+v", p)
	}
	if _, p := loadText(t, examplesSpec).Mock("").Match("POST", "/users/1"); p == nil || !reflect.DeepEqual(p.Allow, []string{"GET", "HEAD", "PUT"}) {
		t.Errorf("documented HEAD: %+v", p)
	}
	if _, p := m.Match("PUT", "/v1/pets/7"); p == nil || p.Status != http.StatusMethodNotAllowed ||
		!reflect.DeepEqual(p.Allow, []string{"DELETE", "GET", "HEAD"}) {
		t.Errorf("unknown method: %+v", p)
	}
	// --server replaces the spec's base path.
	m = load(t, "petstore-3.0.yaml", LoadOptions{}).Mock("http://localhost:4010/api")
	if _, p := m.Match("GET", "/api/pets"); p != nil {
		t.Errorf("--server: %v", p)
	}
	if _, p := m.Match("GET", "/v1/pets"); p == nil {
		t.Error("--server: the spec's base path still matches")
	}
}

func respond(t *testing.T, m *Mock, method, path string, sel Selection) (*Response, *Problem) {
	t.Helper()
	op, p := m.Match(method, path)
	if p != nil {
		t.Fatalf("%s %s: %v", method, path, p)
	}
	return m.Respond(op, sel)
}

func TestMockRespond(t *testing.T) {
	pets := load(t, "petstore-3.0.yaml", LoadOptions{}).Mock("")
	users := loadText(t, examplesSpec).Mock("")
	for _, tc := range []struct {
		name   string
		m      *Mock
		method string
		path   string
		sel    Selection
		status int
		ct     string
		body   string
		header map[string]string
	}{
		{name: "lowest 2xx", m: pets, method: "GET", path: "/v1/pets", status: 200, ct: "application/json",
			body: `[{"id":1,"name":"string"}]`, header: map[string]string{"X-Rate-Limit": "1"}},
		{name: "201", m: pets, method: "POST", path: "/v1/pets", status: 201, ct: "application/json", body: `{"id":1,"name":"string"}`},
		{name: "no content", m: pets, method: "DELETE", path: "/v1/pets/1", status: 204},
		{name: "Prefer code", m: pets, method: "GET", path: "/v1/pets/1", sel: Selection{Status: "404"}, status: 404,
			ct: "application/json", body: `{"code":1,"message":"string"}`},
		{name: "Prefer code via default", m: pets, method: "GET", path: "/v1/pets", sel: Selection{Status: "503"}, status: 503,
			ct: "application/json", body: `{"code":1,"message":"string"}`},
		{name: "first example by name", m: users, method: "GET", path: "/users/1", status: 200, ct: "application/json",
			body: `{"id":1,"name":"alice"}`, header: map[string]string{"X-Request-Id": "00000000-0000-4000-8000-000000000000", "X-Version": "7", "X-Tags": "a,b"}},
		{name: "named example", m: users, method: "GET", path: "/users/1", sel: Selection{Example: "bob"}, status: 200,
			ct: "application/json", body: `{"id":2,"name":"bob"}`},
		{name: "Accept", m: users, method: "GET", path: "/users/1", sel: Selection{Accept: "text/plain;q=0.9, application/json;q=0.1"},
			status: 200, ct: "text/plain", body: "a user"},
		{name: "Accept q=0", m: users, method: "GET", path: "/users/1", sel: Selection{Accept: "text/plain;q=0, */*"},
			status: 200, ct: "application/json", body: `{"id":1,"name":"alice"}`},
		{name: "Accept excludes", m: users, method: "GET", path: "/users/1", sel: Selection{Accept: "*/*, application/json;q=0"},
			status: 200, ct: "text/plain", body: "a user"},
		// A structured example is never sent as XML: the next acceptable type answers.
		{name: "XML skipped", m: users, method: "GET", path: "/users/1", sel: Selection{Accept: "application/xml, application/json;q=0.5"},
			status: 200, ct: "application/json", body: `{"id":1,"name":"alice"}`},
		{name: "range", m: users, method: "GET", path: "/users/1", sel: Selection{Status: "409"}, status: 409,
			ct: "application/problem+json", body: `{"title":"string"}`},
		{name: "default only", m: users, method: "PUT", path: "/users/1", status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, p := respond(t, tc.m, tc.method, tc.path, tc.sel)
			if p != nil {
				t.Fatal(p)
			}
			if r.Status != tc.status || r.Header.Get("Content-Type") != tc.ct || string(r.Body) != tc.body || r.Warning != "" {
				t.Errorf("got %d %q %s (warning %q), want %d %q %s", r.Status, r.Header.Get("Content-Type"), r.Body, r.Warning, tc.status, tc.ct, tc.body)
			}
			for k, v := range tc.header {
				if got := r.Header.Get(k); got != v {
					t.Errorf("header %s = %q, want %q", k, got, v)
				}
			}
		})
	}
	for _, tc := range []struct {
		name, path string
		sel        Selection
		status     int
		detail     string
	}{
		{"undocumented code", "/users/1", Selection{Status: "302"}, 400, "does not document status 302; documented: 200, 4XX"},
		{"bad code", "/users/1", Selection{Status: "abc"}, 400, "not a final HTTP status"},
		{"informational code", "/users/1", Selection{Status: "100"}, 400, "not a final HTTP status"},
		{"only excluded", "/users/1", Selection{Accept: "application/json;q=0"}, 406, "no media type matching"},
		{"XML only", "/users/1", Selection{Accept: "application/xml"}, 406, "add a string example"},
		{"unknown example", "/users/1", Selection{Example: "carol"}, 400, `no example "carol"; documented: alice, bob`},
		{"not acceptable", "/users/1", Selection{Accept: "image/png"}, 406, "documented: application/json, application/xml, text/plain"},
	} {
		if _, p := respond(t, users, "GET", tc.path, tc.sel); p == nil || p.Status != tc.status || !strings.Contains(p.Detail, tc.detail) {
			t.Errorf("%s: %+v, want %d %q", tc.name, p, tc.status, tc.detail)
		}
	}
}

func TestMockRespondWarnsOnUnsatisfiableSchema(t *testing.T) {
	m := loadText(t, `openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /code:
    get:
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {type: string, pattern: "^[0-9]+$"}
`).Mock("")
	r, p := respond(t, m, "GET", "/code", Selection{})
	if p != nil || string(r.Body) != `"string"` || !strings.Contains(r.Warning, "GET /code 200 does not match its schema") {
		t.Errorf("got %+v %v", r, p)
	}
}

func TestMockValidateRequest(t *testing.T) {
	m := load(t, "petstore-3.0.yaml", LoadOptions{}).Mock("")
	for _, tc := range []struct {
		method, path, ct, body string
		status                 int
		violations             []string
	}{
		{"POST", "/v1/pets", "application/json", `{"id":1,"name":"rex"}`, 0, nil},
		// Security requirements are not enforced.
		{"GET", "/v1/pets/mine", "", "", 0, nil},
		{"GET", "/v1/pets?limit=500", "", "", 422, []string{`query parameter "limit": number must be at most 100 (maximum)`}},
		{"GET", "/v1/pets/abc", "", "", 422, []string{`path parameter "petId": value abc: an invalid integer: invalid syntax`}},
		{"POST", "/v1/pets", "application/json", `{"id":1,"name":3}`, 422, []string{"body /name: value must be a string (type)"}},
		{"POST", "/v1/pets", "application/json", "", 422, []string{"body: required but missing"}},
		{"POST", "/v1/pets", "text/plain", "rex", 415, nil},
		// Without a body the content type does not matter.
		{"POST", "/v1/pets", "text/plain", "", 422, []string{"body: required but missing"}},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.ct != "" {
			r.Header.Set("Content-Type", tc.ct)
		}
		op, p := m.Match(r.Method, r.URL.EscapedPath())
		if p != nil {
			t.Fatal(p)
		}
		p = m.ValidateRequest(context.Background(), op, r)
		switch {
		case tc.status == 0 && p != nil:
			t.Errorf("%s %s: %+v", tc.method, tc.path, p)
		case tc.status != 0 && (p == nil || p.Status != tc.status || !reflect.DeepEqual(p.Violations, tc.violations)):
			t.Errorf("%s %s %s: %+v, want %d %q", tc.method, tc.path, tc.body, p, tc.status, tc.violations)
		}
	}
}

func TestNegotiateWildcardContent(t *testing.T) {
	content := openapi3.Content{"*/*": &openapi3.MediaType{}}
	for accept, want := range map[string]string{"": "application/json", "text/csv": "text/csv", "text/*": "text/plain", "image/*": "application/octet-stream"} {
		if c := negotiate(content, accept); len(c) == 0 || c[0].contentType != want {
			t.Errorf("Accept %q: %v, want %q", accept, c, want)
		}
	}
}
