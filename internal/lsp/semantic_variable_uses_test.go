// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"reflect"
	"sort"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// variableUsesFixture exercises every placeholder location variableUses
// dispatches on: the URL, a header key and value, [Query], [Cookies],
// [BasicAuth], [MultipartFormData] (a plain field and a file field with a
// content type), [Options] (a variable definition and a bare option
// value), a response header, [Captures] (name, query argument and a
// filter argument), [Asserts] (a query argument, a cookie path, a filter
// argument, a predicate value and a multiline string), a JSON body (a
// nested list and object), a file body and a GraphQL body's variables
// block.
const variableUsesFixture = `GET http://a/{{vUrl}}
{{vHKey}}: v
H: {{vHVal}}
[Query]
q: {{vQuery}}
[Cookies]
c: {{vCookie}}
[BasicAuth]
{{vAuthUser}}: {{vAuthPass}}
[MultipartFormData]
m: {{vMultipartVal}}
f: file,{{vFilename}}; {{vContentType}}
[Options]
variable: myvar={{vOptVarDef}}
delay: {{vOptDelay}}
HTTP 200
RH: {{vRespHeader}}
[Captures]
{{vCapName}}: jsonpath "$.{{vCapQueryArg}}" nth {{vCapFilterNth}}
[Asserts]
header "{{vAssertHeaderArg}}" == "{{vAssertPredVal}}"
cookie "{{vAssertCookieName}}[Value]" exists
body nth {{vAssertFilterNth}} == "x"
body == ` + "```" + `
plain {{vMultilineBody}}
` + "```" + `

POST http://b/
{"a": {{vBodyA}}, "b": [{{vBodyB}}, "lit"]}
HTTP 200

POST http://c/
file,{{vBodyFile}};
HTTP 200

POST http://g/
` + "```graphql,\nquery Q($id: ID) { a(id: $id) }\nvariables {\"id\": \"{{vGraphQL}}\"}\n```" + `
HTTP 200
`

// TestVariableUsesMatchesReflectionWalk cross-checks variableUses'
// hand-written traversal against uses' generic reflection-based one
// (variables.go) on a fixture exercising every placeholder location: they
// must find exactly the same set of variable references. variableUses
// exists only because uses is too slow for the 1,000-line diagnostics
// budget (see TestLargeFileDiagnosticsBudget under -race); this test is
// what keeps it honest as the AST evolves.
func TestVariableUsesMatchesReflectionWalk(t *testing.T) {
	d := newDocument("file:///w/fixture.hurl", 1, variableUsesFixture, true)
	if len(d.errs) != 0 {
		t.Fatalf("fixture has %d parse errors: %v", len(d.errs), d.errs)
	}

	got := sortedUseNames(variableUses(d.file))
	want := sortedUseNames(usesByReflection(d.file))
	if len(got) != len(want) {
		t.Fatalf("variableUses found %d names, uses found %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("name %d: variableUses=%q, uses=%q", i, got[i], want[i])
		}
	}
}

func sortedUseNames(uses []use) []string {
	names := make([]string, len(uses))
	for i, u := range uses {
		names[i] = u.name
	}
	sort.Strings(names)
	return names
}

// usesByReflection is the test oracle for variableUses: it finds every
// placeholder reachable in the AST, whatever node holds it.
func usesByReflection(f *syntax.File) []use {
	var out []use
	reflectWalk(reflect.ValueOf(f), func(p *syntax.Placeholder) {
		if p.Expr.Kind == syntax.ExprVariable {
			out = append(out, use{name: p.Expr.Name, span: p.Expr.Span})
		}
	})
	return out
}

var placeholderType = reflect.TypeFor[*syntax.Placeholder]()

// walk calls fn for every *syntax.Placeholder reachable from v. The AST is
// a tree, so no visited set is needed.
func reflectWalk(v reflect.Value, fn func(*syntax.Placeholder)) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return
		}
		if v.Type() == placeholderType {
			fn(v.Interface().(*syntax.Placeholder))
			return
		}
		reflectWalk(v.Elem(), fn)
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				reflectWalk(v.Field(i), fn)
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			reflectWalk(v.Index(i), fn)
		}
	}
}
