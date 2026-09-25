// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package value

import (
	"math"
	"regexp"
	"testing"
	"time"
)

func date(y int, mo time.Month, d, h, mi, s, ns int) Date {
	return Date(time.Date(y, mo, d, h, mi, s, ns, time.UTC))
}

func mustRegex(t *testing.T, p string) Regex {
	t.Helper()
	re, err := NewRegex(p)
	if err != nil {
		t.Fatal(err)
	}
	return re
}

func TestRepr(t *testing.T) {
	tests := []struct {
		v    Value
		want string
	}{
		{Bool(true), "boolean <true>"},
		{Bytes{1, 2, 3}, "bytes <010203>"},
		{date(2000, 1, 1, 12, 0, 0, 123_000_000), "date <2000-01-01 12:00:00.123 UTC>"},
		{List{}, "list <[]>"},
		{List{Int(1), String("a"), List{Bool(false)}}, "list <[1,a,[false]]>"},
		{Nodeset(5), "nodeset <Nodeset(size=5)>"},
		{Null{}, "null <null>"},
		{Int(1), "integer <1>"},
		{BigInt("123456789012345678901234567890"), "integer <123456789012345678901234567890>"},
		{Float(1.0), "float <1.0>"},
		{Float(-1.5), "float <-1.5>"},
		{Object{}, "object <Object()>"},
		{mustRegex(t, "[0-9]+"), "regex </[0-9]+/>"},
		{mustRegex(t, "a/b"), `regex </a\/b/>`},
		{String("Hello"), "string <Hello>"},
		{Unit{}, "unit"},
		{HTTPResponse{Location: "http://a/b", HasLocation: true, Status: 302}, "http response <Response(location=http://a/b, status=302)>"},
		{HTTPResponse{Status: 200}, "http response <Response(location=None, status=200)>"},
	}
	for _, tt := range tests {
		if got := Repr(tt.v); got != tt.want {
			t.Errorf("Repr(%#v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestExpected(t *testing.T) {
	tests := []struct {
		v    Value
		want string
	}{
		{Bool(false), "boolean <false>"},
		{Bytes{}, "0 byte"},
		{Bytes{1}, "1 byte"},
		{Bytes{1, 2}, "2 bytes"},
		{date(2000, 1, 1, 0, 0, 0, 0), "date <2000-01-01 00:00:00 UTC>"},
		{HTTPResponse{Status: 200}, "HTTP response <Response(location=None, status=200)>"},
		{List{Int(1)}, "list of size 1"},
		{Nodeset(2), "list of size 2"},
		{Null{}, "null"},
		{Int(3), "integer <3>"},
		{Float(3), "float <3.0>"},
		{BigInt("1e400"), "number <1e400>"},
		{Object{{Key: "a", Value: Null{}}}, "list of size 1"},
		{mustRegex(t, "x"), "regex </x/>"},
		{String("s"), "string <s>"},
		{Unit{}, "something"},
	}
	for _, tt := range tests {
		if got := Expected(tt.v); got != tt.want {
			t.Errorf("Expected(%#v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		v    Value
		want string
		ok   bool
	}{
		{Null{}, "null", true},
		{Bool(true), "true", true},
		{Int(-7), "-7", true},
		{BigInt("99999999999999999999"), "99999999999999999999", true},
		{Float(2), "2.0", true},
		{Float(0.1), "0.1", true},
		{String("x y"), "x y", true},
		{date(2000, 2, 1, 12, 0, 0, 123_456_789), "2000-02-01T12:00:00.123456Z", true},
		{date(2000, 1, 1, 12, 0, 0, 123_000_000), "2000-01-01T12:00:00.123000Z", true},
		{Bytes{1}, "", false},
		{List{}, "", false},
		{Object{}, "", false},
		{Nodeset(1), "", false},
		{Unit{}, "", false},
		{HTTPResponse{}, "", false},
	}
	for _, tt := range tests {
		got, ok := Render(tt.v)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Render(%#v) = %q, %v; want %q, %v", tt.v, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDisplayDate(t *testing.T) {
	tests := []struct {
		v    Date
		want string
	}{
		{date(2000, 1, 1, 12, 0, 0, 0), "2000-01-01 12:00:00 UTC"},
		{date(2000, 1, 1, 12, 0, 0, 120_000_000), "2000-01-01 12:00:00.120 UTC"},
		{date(2000, 1, 1, 12, 0, 0, 123_456_000), "2000-01-01 12:00:00.123456 UTC"},
		{date(2000, 1, 1, 12, 0, 0, 123_456_789), "2000-01-01 12:00:00.123456789 UTC"},
		{date(12345, 1, 1, 0, 0, 0, 0), "+12345-01-01 00:00:00 UTC"},
		{Date(time.Date(2000, 1, 1, 14, 0, 0, 0, time.FixedZone("x", 2*3600))), "2000-01-01 12:00:00 UTC"},
	}
	for _, tt := range tests {
		if got := Display(tt.v); got != tt.want {
			t.Errorf("Display = %q, want %q", got, tt.want)
		}
	}
	if got := Repr(nil); got != "none" {
		t.Errorf("Repr(nil) = %q", got)
	}
	if got := Display(nil); got != "none" {
		t.Errorf("Display(nil) = %q", got)
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		f    float64
		want string
	}{
		{1, "1.0"},
		{1.1, "1.1"},
		{-1.5, "-1.5"},
		{-2, "-2.0"},
		{0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{1e21, "1000000000000000000000.0"},
		{1e-7, "0.0000001"},
		{math.Nextafter(0.3, 1), "0.30000000000000004"},
		{math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"},
		{math.NaN(), "NaN"},
	}
	for _, tt := range tests {
		if got := FormatFloat(tt.f); got != tt.want {
			t.Errorf("FormatFloat(%v) = %q, want %q", tt.f, got, tt.want)
		}
	}
}

func TestNumberFromText(t *testing.T) {
	tests := []struct {
		s    string
		want Value
	}{
		{"1", Int(1)},
		{"-0", Int(0)},
		{"9223372036854775807", Int(math.MaxInt64)},
		{"9223372036854775808", BigInt("9223372036854775808")},
		{"-9223372036854775809", BigInt("-9223372036854775809")},
		{"1.0", Float(1)},
		{"1e2", Float(100)},
		{"1E2", Float(100)},
		{"1e400", BigInt("1e+400")},
		{"1E400", BigInt("1e+400")},
		{"1e+400", BigInt("1e+400")},
		{"-1.5E-999", Float(math.Copysign(0, -1))},
		{"-1.5e999", BigInt("-1.5e+999")},
	}
	for _, tt := range tests {
		got := NumberFromText(tt.s)
		if got != tt.want {
			t.Errorf("NumberFromText(%q) = %#v, want %#v", tt.s, got, tt.want)
		}
	}
}

func TestCompareNumbers(t *testing.T) {
	tests := []struct {
		a, b Value
		want Ordering
	}{
		{Int(-1), Int(0), Less},
		{Int(1), Int(1), Same},
		{Int(1), BigInt("1"), Same},
		{Int(1), Float(1), Same},
		{Int(1), Int(0), Greater},
		{Int(1), Float(0), Greater},
		{Int(1), Float(2), Less},
		{Int(1), BigInt("2"), Less},
		{Int(1), BigInt("2.0"), Less},
		{Int(math.MinInt64), Float(-math.MaxFloat64), Greater},
		{Int(math.MaxInt64), Float(math.MaxFloat64), Less},
		{Float(1), Float(1.000_000_000_000_000_100), Same},
		{Float(1), Float(1.000_000_000_000_001), Less},
		{Float(9_007_199_254_740_992), Int(9_007_199_254_740_993), Same},
		// Exact comparison where a big integer is involved.
		{BigInt("9"), BigInt("10"), Less},
		{BigInt("-1"), BigInt("-2"), Greater},
		{BigInt("1.000"), BigInt("1.0"), Same},
		{BigInt("-001.1000"), BigInt("-1.1"), Same},
		{BigInt("1e400"), Float(math.MaxFloat64), Greater},
		{BigInt("1e400"), Float(math.Inf(1)), Less},
		{Float(math.Inf(-1)), BigInt("-1e400"), Less},
		{BigInt("123456789012345678901234567890"), Int(math.MaxInt64), Greater},
		{BigInt("1e+999999"), Int(1), Greater},
		{BigInt("-1e+999999"), BigInt("-1e+999998"), Less},
		{BigInt("1e+99999999999999999999"), BigInt("1e+999999"), Greater},
		{BigInt("-1e+400"), Float(math.Inf(-1)), Greater},
		{BigInt("0e+400"), Int(0), Same},
		{BigInt("-0.0e+500"), Float(0), Same},
		{BigInt("100e+398"), BigInt("1e+400"), Same},
		{BigInt("9223372036854775808"), Float(9223372036854775808), Less}, // float reads 9223372036854776000
		{BigInt("-9223372036854775809"), Int(math.MinInt64), Less},
		{BigInt("12e+400"), BigInt("1.3e+401"), Less},
		{Float(math.NaN()), Int(1), Unordered},
		{Float(math.NaN()), BigInt("1"), Unordered},
		{Float(math.NaN()), Float(math.NaN()), Unordered},
	}
	for _, tt := range tests {
		got, err := Compare(tt.a, tt.b)
		if err != nil || got != tt.want {
			t.Errorf("Compare(%#v, %#v) = %v, %v; want %v", tt.a, tt.b, got, err, tt.want)
		}
		if eq := Equal(tt.a, tt.b); eq != (tt.want == Same) {
			t.Errorf("Equal(%#v, %#v) = %v", tt.a, tt.b, eq)
		}
	}
}

func TestCompare(t *testing.T) {
	d1, d2 := date(2000, 1, 1, 0, 0, 0, 0), date(2001, 1, 1, 0, 0, 0, 0)
	tests := []struct {
		a, b Value
		want Ordering
		err  bool
	}{
		{String("a"), String("b"), Less, false},
		{String("é"), String("z"), Greater, false},
		{String("x"), String("x"), Same, false},
		{d1, d2, Less, false},
		{d2, d1, Greater, false},
		{String("1"), Int(1), Unordered, true},
		{Bool(true), Bool(true), Unordered, true},
		{Null{}, Null{}, Unordered, true},
	}
	for _, tt := range tests {
		got, err := Compare(tt.a, tt.b)
		if got != tt.want || (err != nil) != tt.err {
			t.Errorf("Compare(%#v, %#v) = %v, %v", tt.a, tt.b, got, err)
		}
	}
}

func TestEqual(t *testing.T) {
	re := mustRegex(t, "a")
	tests := []struct {
		a, b Value
		want bool
	}{
		{Bool(true), Bool(false), false},
		{Bool(true), Bool(true), true},
		{Bool(true), String("true"), false},
		{Int(1), String("1"), false},
		{Null{}, Null{}, true},
		{Null{}, Unit{}, false},
		{Unit{}, Unit{}, true},
		{String("a"), String("a"), true},
		{Bytes{1}, Bytes{1}, true},
		{Bytes{1}, Bytes{2}, false},
		{Nodeset(1), Nodeset(1), true},
		{date(2000, 1, 1, 0, 0, 0, 0), date(2000, 1, 1, 0, 0, 0, 0), true},
		{List{Int(1), Float(2)}, List{Float(1), Int(2)}, true},
		{List{Int(1)}, List{Int(1), Int(2)}, false},
		{List{Int(1)}, List{Int(2)}, false},
		{Object{{"a", Int(1)}}, Object{{"a", Float(1)}}, true},
		{Object{{"a", Int(1)}}, Object{{"b", Int(1)}}, false},
		{Object{{"a", Int(1)}}, Object{{"a", Int(2)}}, false},
		{Object{{"a", Int(1)}}, Object{}, false},
		{re, re, false},
		{HTTPResponse{Status: 1}, HTTPResponse{Status: 1}, false},
	}
	for _, tt := range tests {
		if got := Equal(tt.a, tt.b); got != tt.want {
			t.Errorf("Equal(%#v, %#v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDecodeJSON(t *testing.T) {
	v, err := DecodeJSON(` {"b": [1, 2.5, 1E400, 123456789012345678901234567890, "x", true, null], "a": {"z": 1, "y": {}}, "b2": -0} `)
	if err != nil {
		t.Fatal(err)
	}
	want := Object{
		{"a", Object{{"y", Object{}}, {"z", Int(1)}}},
		{"b", List{Int(1), Float(2.5), BigInt("1e+400"), BigInt("123456789012345678901234567890"), String("x"), Bool(true), Null{}}},
		{"b2", Int(0)},
	}
	if !Equal(v, want) {
		t.Errorf("DecodeJSON = %#v", v)
	}
	if v, err := DecodeJSON(`{"a": 1, "a": 2}`); err != nil || !Equal(v, Object{{"a", Int(2)}}) {
		t.Errorf("duplicate key: %#v, %v", v, err)
	}
	for _, bad := range []string{``, `{`, `[1,]`, `{} {}`, `1 x`, `NaN`} {
		if _, err := DecodeJSON(bad); err == nil {
			t.Errorf("DecodeJSON(%q) succeeded", bad)
		}
	}
	if got := FromJSON(3.5); got != Float(3.5) {
		t.Errorf("FromJSON(float64) = %#v", got)
	}
	if got := FromJSON(struct{}{}); got != (Null{}) {
		t.Errorf("FromJSON(unknown) = %#v", got)
	}
}

func TestObjectGet(t *testing.T) {
	o := Object{{"a", Int(1)}, {"a", Int(2)}}
	if v, ok := o.Get("a"); !ok || v != Int(1) {
		t.Errorf("Get(a) = %v, %v", v, ok)
	}
	if _, ok := o.Get("b"); ok {
		t.Error("Get(b) found")
	}
}

func TestKindString(t *testing.T) {
	want := map[Kind]string{
		KindNull: "null", KindBool: "boolean", KindInteger: "integer", KindFloat: "float",
		KindString: "string", KindBytes: "bytes", KindList: "list", KindObject: "object",
		KindDate: "date", KindNodeset: "nodeset", KindUnit: "unit", KindRegex: "regex",
		KindHTTPResponse: "http response", Kind(99): "Kind(99)",
	}
	for k, s := range want {
		if k.String() != s {
			t.Errorf("%d.String() = %q, want %q", int(k), k.String(), s)
		}
	}
	if BigInt("1").Kind() != KindInteger {
		t.Error("BigInt kind")
	}
}

func TestTranslateRegex(t *testing.T) {
	tests := []struct {
		pattern     string
		match, miss []string
	}{
		{`^\d+$`, []string{"123", "١٢٣"}, []string{"12a"}},
		{`^\D+$`, []string{"abc"}, []string{"a1", "a٣"}},
		{`^\w+$`, []string{"café", "naïve_1", "日本語"}, []string{"a-b"}},
		{`^\W+$`, []string{"-+ "}, []string{"é"}},
		{`^\s+$`, []string{" \t\n\v 　"}, []string{"a"}},
		{`^\S+$`, []string{"abc"}, []string{"a "}},
		{`^[\d.]+$`, []string{"1.٢"}, []string{"a"}},
		{`^[\w-]+$`, []string{"é-x"}, []string{"a b"}},
		{`^[\s,]+$`, []string{" ,"}, []string{"a"}},
		{`^[^\d]+$`, []string{"abc"}, []string{"a٣"}},
		{`^\\d$`, []string{`\d`}, []string{"1"}},
		{`^\p{Greek}\x{41}\x41\pL$`, []string{"αAAb"}, []string{"aAAb"}},
		{`^[]\d]+$`, []string{"]1"}, []string{"a"}},
		{`^[^]\d]+$`, []string{"ab"}, []string{"]"}},
		{`^[[:alpha:]\d]+$`, []string{"ab1"}, []string{"-"}},
		{`^[\W]$`, []string{"-"}, []string{"a"}},
		{`^a\.b$`, []string{"a.b"}, []string{"axb"}},
	}
	for _, tt := range tests {
		re, err := NewRegex(tt.pattern)
		if err != nil {
			t.Errorf("%s: %v", tt.pattern, err)
			continue
		}
		if re.Source != tt.pattern {
			t.Errorf("Source = %q", re.Source)
		}
		for _, s := range tt.match {
			if !re.Re.MatchString(s) {
				t.Errorf("%s (%s) does not match %q", tt.pattern, re.Re, s)
			}
		}
		for _, s := range tt.miss {
			if re.Re.MatchString(s) {
				t.Errorf("%s (%s) matches %q", tt.pattern, re.Re, s)
			}
		}
	}
	for _, p := range []string{`a\`, `[a`, `\p{`, `[[:x`} {
		// Must not panic; validity follows the regexp package.
		_, errWant := regexp.Compile(p)
		if _, err := NewRegex(p); (err == nil) != (errWant == nil) {
			t.Errorf("NewRegex(%q) error = %v, regexp error = %v", p, err, errWant)
		}
	}
}
