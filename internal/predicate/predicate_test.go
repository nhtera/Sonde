// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package predicate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

func testEnv(t *testing.T) *template.Env {
	vars := template.Vars{}
	vars.Set("base_url", value.String("http://localhost:8000"))
	vars.Set("n", value.Int(10))
	vars.Set("list", value.List{value.Int(1)})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	return &template.Env{Vars: vars, ReadFile: func(name string) ([]byte, error) {
		if name == "../secret" {
			return nil, fmt.Errorf("%w: %s", template.ErrFileAccessDenied, name)
		}
		return os.ReadFile(filepath.Join(dir, name))
	}}
}

// parsePredicate parses `variable "x" <predicate>`.
func parsePredicate(t *testing.T, pred string) *syntax.Predicate {
	t.Helper()
	src := "GET http://a\nHTTP 200\n[Asserts]\nvariable \"x\" " + pred + "\n"
	f, err := syntax.Parse("t.hurl", []byte(src), syntax.DialectHurl)
	if err != nil {
		t.Fatalf("%s: %v", pred, err)
	}
	return f.Entries[0].Response.Sections[0].Asserts[0].Predicate
}

type predCase struct {
	pred     string
	actual   value.Value
	ok       bool
	gotText  string // actual text of a failure
	wantText string // expected text of a failure
	mismatch bool
}

func date(s string) value.Date {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return value.Date(t)
}

