// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/syntaxedit"
)

const suggestCollection = `{
  "info": {"name": "c", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
  "item": [
    {"name": "me", "request": {"method": "GET", "url": "https://api.test/me"},
     "event": [{"listen": "test", "script": {"exec": [
       "pm.test(\"ok\", function () {",
       "    pm.response.to.have.status(200);",
       "    var jsonData = pm.response.json();",
       "    pm.expect(jsonData.user.id).to.eql(42);",
       "    pm.expect(pm.response.json()[\"items\"][0].name).to.equal(\"a . b\");",
       "    pm.expect( pm.response.headers.get('Content-Type') ).to.eql('application/json');",
       "    pm.expect(pm.response.code).to.equal(200);",
       "    pm.response.to.have.header(\"X-Req\");",
       "    pm.environment.set(\"token\", jsonData.access_token);",
       "    pm.collectionVariables.set('user-id', jsonData.user.id);",
       "    // pm.expect(jsonData.x).to.eql(1);",
       "    pm.expect(jsonData.list).to.eql([1, 2]);",
       "    pm.expect(other.bar).to.eql(1);",
       "    pm.expect(jsonData.a).to.be.above(1);",
       "    pm.environment.set(\"t2\", \"constant\");",
       "});"
     ]}}]},
    {"name": "orders", "request": {"method": "GET", "url": "https://api.test/orders",
     "auth": {"type": "oauth2", "oauth2": [
       {"key": "accessTokenUrl", "value": "https://auth.test/token"},
       {"key": "grant_type", "value": "client_credentials"},
       {"key": "clientId", "value": "{{client_id}}"},
       {"key": "clientSecret", "value": "{{client_secret}}"}
     ]}}}
  ]
}`

func TestSuggest(t *testing.T) {
	out, err := Import([]byte(suggestCollection), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	sugg, err := Suggest([]byte(suggestCollection), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(sugg) != 3 {
		t.Fatalf("suggestions %+v", sugg)
	}
	asserts := []Op{
		{Entry: 1, Section: syntaxedit.Asserts, Value: `jsonpath "$.user.id" == 42`},
		{Entry: 1, Section: syntaxedit.Asserts, Value: `jsonpath "$['items'][0].name" == "a . b"`},
		{Entry: 1, Section: syntaxedit.Asserts, Value: `header "Content-Type" == "application/json"`},
		{Entry: 1, Section: syntaxedit.Asserts, Value: `status == 200`},
		{Entry: 1, Section: syntaxedit.Asserts, Value: `header "X-Req" exists`},
	}
	if s := sugg[0]; s.File != 0 || !reflect.DeepEqual(s.Ops, asserts) {
		t.Errorf("asserts %+v", s.Ops)
	}
	captures := []Op{
		{Entry: 1, Section: syntaxedit.Captures, Key: "token", Value: `jsonpath "$.access_token"`},
		{Entry: 1, Section: syntaxedit.Captures, Key: "user-id", Value: `jsonpath "$.user.id"`},
	}
	if s := sugg[1]; s.File != 0 || !reflect.DeepEqual(s.Ops, captures) {
		t.Errorf("captures %+v", s.Ops)
	}
	login := sugg[2]
	if login.File != 1 || len(login.Ops) != 2 || login.Ops[0].Login == nil || login.Ops[1].Value != "Bearer {{access_token}}" {
		t.Fatalf("login %+v", login)
	}

	for _, s := range sugg {
		name := out.Files[s.File].Path + ".hurl"
		src := syntax.Format(out.Files[s.File].File)
		got, err := s.Apply(name, src)
		if err != nil {
			t.Fatalf("%s: %v", s.Label, err)
		}
		if _, err := syntax.Parse(name, got, syntax.DialectHurl); err != nil {
			t.Fatalf("%s: %v\n%s", s.Label, err, got)
		}
		switch s.File {
		case 0:
			if !strings.Contains(string(got), `jsonpath "$.user.id" == 42`) && !strings.Contains(string(got), `token: jsonpath "$.access_token"`) {
				t.Errorf("%s:\n%s", s.Label, got)
			}
		case 1:
			want := "grant_type: client_credentials\nclient_id: {{client_id}}\nclient_secret: {{client_secret}}\nHTTP 200\n[Captures]\naccess_token: jsonpath \"$.access_token\" redact\n"
			if !strings.Contains(string(got), want) || !strings.Contains(string(got), "Authorization: Bearer {{access_token}}") {
				t.Errorf("%s:\n%s", s.Label, got)
			}
		}
	}
}

func TestSuggestFolderEntries(t *testing.T) {
	col := `{"info": {"name": "c", "schema": "v2.1.0"}, "item": [
	  {"name": "a", "request": {"method": "GET", "url": "https://x/a"}},
	  {"name": "b", "request": {"method": "GET", "url": "https://x/b"},
	   "event": [{"listen": "test", "script": {"exec": "pm.expect(pm.response.json().ok).to.eql(true)"}}]}]}`
	sugg, err := Suggest([]byte(col), Options{Group: GroupFolder, Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(sugg) != 1 || sugg[0].Ops[0].Entry != 2 || sugg[0].Ops[0].Value != `jsonpath "$.ok" == true` {
		t.Errorf("suggestions %+v", sugg)
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		`  pm.expect( a . b ) .to .eql( "x . y" ) `: `pm.expect(a.b).to.eql("x . y")`,
		`var  d=pm.response.json( )`:                `var d = pm.response.json()`,
		`a === b`:                                   `a===b`,
		`f(a ,b)`:                                   `f(a, b)`,
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
