// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/nhtera/sonde/internal/syntax"
)

var update = flag.Bool("update", false, "rewrites the golden files")

// TestGenerateGolden generates every fixture and compares the files,
// variables and warnings with testdata/openapi/import/<fixture>.golden.
func TestGenerateGolden(t *testing.T) {
	for _, tc := range []struct{ spec, group string }{
		{"petstore-3.0.yaml", GroupTag},
		{"petstore-3.1.yaml", GroupFlat},
		{"edge-cases-3.1.yaml", GroupPath},
		{"swagger-2.0.yaml", GroupTag},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			gen, err := load(t, tc.spec, LoadOptions{}).Generate(GenerateOptions{Group: tc.group})
			if err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			for _, f := range gen.Files {
				out := syntax.Lint(f.File)
				if _, err := syntax.Parse(f.Path+".hurl", out, syntax.DialectHurl); err != nil {
					t.Errorf("%s does not parse: %v", f.Path, err)
				}
				fmt.Fprintf(&b, "== %s\n%s", f.Path, out)
			}
			names := make([]string, 0, len(gen.Variables))
			for n := range gen.Variables {
				names = append(names, n)
			}
			sort.Strings(names)
			b.WriteString("== variables\n")
			for _, n := range names {
				fmt.Fprintf(&b, "%s=%s\n", n, gen.Variables[n])
			}
			for _, w := range gen.Warnings {
				fmt.Fprintf(&b, "warning %s: %s\n", w.Kind, w.Message)
			}
			for _, s := range gen.Skipped {
				fmt.Fprintf(&b, "skipped %s: %s\n", s.Kind, s.Message)
			}
			golden := filepath.Join("../../testdata/openapi/import", strings.TrimSuffix(tc.spec, ".yaml")+".golden")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(b.String()), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden) //nolint:gosec // G304: test fixture
			if err != nil {
				t.Fatal(err)
			}
			if b.String() != string(want) {
				t.Errorf("output differs from %s (go test -run TestGenerateGolden -update):\n%s", golden, b.String())
			}
		})
	}
}

func TestGenerateOptions(t *testing.T) {
	s := load(t, "petstore-3.0.yaml", LoadOptions{})
	if _, err := s.Generate(GenerateOptions{Group: "folder"}); err == nil {
		t.Error("no error for an unknown group")
	}
	if _, err := s.Generate(GenerateOptions{BaseURLVar: "base url"}); err == nil {
		t.Error("no error for an invalid variable name")
	}
	gen, err := s.Generate(GenerateOptions{BaseURLVar: "api"})
	if err != nil {
		t.Fatal(err)
	}
	if gen.Variables["api"] == "" || !strings.Contains(string(syntax.Lint(gen.Files[0].File)), "{{api}}/") {
		t.Errorf("base URL variable not used: %v", gen.Variables)
	}
}

// TestExamplesConform checks that the example of every request and
// response schema of the fixtures is valid against the schema.
func TestExamplesConform(t *testing.T) {
	for _, name := range []string{"petstore-3.0.yaml", "petstore-3.1.yaml", "edge-cases-3.1.yaml"} {
		s := load(t, name, LoadOptions{})
		var opts []openapi3.SchemaValidationOption
		if s.doc.IsOpenAPI31OrLater() {
			opts = append(opts, openapi3.EnableJSONSchema2020())
		}
		checked := 0
		visit := func(where string, ref *openapi3.SchemaRef) {
			if ref == nil || ref.Value == nil {
				return
			}
			checked++
			v := jsonRoundTrip(t, Example(ref))
			if err := ref.Value.VisitJSON(v, opts...); err != nil {
				t.Errorf("%s %s: example %v does not conform: %v", name, where, v, err)
			}
		}
		for _, tmpl := range s.templates {
			for m, op := range s.doc.Paths.Value(tmpl).Operations() {
				where := m + " " + tmpl
				if op.RequestBody != nil && op.RequestBody.Value != nil {
					for mt, media := range op.RequestBody.Value.Content {
						visit(where+" request "+mt, media.Schema)
					}
				}
				for code, r := range op.Responses.Map() {
					for mt, media := range r.Value.Content {
						visit(where+" "+code+" "+mt, media.Schema)
					}
				}
			}
		}
		if checked == 0 {
			t.Errorf("%s: no schema checked", name)
		}
	}
}