func runPredicates(t *testing.T, cases []predCase) {
	t.Helper()
	for _, tt := range cases {
		err := Eval(parsePredicate(t, tt.pred), tt.actual, testEnv(t))
		name := tt.pred + " on " + value.Display(tt.actual)
		if tt.ok {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		var re *runerr.Error
		if !errors.As(err, &re) || re.Kind != runerr.AssertFailure {
			t.Errorf("%s: error %v, want an assert failure", name, err)
			continue
		}
		if re.Actual != tt.gotText || re.Expected != tt.wantText || re.TypeMismatch != tt.mismatch || !re.Assert {
			t.Errorf("%s:\n  actual   %q\n  expected %q\n  mismatch %v", name, re.Actual, re.Expected, re.TypeMismatch)
		}
		if re.Span.Start.Col != 0 || re.Span.Start.Line != 4 {
			t.Errorf("%s: span %v", name, re.Span)
		}
	}
}

func TestEquality(t *testing.T) {
	runPredicates(t, []predCase{
		{"== 10", value.Int(10), true, "", "", false},
		{"== 10", value.Float(10), true, "", "", false},
		{"== 10.0", value.Int(10), true, "", "", false},
		{"== 10", value.Bool(true), false, "boolean <true>", "integer <10>", false},
		{"== 10", value.Unit{}, false, "unit", "integer <10>", false},
		{"== 10", value.Int(1), false, "integer <1>", "integer <10>", false},
		{"== true", value.Bool(false), false, "boolean <false>", "boolean <true>", false},
		{"== 1.2", value.Float(1.1), false, "float <1.1>", "float <1.2>", false},
		{`== "{{base_url}}"`, value.String("http://localhost:8000"), true, "", "", false},
		{"== {{n}}", value.Int(10), true, "", "", false},
		{"== null", value.Null{}, true, "", "", false},
		{"== 10", nil, false, "none", "integer <10>", false},
		{"== null", nil, false, "none", "null", false},
		{"== hex,010203;", value.Bytes{1, 2, 3}, true, "", "", false},
		{"== base64,AQID;", value.Bytes{1, 2, 3}, true, "", "", false},
		{"== file,data.bin;", value.Bytes{1, 2, 3}, true, "", "", false},
		{"== hex,01;", nil, false, "none", "1 byte", false},
		{"== 123456789012345678901234567890", value.BigInt("123456789012345678901234567890"), true, "", "", false},
		{"== 123456789012345678901234567890", nil, false, "none", "number <123456789012345678901234567890>", false},
		{"== {{list}}", value.List{value.Float(1)}, true, "", "", false},
		{"!= 10", value.Int(1), true, "", "", false},
		{"!= 10", value.Int(10), false, "integer <10>", "integer <10>", false},
		{"not == 10", value.Int(10), false, "integer <10>", "not integer <10>", false},
		{"not == 10", value.Int(1), true, "", "", false},
		{"not == 10", nil, true, "", "", false},
	})
}

func TestComparison(t *testing.T) {
	runPredicates(t, []predCase{
		{"> 1", value.Int(2), true, "", "", false},
		{"> 1", value.Int(1), false, "integer <1>", "greater than integer <1>", false},
		{"> 1", value.Float(1.1), true, "", "", false},
		{"> 2", value.Float(1.1), false, "float <1.1>", "greater than integer <2>", false},
		{">= 1", value.Int(1), true, "", "", false},
		{">= 2", value.Int(1), false, "integer <1>", "greater or equal than integer <2>", false},
		{"< 2", value.Int(1), true, "", "", false},
		{"< 1", value.Int(1), false, "integer <1>", "less than integer <1>", false},
		{"<= 1", value.Int(1), true, "", "", false},
		{"<= 0", value.Int(1), false, "integer <1>", "less or equal than integer <0>", false},
		{`> "a"`, value.String("b"), true, "", "", false},
		{`> "a"`, value.Int(1), false, "integer <1>", `greater than string <a>`, true},
		{"not > 1", value.String("x"), false, "string <x>", "not greater than integer <1>", true},
		{"> 1", nil, false, "none", "greater than <integer <1>>", false},
		{">= 1", nil, false, "none", "greater than or equals to <integer <1>>", false},
		{"< 1", nil, false, "none", "less than <integer <1>>", false},
		{"<= 1", nil, false, "none", "less than or equals to <integer <1>>", false},
	})
}

func TestStringPredicates(t *testing.T) {
	runPredicates(t, []predCase{
		{`startsWith "to"`, value.String("toto"), true, "", "", false},
		{`startsWith "x"`, value.String("toto"), false, "string <toto>", "starts with string <x>", false},
		{`startsWith hex,01;`, value.Bytes{1, 2}, true, "", "", false},
		{`not startsWith "toto"`, value.Int(1), false, "integer <1>", "not starts with string <toto>", true},
		{`startsWith "x"`, nil, false, "none", "starts with string <x>", false},
		{`endsWith "to"`, value.String("toto"), true, "", "", false},
		{`endsWith hex,02;`, value.Bytes{1, 2}, true, "", "", false},
		{`endsWith "x"`, value.Bytes{1}, false, "bytes <01>", "ends with string <x>", true},
		{`endsWith "x"`, nil, false, "none", "ends with string <x>", false},
		{`contains "ot"`, value.String("toto"), true, "", "", false},
		{`contains hex,0203;`, value.Bytes{1, 2, 3}, true, "", "", false},
		{`contains 2`, value.List{value.Int(1), value.Float(2)}, true, "", "", false},
		{`contains 3`, value.List{value.Int(1)}, false, "list <[1]>", "contains integer <3>", false},
		{`contains "x"`, value.Int(1), false, "integer <1>", "contains string <x>", true},
		{`contains "x"`, nil, false, "none", "contains string <x>", false},
		{`includes 1`, value.List{value.Int(1)}, true, "", "", false},
		{`includes 1`, value.String("1"), false, "string <1>", "includes integer <1>", true},
		{`includes 1`, nil, false, "none", "include integer <1>", false},
		{`matches /a{3}/`, value.String("aa"), false, "string <aa>", "matches regex </a{3}/>", false},
		{`matches /^\d+$/`, value.String("١٢"), true, "", "", false},
		{`matches "^a+$"`, value.String("aa"), true, "", "", false},
		{`matches "^b"`, value.String("aa"), false, "string <aa>", "matches regex <^b>", false},
		{`matches "a"`, value.Int(1), false, "integer <1>", "matches regex <a>", true},
		{`matches /a/`, nil, false, "none", "matches regex <a>", false},
		{`matches "{{base_url}}"`, nil, false, "none", "matches regex <http://localhost:8000>", false},
	})
	err := Eval(parsePredicate(t, `matches "("`), value.String("x"), testEnv(t))
	var re *runerr.Error
	if !errors.As(err, &re) || re.Kind != runerr.InvalidRegex || re.Assert {
		t.Errorf("invalid regex: %v", err)
	}
}

func TestTypePredicates(t *testing.T) {
	runPredicates(t, []predCase{
		{"exists", value.Unit{}, true, "", "", false},
		{"exists", value.Nodeset(0), false, "nodeset <Nodeset(size=0)>", "something", false},
		{"exists", nil, false, "none", "something", false},
		{"not exists", nil, true, "", "", false},
		{"isEmpty", value.List{}, true, "", "", false},
		{"isEmpty", value.String(""), true, "", "", false},
		{"isEmpty", value.Object{}, true, "", "", false},
		{"isEmpty", value.Nodeset(0), true, "", "", false},
		{"isEmpty", value.Bytes{1}, false, "count equals to 1", "count equals to 0", false},
		{"isEmpty", value.String("café"), false, "count equals to 5", "count equals to 0", false},
		{"isEmpty", value.Int(1), false, "integer <1>", "count equals to 0", true},
		{"isEmpty", nil, false, "none", "empty", false},
		{"isInteger", value.Int(1), true, "", "", false},
		{"isInteger", value.BigInt("1e400"), true, "", "", false},
		{"isInteger", value.Float(1), false, "float <1.0>", "integer", false},
		{"isInteger", nil, false, "none", "integer", false},
		{"isFloat", value.Float(1), true, "", "", false},
		{"isNumber", value.Int(1), true, "", "", false},
		{"isNumber", value.Float(1), true, "", "", false},
		{"isNumber", value.String("1"), false, "string <1>", "number", false},
		{"isBoolean", value.Bool(false), true, "", "", false},
		{"isString", value.String(""), true, "", "", false},
		{"isCollection", value.Nodeset(1), true, "", "", false},
		{"isCollection", value.Int(1), false, "integer <1>", "collection", false},
		{"isList", value.Bytes{}, true, "", "", false},
		{"isList", value.Object{}, false, "object <Object()>", "list", false},
		{"isObject", value.Nodeset(1), true, "", "", false},
		{"isDate", date("2002-06-16T10:10:10Z"), true, "", "", false},
		{"isDate", value.String("toto"), false, "string <toto>", "date", false},
		{"not isDate", date("2002-06-16T10:10:10Z"), false, "date <2002-06-16 10:10:10 UTC>", "not date", false},
		{"isIsoDate", value.String("2020-03-09T22:18:26.625Z"), true, "", "", false},
		{"isIsoDate", value.String("2020-03-09"), false, "2020-03-09", "string with format YYYY-MM-DDTHH:mm:ss.sssZ", false},
		{"isIsoDate", value.Int(1), false, "integer <1>", "string", true},
		{"isIsoDate", nil, false, "none", "date", false},
		{"isIpv4", value.String("192.168.0.1"), true, "", "", false},
		{"isIpv4", value.String("192.168.0.01"), false, "192.168.0.01", "string in IPv4 format", false},
		{"isIpv4", value.String("::1"), false, "::1", "string in IPv4 format", false},
		{"isIpv4", value.Int(1), false, "integer <1>", "string", true},
		{"isIpv6", value.String("::ffff:1.2.3.4"), true, "", "", false},
		{"isIpv6", value.String("fe80::1%eth0"), false, "fe80::1%eth0", "string in IPv6 format", false},
		{"isIpv6", nil, false, "none", "ipv6", false},
		{"isIpv4", nil, false, "none", "ipv4", false},
		{"isUuid", value.String("67e55044-10b1-426f-9247-bb680e5fe0c8"), true, "", "", false},
		{"isUuid", value.String("67E5504410B1426F9247BB680E5FE0C8"), true, "", "", false},
		{"isUuid", value.String("{67e55044-10b1-426f-9247-bb680e5fe0c8}"), true, "", "", false},
		{"isUuid", value.String("urn:uuid:67e55044-10b1-426f-9247-bb680e5fe0c8"), true, "", "", false},
		{"isUuid", value.String("67e55044-10b1-426f-9247-bb680e5fe0cz"), false, "67e55044-10b1-426f-9247-bb680e5fe0cz", "string in UUID format", false},
		{"isUuid", value.String("67e55044_10b1-426f-9247-bb680e5fe0c8"), false, "67e55044_10b1-426f-9247-bb680e5fe0c8", "string in UUID format", false},
		{"isUuid", value.Bytes{}, false, "bytes <>", "string", true},
		{"isUuid", nil, false, "none", "uuid", false},
		{"isString", nil, false, "none", "string", false},
	})
}

func TestPredicateValueErrors(t *testing.T) {
	env := testEnv(t)
	tests := []struct{ pred, msg string }{
		{"== {{missing}}", "Undefined variable: you must set the variable missing"},
		{"== file,nope.bin;", "File read access: file nope.bin can not be read"},
		{"== file,../secret;", "Unauthorized file access: unauthorized access to file ../secret, check --file-root option"},
		{`== "{{missing}}"`, "Undefined variable: you must set the variable missing"},
	}
	for _, tt := range tests {
		for _, actual := range []value.Value{value.Int(1), nil} {
			err := Eval(parsePredicate(t, tt.pred), actual, env)
			if err == nil || err.Error() != tt.msg {
				t.Errorf("%s: %v", tt.pred, err)
			}
		}
	}
	noFiles := &template.Env{Vars: template.Vars{}}
	if err := Eval(parsePredicate(t, "== file,a;"), value.Int(1), noFiles); err == nil || err.Error() != "File read access: file a can not be read" {
		t.Errorf("no ReadFile: %v", err)
	}
}

func TestMultilinePredicateValue(t *testing.T) {
	src := "GET http://a\nHTTP 200\n[Asserts]\nvariable \"x\" == ```\nline {{n}}\n```\nvariable \"x\" == ```raw\n{{n}}\n```\n" +
		"variable \"x\" == ```graphql\nquery { a }\n\nvariables {\"n\": {{n}}, \"s\": \"a\\\"b\"}\n```\n"
	f, err := syntax.Parse("t.hurl", []byte(src), syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	asserts := f.Entries[0].Response.Sections[0].Asserts
	want := []string{"line 10\n", "{{n}}\n", `{"query":"query { a }","variables":{"n":10,"s":"a\"b"}}`}
	for i, a := range asserts {
		v, err := Value(a.Predicate.Func.Value, a.Predicate.Func.Span, testEnv(t))
		if err != nil || v != value.String(want[i]) {
			t.Errorf("assert %d: %q, %v; want %q", i, v, err, want[i])
		}
	}
}
