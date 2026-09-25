// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

var now = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func testEnv() *template.Env {
	vars := template.Vars{}
	vars.Set("idx", value.Int(1))
	vars.Set("fidx", value.Float(1))
	vars.Set("sep", value.String(","))
	return &template.Env{Vars: vars, Now: func() time.Time { return now }}
}

// parseFilters parses `variable "x" <filters> exists` and returns its filters.
func parseFilters(t *testing.T, filters string) []*syntax.FilterItem {
	t.Helper()
	src := "GET http://a\nHTTP 200\n[Asserts]\nvariable \"x\" " + filters + " exists\n"
	f, err := syntax.Parse("t.hurl", []byte(src), syntax.DialectHurl)
	if err != nil {
		t.Fatalf("%s: %v", filters, err)
	}
	return f.Entries[0].Response.Sections[0].Asserts[0].Filters
}

type filterCase struct {
	filters string
	in      value.Value
	want    value.Value // nil: no value
	err     string      // expected error message (Description: Message)
}

func runCases(t *testing.T, cases []filterCase) {
	t.Helper()
	for _, tt := range cases {
		got, err := Apply(parseFilters(t, tt.filters), tt.in, testEnv(), true)
		if tt.err != "" {
			if err == nil || err.Error() != tt.err {
				t.Errorf("%s on %s: error %v, want %q", tt.filters, value.Repr(tt.in), err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s on %s: %v", tt.filters, value.Repr(tt.in), err)
			continue
		}
		if (got == nil) != (tt.want == nil) || got != nil && !value.Equal(got, tt.want) {
			t.Errorf("%s on %s = %#v, want %#v", tt.filters, value.Repr(tt.in), got, tt.want)
		}
	}
}

func list(vs ...value.Value) value.List { return value.List(vs) }

func strs(ss ...string) value.List {
	l := make(value.List, len(ss))
	for i, s := range ss {
		l[i] = value.String(s)
	}
	return l
}

func TestCollectionFilters(t *testing.T) {
	runCases(t, []filterCase{
		{"count", list(value.Int(1), value.Int(2)), value.Int(2), ""},
		{"count", value.Bytes{1, 2, 3}, value.Int(3), ""},
		{"count", value.Nodeset(4), value.Int(4), ""},
		{"count", value.String("abc"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: list, bytes or nodeset"},
		{"first", strs("a", "b"), value.String("a"), ""},
		{"last", strs("a", "b"), value.String("b"), ""},
		{"first", list(), nil, "Filter error: invalid filter input: list is empty"},
		{"last", list(), nil, "Filter error: invalid filter input: list is empty"},
		{"first", value.String("a"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: list"},
		{"nth 0", strs("a", "b", "c"), value.String("a"), ""},
		{"nth 2", strs("a", "b", "c"), value.String("c"), ""},
		{"nth -1", strs("a", "b", "c"), value.String("c"), ""},
		{"nth -3", strs("a", "b", "c"), value.String("a"), ""},
		{"nth 3", strs("a", "b", "c"), nil, "Filter error: invalid filter input: out of bound - size is 3"},
		{"nth -4", strs("a", "b", "c"), nil, "Filter error: invalid filter input: out of bound - size is 3"},
		{"nth {{idx}}", strs("a", "b"), value.String("b"), ""},
		{"nth {{fidx}}", strs("a", "b"), nil, "Invalid expression type: expecting integer, actual value is float <1.0>"},
		{"nth {{nope}}", strs("a", "b"), nil, "Undefined variable: you must set the variable nope"},
		{"nth 0", value.String("a"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: list"},
		{"location", value.HTTPResponse{Location: "http://b", HasLocation: true, Status: 301}, value.String("http://b"), ""},
		{"location", value.HTTPResponse{Status: 200}, nil, ""},
		{"location", value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: http response"},
		{`split ","`, value.String("a,b,,c"), strs("a", "b", "", "c"), ""},
		{`split "{{sep}}"`, value.String("a,b"), strs("a", "b"), ""},
		{`split ""`, value.String("ab"), strs("", "a", "b", ""), ""},
		{`split ""`, value.String(""), strs("", ""), ""},
		{`split ","`, value.String(""), strs(""), ""},
		{`split ","`, value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
	})
}

func TestChainAndMissingInput(t *testing.T) {
	runCases(t, []filterCase{
		{`split "," nth 1 toInt`, value.String("1,22,3"), value.Int(22), ""},
		{`regex "x(y)?" count`, value.String("x"), nil, "Filter error: missing value to apply filter"},
		{`location toString`, value.HTTPResponse{Status: 200}, nil, "Filter error: missing value to apply filter"},
	})
	var re *runerr.Error
	_, err := Apply(parseFilters(t, "count"), value.Null{}, testEnv(), true)
	if !errors.As(err, &re) || !re.Assert || re.Span.Start.Col != 14 {
		t.Errorf("error = %#v", err)
	}
	if v, err := Apply(nil, value.Int(1), testEnv(), false); err != nil || v != value.Int(1) {
		t.Errorf("no filters = %v, %v", v, err)
	}
}

func TestNumberFilters(t *testing.T) {
	runCases(t, []filterCase{
		{"toInt", value.Int(3), value.Int(3), ""},
		{"toInt", value.Float(3.9), value.Int(3), ""},
		{"toInt", value.Float(-3.9), value.Int(-3), ""},
		{"toInt", value.Float(1e30), value.Int(math.MaxInt64), ""},
		{"toInt", value.Float(-1e30), value.Int(math.MinInt64), ""},
		{"toInt", value.Float(math.NaN()), value.Int(0), ""},
		{"toInt", value.String("-12"), value.Int(-12), ""},
		{"toInt", value.String("+12"), value.Int(12), ""},
		{"toInt", value.String("1.0"), nil, "Filter error: invalid filter input: string <1.0>"},
		{"toInt", value.String(" 1"), nil, "Filter error: invalid filter input: string < 1>"},
		{"toInt", value.String("+-1"), nil, "Filter error: invalid filter input: string <+-1>"},
		{"toInt", value.String("99999999999999999999"), nil, "Filter error: invalid filter input: string <99999999999999999999>"},
		{"toInt", value.BigInt("99999999999999999999"), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: float, integer or string"},
		{"toInt", value.Bool(true), nil, "Filter error: invalid filter input type\n   actual:   boolean\n   expected: float, integer or string"},
		{"toFloat", value.Int(3), value.Float(3), ""},
		{"toFloat", value.Float(3.5), value.Float(3.5), ""},
		{"toFloat", value.String("1."), value.Float(1), ""},
		{"toFloat", value.String(".5"), value.Float(0.5), ""},
		{"toFloat", value.String("-1.5e3"), value.Float(-1500), ""},
		{"toFloat", value.String("1e400"), value.Float(math.Inf(1)), ""},
		{"toFloat", value.String("-Infinity"), value.Float(math.Inf(-1)), ""},
		{"toFloat", value.String("inf"), value.Float(math.Inf(1)), ""},
		{"toFloat", value.String("abc"), nil, "Filter error: invalid filter input: string <abc>"},
		{"toFloat", value.String("0x10"), nil, "Filter error: invalid filter input: string <0x10>"},
		{"toFloat", value.String("1_000"), nil, "Filter error: invalid filter input: string <1_000>"},
		{"toFloat", value.String("."), nil, "Filter error: invalid filter input: string <.>"},
		{"toFloat", value.String("1e"), nil, "Filter error: invalid filter input: string <1e>"},
		{"toFloat", value.String(""), nil, "Filter error: invalid filter input: string <>"},
		{"toFloat", value.BigInt("1e+400"), nil, "Filter error: invalid filter input: integer <1e+400> is too big to be cast as a float"},
		{"toFloat", value.Null{}, nil, "Filter error: invalid filter input type\n   actual:   null\n   expected: float, integer or string"},
		{"toString", value.Int(1), value.String("1"), ""},
		{"toString", value.Float(1), value.String("1.0"), ""},
		{"toString", value.Bool(false), value.String("false"), ""},
		{"toString", value.Null{}, value.String("null"), ""},
		{"toString", value.Date(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)), value.String("2020-01-01T00:00:00.000000Z"), ""},
		{"toString", list(), nil, "Filter error: invalid filter input: list <[]> can not be converted to a string"},
	})
	if f, ok := parseFloat("NaN"); !ok || !math.IsNaN(f) {
		t.Error("NaN")
	}
}

func TestEncodingFilters(t *testing.T) {
	runCases(t, []filterCase{
		{"base64Decode", value.String("SGVsbG8="), value.Bytes("Hello"), ""},
		{"base64Decode", value.String("SGVsbG8"), nil, "Filter error: invalid filter input: string is not base64"},
		{"base64Decode", value.String("SGVs\nbG8="), nil, "Filter error: invalid filter input: string is not base64"},
		{"base64Decode", value.String("SGVsbG9="), nil, "Filter error: invalid filter input: string is not base64"},
		{"base64Decode", value.Bytes{}, nil, "Filter error: invalid filter input type\n   actual:   bytes\n   expected: base64 string"},
		{"base64Encode", value.Bytes("Hello"), value.String("SGVsbG8="), ""},
		{"base64Encode", value.String("x"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: bytes"},
		{"base64UrlSafeDecode", value.String("-_8"), value.Bytes{0xfb, 0xff}, ""},
		{"base64UrlSafeDecode", value.String("-_8="), nil, "Filter error: invalid filter input: base64 string contains padding"},
		{"base64UrlSafeDecode", value.String("+/8"), nil, "Filter error: invalid filter input: string is not base64"},
		{"base64UrlSafeDecode", value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
		{"base64UrlSafeEncode", value.Bytes{0xfb, 0xff}, value.String("-_8"), ""},
		{"base64UrlSafeEncode", value.Null{}, nil, "Filter error: invalid filter input type\n   actual:   null\n   expected: bytes"},
		{"toHex", value.Bytes{0xca, 0xfe}, value.String("cafe"), ""},
		{"toHex", value.String("x"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: bytes"},
		{`charsetDecode "iso-8859-1"`, value.Bytes("caf\xe9"), value.String("café"), ""},
		{`decode "utf-8"`, value.Bytes("café"), value.String("café"), ""},
		{`charsetDecode "utf-8"`, value.Bytes("caf\xe9"), nil, "Filter error: value can not be decoded with <utf-8> encoding"},
		{`charsetDecode "nope"`, value.Bytes("x"), nil, "Filter error: <nope> encoding is not supported"},
		{`charsetDecode "utf-8"`, value.String("x"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: bytes"},
		{"utf8Decode", value.Bytes("caf\xc3\xa9"), value.String("café"), ""},
		{"utf8Decode", value.Bytes("a\xff\xfeb\xe2\x82c\xf0\x9f\x98"), value.String("a��b�c�"), ""},
		{"utf8Decode", value.Bytes("\xed\xa0\x80\xe0\x80\xf4\x90"), value.String("�������"), ""},
		{"utf8Decode", value.String("x"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: bytes"},
		{"utf8Encode", value.String("é"), value.Bytes{0xc3, 0xa9}, ""},
		{"utf8Encode", value.Bytes{}, nil, "Filter error: invalid filter input type\n   actual:   bytes\n   expected: string"},
		{"urlEncode", value.String("a b/c?d=é~_.-"), value.String("a%20b/c%3Fd%3D%C3%A9~_.-"), ""},
		{"urlEncode", value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
		{"urlDecode", value.String("a%20b%2Fc+d%C3%A9%zz%4"), value.String("a b/c+dé%zz%4"), ""},
		{"urlDecode", value.String("%ff"), value.String("�"), ""},
		{"urlDecode", value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
		{"htmlEscape", value.String(`<a href="x">'&'</a>`), value.String("&lt;a href=&quot;x&quot;&gt;&#x27;&amp;&#x27;&lt;/a&gt;"), ""},
		{"htmlEscape", value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
		{"htmlUnescape", value.String("&lt;&gt;&amp;&quot;&#39;&#x27;&#X41;&eacute;&ampx&notit;&unknown;"), value.String("<>&\"''Aé&x¬it;&unknown;"), ""},
		{"htmlUnescape", value.String("&#0;&#13;&#128;&#x81;&#1;&#xD800;&#x110000;&#99999999999;&#xFFFF;&#65;"), value.String("�\r€\u0081���A"), ""},
		{"htmlUnescape", value.String("no refs"), value.String("no refs"), ""},
		{"htmlUnescape", value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
	})
}

func TestStringFilters(t *testing.T) {
	runCases(t, []filterCase{
		{`regex "(\\d+)-(\\d+)"`, value.String("id 12-34"), value.String("12"), ""},
		{`regex /(\d+)/`, value.String("x١٢"), value.String("١٢"), ""},
		{`regex /\d+/`, value.String("12"), nil, ""},
		{`regex /(x)/`, value.String("y"), nil, ""},
		{`regex "("`, value.String("y"), nil, "Invalid regex: regex expression is not valid"},
		{`regex /(x)/`, value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
		{`replace "a" "b"`, value.String("aXa"), value.String("bXb"), ""},
		{`replace "" "-"`, value.String("ab"), value.String("-a-b-"), ""},
		{`replace "a" "b"`, value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
		{`replaceRegex /(\w+)@(\w+)/ "$2 at ${1}$$"`, value.String("me@host"), value.String("host at me$"), ""},
		{`replaceRegex "[0-9]" "#"`, value.String("a1b22"), value.String("a#b##"), ""},
		{`replaceRegex "[" "#"`, value.String("a"), nil, "Invalid regex: regex expression is not valid"},
		{`replaceRegex "a" "b"`, value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
		{`urlQueryParam "q"`, value.String("https://a.org/p?x=1&q=a%20b+c&q=2#q=3"), value.String("a b c"), ""},
		{`urlQueryParam "z"`, value.String("http://a/?x"), nil, ""},
		{`urlQueryParam "x"`, value.String("http://a/?x"), value.String(""), ""},
		{`urlQueryParam "q"`, value.String("a.org?q=1"), nil, "Invalid URL: invalid URL <a.org?q=1> (Missing scheme <http://> or <https://>)"},
		{`urlQueryParam "q"`, value.String("ftp://a.org?q=1"), nil, "Invalid URL: invalid URL <ftp://a.org?q=1> (Only <http://> and <https://> schemes are supported)"},
		{`urlQueryParam "q"`, value.String("http://?q=1"), nil, "Invalid URL: invalid URL <http://?q=1> (empty host)"},
		{`urlQueryParam "q"`, value.String("http://a:b/"), nil, `Invalid URL: invalid URL <http://a:b/> (invalid port ":b" after host)`},
		{`urlQueryParam "q"`, value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
	})
}

func TestPathFilters(t *testing.T) {
	runCases(t, []filterCase{
		{`jsonpath "$.a"`, value.String(`{"a": 1}`), value.Int(1), ""},
		{`jsonpath "$.a[*]"`, value.String(`{"a": [1, "x"]}`), list(value.Int(1), value.String("x")), ""},
		{`jsonpath "$.a"`, value.String(`{"a": [1]}`), list(value.Int(1)), ""},
		{`jsonpath "$.b"`, value.String(`{"a": 1}`), nil, ""},
		{`jsonpath "$.a"`, value.String(`{`), nil, "Filter error: invalid filter input: value is not a valid JSON"},
		{`jsonpath "$$"`, value.String(`{}`), nil, "Invalid JSONPath: JSONPath expression '$$' is not valid"},
		{`jsonpath "$.a"`, value.Bytes("{}"), nil, "Filter error: invalid filter input type\n   actual:   bytes\n   expected: string"},
		{`xpath "string(//b)"`, value.String("<a><b>x</b></a>"), value.String("x"), ""},
		{`xpath "//b"`, value.String("<a><b>x</b><b/></a>"), value.Nodeset(2), ""},
		{`xpath "//["`, value.String("<a/>"), nil, "Invalid XPath expression: XPath expression is not valid"},
		{`xpath "//a"`, value.String(" "), nil, "Filter error: invalid filter input: value is not a valid XML"},
		{`xpath "//a"`, value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
	})
}

func TestDateFilters(t *testing.T) {
	d := func(s string) value.Date {
		tm, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return value.Date(tm)
	}
	runCases(t, []filterCase{
		{"daysAfterNow", d("2026-10-06T12:00:00Z"), value.Int(10), ""},
		{"daysAfterNow", d("2026-10-06T11:59:59Z"), value.Int(9), ""},
		{"daysAfterNow", d("2026-09-16T12:00:00Z"), value.Int(-10), ""},
		{"daysAfterNow", d("2026-09-16T12:00:00.5Z"), value.Int(-9), ""},
		{"daysBeforeNow", d("2026-09-16T12:00:00Z"), value.Int(10), ""},
		{"daysBeforeNow", d("2026-09-16T11:59:59.999Z"), value.Int(10), ""},
		{"daysBeforeNow", d("9999-01-01T00:00:00Z"), value.Int(-2911809), ""},
		{"daysAfterNow", value.String("x"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: date"},
		{`dateFormat "%Y-%m-%d"`, d("2026-09-26T01:02:03Z"), value.String("2026-09-26"), ""},
		{`format "%A"`, d("2026-09-26T01:02:03Z"), value.String("Saturday"), ""},
		{`dateFormat "%👻"`, d("2026-09-26T01:02:03Z"), nil, "Filter error: date format <%👻> is not supported"},
		{`dateFormat "%Y"`, value.String("x"), nil, "Filter error: invalid filter input type\n   actual:   string\n   expected: date"},
		{`toDate "%Y-%m-%dT%H:%M:%S%z"`, value.String("2026-09-26T01:02:03+0200"), d("2026-09-25T23:02:03Z"), ""},
		{`toDate "%Y-%m-%dT%H:%M:%S"`, value.String("2026-09-26T01:02:03"), d("2026-09-26T01:02:03Z"), ""},
		{`toDate "%Y-%m-%d"`, value.String("2026-09-26"), d("2026-09-26T00:00:00Z"), ""},
		{`toDate "%Y-%m-%d"`, value.String("26/09/2026"), nil, "Filter error: value <26/09/2026> could not be parsed with <%Y-%m-%d> format"},
		{`toDate "%Y"`, value.Int(1), nil, "Filter error: invalid filter input type\n   actual:   integer\n   expected: string"},
	})
	env := &template.Env{Vars: template.Vars{}}
	v, err := Apply(parseFilters(t, "daysBeforeNow"), value.Date(time.Now().Add(-49*time.Hour)), env, false)
	if err != nil || v != value.Int(2) {
		t.Errorf("default clock: %v, %v", v, err)
	}
}

func TestFirstGroup(t *testing.T) {
	re, _ := value.NewRegex(`a(b)?`)
	if g, ok := FirstGroup(re.Re, "xa"); ok || g != "" {
		t.Errorf("non-participating group = %q, %v", g, ok)
	}
	if !strings.Contains(re.Re.String(), "a") {
		t.Error("regex")
	}
}