func TestExampleSchemas(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	u := func(v uint64) *uint64 { return &v }
	for _, tc := range []struct {
		schema *openapi3.Schema
		want   string
	}{
		{&openapi3.Schema{Type: &openapi3.Types{"integer"}, Min: f(10)}, "10"},
		{&openapi3.Schema{Type: &openapi3.Types{"integer"}, Max: f(-3)}, "-3"},
		{&openapi3.Schema{Type: &openapi3.Types{"number"}, ExclusiveMin: openapi3.ExclusiveBound{Value: f(5)}}, "6"},
		{&openapi3.Schema{Type: &openapi3.Types{"string"}, MinLength: 10}, "stringxxxx"},
		{&openapi3.Schema{Type: &openapi3.Types{"string"}, MaxLength: u(3)}, "str"},
		{&openapi3.Schema{Type: &openapi3.Types{"string", "null"}, Format: "email"}, "user@example.com"},
		{&openapi3.Schema{Type: &openapi3.Types{"null"}}, "<nil>"},
		{&openapi3.Schema{Type: &openapi3.Types{"array"}, MinItems: 2, Items: openapi3.NewSchemaRef("", openapi3.NewBoolSchema())}, "[true true]"},
		{&openapi3.Schema{Enum: []any{"b", "a"}}, "b"},
		{&openapi3.Schema{OneOf: openapi3.SchemaRefs{openapi3.NewSchemaRef("", openapi3.NewStringSchema())}}, "string"},
	} {
		if got := fmt.Sprint(Example(openapi3.NewSchemaRef("", tc.schema))); got != tc.want {
			t.Errorf("Example(%+v) = %s, want %s", tc.schema, got, tc.want)
		}
	}
	// A recursive schema ends.
	node := &openapi3.Schema{Type: &openapi3.Types{"object"}, Required: []string{"next"}}
	node.Properties = openapi3.Schemas{"next": openapi3.NewSchemaRef("", node)}
	if Example(openapi3.NewSchemaRef("", node)) == nil {
		t.Error("recursive schema: nil example")
	}
}

func jsonRoundTrip(t *testing.T, v any) any {
	t.Helper()
	jv := jsonValue(v, nil)
	b, err := syntax.JSONBody(jv)
	if err != nil {
		t.Fatal(err)
	}
	_ = b
	return normalize(jv)
}

// normalize converts json.Number values to float64, as decoded JSON is.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
	case fmt.Stringer:
		var f float64
		if _, err := fmt.Sscan(x.String(), &f); err == nil {
			return f
		}
	}
	return v
}

var _ = context.Background

// TestOperationServersAndNames checks a spec whose servers are on its
// operations and paths, not at the top: the one most operations use is
// base_url, another is written into its operation's URL. A generated operationId loses to a
// shorter summary. A {{name}} in an example stays a variable.
func TestOperationServersAndNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.yaml")
	spec := `openapi: 3.0.0
info: {title: t, version: "1"}
paths:
  /me:
    servers: [{url: "http://localhost:8080"}]
    get:
      summary: Who am I
      operationId: users_me_documents_collection_auth_who_am_i_yml
      responses: {"200": {description: ok}}
    delete:
      servers: [{url: "http://localhost:8080"}]
      parameters:
        - {name: Cookie, in: header, required: true, schema: {type: string}, example: "{{session-cookie}}"}
      requestBody:
        content:
          application/json:
            example: {name: "{{name}} (copy)"}
      responses: {"204": {description: ok}}
  /policies//duplicate:
    post:
      servers: [{url: "http://localhost:8080"}]
      responses: {"200": {description: ok}}
  /pets:
    servers: [{url: "http://pets.local:9000/"}]
    get:
      summary: List all the pets there are
      operationId: listPets
      responses: {"200": {description: ok}}
`
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gen, err := s.Generate(GenerateOptions{Group: GroupFlat})
	if err != nil {
		t.Fatal(err)
	}
	if gen.Variables["base_url"] != "http://localhost:8080" || fmt.Sprint(gen.Warnings) != "[{path POST /policies//duplicate: the path has an empty segment}]" {
		t.Errorf("base_url %q, warnings %v", gen.Variables["base_url"], gen.Warnings)
	}
	got := map[string]string{}
	for _, f := range gen.Files {
		for _, line := range strings.Split(string(syntax.Lint(f.File)), "\n") {
			if !strings.HasPrefix(line, "#") {
				got[f.Path] = line // the request line
				break
			}
		}
	}
	want := map[string]string{"Who am I": "GET {{base_url}}/me", "delete-me": "DELETE {{base_url}}/me", "list-pets": "GET http://pets.local:9000/pets", "post-policies--duplicate": "POST {{base_url}}/policies//duplicate"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("files %v, want %v", got, want)
	}
	for _, f := range gen.Files {
		if src := string(syntax.Lint(f.File)); f.Path == "delete-me" &&
			(!strings.Contains(src, "Cookie: {{session-cookie}}") || !strings.Contains(src, `{"name": "{{name}} (copy)"}`)) {
			t.Errorf("placeholders not kept:\n%s", src)
		}
	}
}
