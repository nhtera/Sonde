// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *File {
	t.Helper()
	f, err := Parse("t.hurl", []byte(src), DialectHurl)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if got := string(Print(f)); got != src {
		t.Fatalf("round trip:\n%q\nwant\n%q", got, src)
	}
	return f
}

// value decodes a template without placeholders.
func value(t *testing.T, tpl *Template) string {
	t.Helper()
	var b strings.Builder
	for _, e := range tpl.Elements {
		s, ok := e.(*TemplateString)
		if !ok {
			t.Fatalf("unexpected placeholder in %+v", tpl)
		}
		b.WriteString(s.Value)
	}
	return b.String()
}

func firstAssert(t *testing.T, src string) *Assert {
	t.Helper()
	f := mustParse(t, "GET http://a\nHTTP 200\n[Asserts]\n"+src+"\n")
	return f.Entries[0].Response.Sections[0].Asserts[0]
}

func TestParseValues(t *testing.T) {
	t.Run("escapes", func(t *testing.T) {
		a := firstAssert(t, `header "x" == "q\"b\\\u{41}\t\n\r\b\f\/\#"`)
		if got := value(t, a.Predicate.Func.Value.(*Template)); got != "q\"b\\A\t\n\r\b\f/#" {
			t.Errorf("decoded %q", got)
		}
	})
	t.Run("key string escapes and placeholders", func(t *testing.T) {
		f := mustParse(t, "GET http://a\nx\\:y{{ v }}z\\u{2764}\\#\\b\\f\\n\\r\\t\\\\\\/: 1\n")
		key := f.Entries[0].Request.Headers[0].Key
		if len(key.Elements) != 3 {
			t.Fatalf("elements %d", len(key.Elements))
		}
		if s := key.Elements[0].(*TemplateString); s.Value != "x:y" || s.Source != `x\:y` {
			t.Errorf("first element %+v", s)
		}
		if ph := key.Elements[1].(*Placeholder); ph.Expr.Name != "v" || ph.Expr.Kind != ExprVariable {
			t.Errorf("placeholder %+v", ph)
		}
	})
	t.Run("functions", func(t *testing.T) {
		f := mustParse(t, "GET http://a/{{newUuid}}/{{ newDate }}/{{getEnv}}\n")
		els := f.Entries[0].Request.URL.Elements
		if els[1].(*Placeholder).Expr.Kind != ExprFunction || els[3].(*Placeholder).Expr.Kind != ExprFunction {
			t.Error("newUuid/newDate are functions")
		}
		if els[5].(*Placeholder).Expr.Kind != ExprVariable {
			t.Error("getEnv is a plain variable name")
		}
	})
	t.Run("placeholder trailing text", func(t *testing.T) {
		f := mustParse(t, "GET http://a/{{x y}}/{{z}w}}\n")
		els := f.Entries[0].Request.URL.Elements
		if ph := els[1].(*Placeholder); ph.Expr.Name != "x" || ph.Trailing != "y" {
			t.Errorf("placeholder %+v", ph)
		}
		if ph := els[3].(*Placeholder); ph.Expr.Name != "z" || ph.Trailing != "}w" {
			t.Errorf("placeholder %+v", ph)
		}
	})
	t.Run("regex escapes and classes", func(t *testing.T) {
		for _, re := range []string{`/\p{L}+/`, `/^\p{Lu}\p{Ll}+$/`, `/\P{Greek}/`, `/\x{4F}/`, `/[[:alpha:]{]/`, `/[]{]/`, `/a{2,}/`} {
			firstAssert(t, "body matches "+re)
		}
	})
	t.Run("xml declarations", func(t *testing.T) {
		for _, decl := range []string{`<?xml version="1.0" encoding="ISO-8859-1"?>`, `<?xml version="1.1"?>`, `<?xml version='1.0' encoding="windows-1252"?>`} {
			mustParse(t, "POST http://a\n"+decl+"\n<a>é</a>\n")
		}
	})
	t.Run("single brace", func(t *testing.T) {
		f := mustParse(t, "GET http://a/{x}/{\n")
		if got := value(t, f.Entries[0].Request.URL); got != "http://a/{x}/{" {
			t.Errorf("url %q", got)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		b64 := firstAssert(t, "bytes == base64, SGVs\n bG8=;").Predicate.Func.Value.(*Base64)
		if string(b64.Value) != "Hello" {
			t.Errorf("base64 %q", b64.Value)
		}
		for src, want := range map[string]string{"SGk": "Hi", "SA": "H"} {
			if v := firstAssert(t, "bytes == base64,"+src+";").Predicate.Func.Value.(*Base64); string(v.Value) != want {
				t.Errorf("base64 %s = %q, want %q", src, v.Value, want)
			}
		}
		hex := firstAssert(t, "bytes startsWith hex, 48656C6c6f ;").Predicate.Func.Value.(*Hex)
		if string(hex.Value) != "Hello" {
			t.Errorf("hex %q", hex.Value)
		}
		if _, ok := firstAssert(t, "bytes == file, a\\ b.bin;").Predicate.Func.Value.(*FileRef); !ok {
			t.Error("file ref")
		}
	})
	t.Run("numbers", func(t *testing.T) {
		tests := map[string]NumberKind{"12": NumberInteger, "-3.25": NumberFloat, "99999999999999999999": NumberBigInteger}
		for src, kind := range tests {
			n := firstAssert(t, "jsonpath \"$\" == "+src).Predicate.Func.Value.(*Number)
			if n.Kind != kind || n.Source != src {
				t.Errorf("%s: %+v", src, n)
			}
		}
	})
	t.Run("predicates", func(t *testing.T) {
		for _, src := range []string{
			`status != 200`, `status >= 200`, `status > {{n}}`, `status <= 2.5`, `status < "3"`,
			`body startsWith "a"`, `body endsWith base64,YQ==;`, `body contains "a"`,
			`jsonpath "$" includes null`, `body matches /a\/b/`, `body matches "a"`,
			`jsonpath "$" == true`, `jsonpath "$" == false`, "body == `x`",
			"body == ```\nmulti\n```", `jsonpath "$" not exists`, `jsonpath "$" isInteger`,
			`jsonpath "$" isFloat`, `jsonpath "$" isBoolean`, `jsonpath "$" isString`,
			`jsonpath "$" isCollection`, `jsonpath "$" isList`, `jsonpath "$" isObject`,
			`jsonpath "$" isDate`, `jsonpath "$" isIsoDate`, `jsonpath "$" isEmpty`,
			`jsonpath "$" isNumber`, `jsonpath "$" isIpv4`, `jsonpath "$" isIpv6`, `jsonpath "$" isUuid`,
		} {
			firstAssert(t, src)
		}
	})
	t.Run("queries and filters", func(t *testing.T) {
		src := `url == "u"
version == "2"
ip isIpv4
redirects count == 0
duration < 10
rawbytes count > 0
sha256 == hex,00;
md5 == hex,00;
xpath "//a" count == 1
regex "(a)" == "a"
regex /(a)/ == "a"
variable "v" == 1
certificate "Subject" == "s"
certificate "Issuer" exists
certificate "Start-Date" exists
certificate "Expire-Date" exists
certificate "Serial-Number" exists
certificate "Subject-Alt-Name" exists
certificate "Value" exists
cookie "a[Value]" == "1"
cookie "b[ max-age ]" == 1
cookie "c" exists
body base64Decode base64Encode base64UrlSafeDecode base64UrlSafeEncode count exists
body charsetDecode "utf-8" decode "utf-8" daysAfterNow daysBeforeNow first last exists
body format "%Y" dateFormat "%Y" htmlEscape htmlUnescape jsonpath "$" location exists
body nth 1 nth -1 nth {{i}} regex "a" regex /a/ replace "a" "b" replaceRegex /a/ "b" exists
body split "," toDate "%Y" toFloat toHex toInt toString urlDecode urlEncode exists
body urlQueryParam "q" utf8Decode utf8Encode xpath "//a" exists`
		f := mustParse(t, "GET http://a\nHTTP 200\n[Asserts]\n"+src+"\n")
		asserts := f.Entries[0].Response.Sections[0].Asserts
		if len(asserts) != strings.Count(src, "\n")+1 {
			t.Fatalf("parsed %d asserts", len(asserts))
		}
		cookie := asserts[20].Query.Arg.(*CookiePath)
		if value(t, cookie.Name) != "b" || cookie.Attribute.Name != "max-age" {
			t.Errorf("cookie path %+v", cookie)
		}
		cookie.Source = "" // printed from its parts
		if !bytes.Contains(Print(f), []byte(`cookie "b[ max-age ]"`)) {
			t.Error("cookie path printed from parts")
		}
	})
	t.Run("captures", func(t *testing.T) {
		f := mustParse(t, "GET http://a\nHTTP 200\n[Captures]\na: header \"x\" redact\nb: jsonpath \"$\" count\n")
		cs := f.Entries[0].Response.Sections[0].Captures
		if !cs[0].Redact || cs[1].Redact || len(cs[1].Filters) != 1 {
			t.Errorf("captures %+v %+v", cs[0], cs[1])
		}
	})
	t.Run("request sections", func(t *testing.T) {
		src := "POST http://a\n[QueryStringParams]\na: 1\n[FormParams]\nb: 2\n[BasicAuth]\nu: p\n" +
			"[Cookies]\nc: 3\n[MultipartFormData]\nf: file,a.txt;\ng: file,{{d}}/b\\;c.txt; text/plain\nh: v\n"
		f := mustParse(t, src)
		ss := f.Entries[0].Request.Sections
		kinds := []SectionKind{SectionQueryParams, SectionFormParams, SectionBasicAuth, SectionCookies, SectionMultipart}
		for i, k := range kinds {
			if ss[i].Kind != k {
				t.Errorf("section %d kind %d, want %d", i, ss[i].Kind, k)
			}
		}
		mp := ss[4].Multipart
		if fp := mp[1].(*FilenameParam); value(t, fp.Value.ContentType) != "text/plain" {
			t.Errorf("content type %+v", fp.Value)
		}
		if _, ok := mp[2].(*KeyValue); !ok {
			t.Error("plain multipart param")
		}
		mustParse(t, "GET http://a\n[BasicAuth]\n")
	})
	t.Run("options", func(t *testing.T) {
		src := `aws-sigv4: aws:amz:eu:s3
cacert: /c\ a.pem
cert: /c.pem:pa\:ss
key: k.pem
compressed: true
connect-to: a:1:b:2
connect-timeout: 5s
delay: {{d}}
digest: false
insecure: {{b}}
header: X: y
http1.0: false
http1.1: false
http2: false
http3: false
ipv4: false
ipv6: false
limit-rate: 100
location: true
location-trusted: true
max-redirs: -1
max-time: 3m
negotiate: false
netrc: false
netrc-file: n
netrc-optional: false
ntlm: false
output: out.bin
path-as-is: false
pinnedpubkey: sha256//x
proxy: p:1
repeat: {{n}}
resolve: a:1:b
retry: 2
retry-interval: 1h
skip: false
unix-socket: s
user: u:p
variable: a=null
variable: b=true
variable: c=1.5
variable: d="q"
variable: e=text value
verbose: true
verbosity: debug
very-verbose: false`
		f := mustParse(t, "GET http://a\n[Options]\n"+src+"\n")
		opts := f.Entries[0].Request.Sections[0].Options
		if len(opts) != strings.Count(src, "\n")+1 {
			t.Fatalf("parsed %d options", len(opts))
		}
		if v := value(t, opts[2].Value.(*Template)); v != `/c.pem:pa\:ss` {
			t.Errorf("cert keeps escaped colon: %q", v)
		}
		if d := opts[21].Value.(*Duration); d.Unit != "m" || d.Value.Int != 3 {
			t.Errorf("duration %+v", d)
		}
		if v := opts[len(opts)-2].Value.(*Identifier); v.Value != "debug" {
			t.Errorf("verbosity %+v", v)
		}
	})
	t.Run("bodies", func(t *testing.T) {
		for _, body := range []string{
			`{"a": [1, -2.5e+3, true, null, {{v}}, "s{{w}}", "\u00e9\ud83d\ude00\"\\\/\b\f\n\r\t"], "e": {}, "l": [ ]}`,
			"<?xml version=\"1.0\"?>\n<a b=\"1\"><c/>text</a>",
			"```json\n{\"a\": {{v}}}\n```", "```xml\n<a/>\n```", "```raw\n{{not a placeholder}}\n```",
			"```\nplain {{v}}\n```", "```graphql\nquery { a }\n```",
			"```graphql,\nquery Q($id: ID) { a(id: $id) }\nvariables {\"id\": \"{{id}}\"}\n```",
			"`one line {{v}}`", "base64,SGk=;", "hex,4869;", "file,body.json;", "null", "true", "42", "\"s\"", "[]",
		} {
			f := mustParse(t, "POST http://a\n"+body+"\n")
			if f.Entries[0].Request.Body == nil {
				t.Errorf("no body for %q", body)
			}
		}
		f := mustParse(t, "POST http://a\n```graphql\nq\nvariables {\"a\": 1}\n```\n")
		if ms := f.Entries[0].Request.Body.Value.(*MultilineString); ms.Variables == nil {
			t.Error("graphql variables")
		}
	})
	t.Run("versions and statuses", func(t *testing.T) {
		for _, line := range []string{"HTTP *", "HTTP/1.0 200", "HTTP/1.1 200", "HTTP/2 200", "HTTP/3 200", "HTTP\t200"} {
			mustParse(t, "GET http://a\n"+line+"\n")
		}
	})
	t.Run("bom and comments", func(t *testing.T) {
		f := mustParse(t, "\uFEFF# c\nGET http://a # tail\n\n# end")
		if !f.BOM || len(f.LineTerminators) != 2 {
			t.Errorf("bom=%v trailing=%d", f.BOM, len(f.LineTerminators))
		}
	})
	t.Run("empty", func(t *testing.T) {
		if f := mustParse(t, ""); len(f.Entries) != 0 {
			t.Error("entries in empty file")
		}
		mustParse(t, "  \n\t\n")
	})
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		src       string
		kind      ErrorKind
		line, col int
	}{
		{"get http://a\n", ErrMethod, 1, 1},
		{"GET\n", ErrSpace, 1, 4},
		{"GET http://a\nHTTP/4 200\n", ErrVersion, 2, 1},
		{"GET http://a\nHTTPS 200\n", ErrVersion, 2, 1},
		{"GET http://a\nHTTP abc\n", ErrStatus, 2, 6},
		{"GET http://a\nHTTP 99999999999999999999\n", ErrStatus, 2, 6},
		{"GET http://a\n[Query]\na: 1\n[Query]\n", ErrDuplicateSection, 4, 1},
		{"GET http://a\n[Foo]\n", ErrRequestSectionName, 2, 2},
		{"GET http://a\nHTTP 200\n[Query]\n", ErrResponseSectionName, 3, 2},
		{"GET http://a\n[Options]\nfoo: 1\n", ErrInvalidOption, 3, 1},
		{"GET http://a\n[Options]\nretry: x\n", ErrExpecting, 3, 8},
		{"GET http://a\n[Options]\nretry: -2\n", ErrExpecting, 3, 8},
		{"GET http://a\n[Options]\ndelay: 1y\n", ErrInvalidDurationUnit, 3, 9},
		{"GET http://a\n[Options]\nverbosity: loud\n", ErrExpecting, 3, 12},
		{"GET http://a\n[Options]\nlimit-rate: -1\n", ErrExpecting, 3, 13},
		{"GET http://a\n[Options]\nvariable: newDate=1\n", ErrVariable, 3, 11},
		{"GET http://a\n[Options]\nvariable: =1\n", ErrExpecting, 3, 11},
		{"GET http://a\n[Options]\ncacert: \n", ErrFilename, 3, 9},
		{"GET http://a\nx-\\q: 1\n", ErrMethod, 2, 1}, // bad key is recoverable: read as a next request
		{"GET http://a/\\q\n", ErrEscapeChar, 1, 15},
		{"GET http://a/\\u{zz}\n", ErrHexDigit, 1, 17},
		{"GET http://a/\\u{d800}\n", ErrUnicode, 1, 21},
		{"GET http://a/{{ }}\n", ErrTemplateVariable, 1, 17},
		{"GET http://a/{{x\n", ErrExpecting, 1, 17},
		{"GET http://a\nHTTP 200\n[Asserts]\nstatus foo\n", ErrPredicate, 4, 8},
		{"GET http://a\nHTTP 200\n[Asserts]\nstatus > true\n", ErrPredicateValue, 4, 10},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody startsWith 1\n", ErrPredicateValue, 4, 17},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody matches 1\n", ErrPredicateValue, 4, 14},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody == ~\n", ErrPredicateValue, 4, 9},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody matches /a(/\n", ErrRegexExpr, 4, 15},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody matches /a\n", ErrRegexExpr, 5, 1},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody == hex,abc;\n", ErrOddNumberOfHexDigits, 4, 16},
		{"GET http://a\nHTTP 200\n[Asserts]\ncookie \"a[foo]\" exists\n", ErrInvalidCookieAttribute, 4, 11},
		{"GET http://a\nHTTP 200\n[Asserts]\ncertificate \"Foo\" exists\n", ErrExpecting, 4, 14},
		{"GET http://a\nHTTP 200\n[Asserts]\nregex 1 == 1\n", ErrExpecting, 4, 7},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody nth x == 1\n", ErrExpecting, 4, 10},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody == 01\n", ErrExpecting, 4, 11},
		{"GET http://a\nHTTP 200\n[Asserts]\nbody == 1.\n", ErrExpecting, 4, 11},
		{"POST http://a\n{\"a\": 1,}\n", ErrJSONTrailingComma, 2, 8},
		{"POST http://a\n[1,]\n", ErrJSONTrailingComma, 2, 3},
		{"POST http://a\n[1,,2]\n", ErrJSONEmptyElement, 2, 4},
		{"POST http://a\n{\"a\": }\n", ErrJSONEmptyElement, 2, 6},
		{"POST http://a\n[ x]\n", ErrJSONExpectingElement, 2, 3},
		{"POST http://a\n[x]\n", ErrRequestSectionName, 2, 2}, // sections come before the body
		{"POST http://a\n[\"\\x\"]\n", ErrEscapeChar, 2, 4},
		{"POST http://a\n[\"\\ud800x\"]\n", ErrExpecting, 2, 9},
		{"POST http://a\n[\"\\ud800\\u0041\"]\n", ErrUnicode, 2, 11},
		{"POST http://a\n[1.]\n", ErrExpecting, 2, 4},
		{"POST http://a\n<a>\n", ErrXML, 2, 4}, // last character read
		{"POST http://a\n```foo\nx\n```\n", ErrMultilineLanguageHint, 2, 4},
		{"POST http://a\n```graphql\nq\nvariables [1]\n```\n", ErrGraphQLVariables, 4, 11},
		{"POST http://a\n```\nunterminated\n", ErrExpecting, 4, 1},
		{"GET http://a\nHTTP 200\nnot a header\n", ErrMethod, 3, 1},
		{"GET http://a\n\xff\n", ErrInvalidUTF8, 2, 1},
		{"GET http://a\n" + strings.Repeat("[", maxJSONDepth+1), ErrNestingTooDeep, 2, maxJSONDepth + 2},
	}
	for _, tt := range tests {
		_, err := Parse("t.hurl", []byte(tt.src), DialectHurl)
		if err == nil {
			t.Errorf("Parse(%q): no error", tt.src)
			continue
		}
		e := err.(*Error)
		if e.Kind != tt.kind || e.Pos.Line != tt.line || e.Pos.Col != tt.col {
			t.Errorf("Parse(%q) = %v (kind %d), want kind %d at %d:%d", tt.src, err, e.Kind, tt.kind, tt.line, tt.col)
		}
	}
}

func TestParseTooLarge(t *testing.T) {
	_, err := Parse("big.hurl", make([]byte, MaxFileSize+1), DialectHurl)
	if e, ok := err.(*Error); !ok || e.Kind != ErrFileTooLarge {
		t.Fatalf("got %v, want ErrFileTooLarge", err)
	}
}

func TestErrorTexts(t *testing.T) {
	for kind := ErrDuplicateSection; kind <= ErrNestingTooDeep; kind++ {
		e := &Error{Pos: Pos{Line: 1, Col: 1}, Kind: kind, Arg: "x"}
		if e.Description() == "" || e.Message() == "" || !strings.Contains(e.Error(), e.Message()) {
			t.Errorf("kind %d: empty text", kind)
		}
	}
	suggestions := map[ErrorKind]string{
		ErrMethod: "GTE", ErrInvalidOption: "retyr", ErrRequestSectionName: "Qeury",
		ErrResponseSectionName: "Assert", ErrInvalidDurationUnit: "sec",
	}
	for kind, arg := range suggestions {
		msg := (&Error{Kind: kind, Arg: arg}).Message()
		if !strings.Contains(msg, "Did you mean") && !strings.Contains(msg, "Valid values") {
			t.Errorf("kind %d: %q has no hint", kind, msg)
		}
	}
}

func TestRender(t *testing.T) {
	src := []byte("GET http://a\n\tx: \\q\n")
	_, err := Parse("t.hurl", src, DialectHurl)
	want := "Parsing escape character\n" +
		"  --> t.hurl:2:6\n" +
		"   |\n" +
		" 2 |     x: \\q\n" +
		"   |         ^ the escaping sequence is not valid\n" +
		"   |"
	if got := err.(*Error).Render("t.hurl", src); got != want {
		t.Errorf("Render:\n%s\nwant:\n%s", got, want)
	}
	if got := (&Error{Pos: Pos{Line: 9, Col: 1}}).Render("t", nil); !strings.Contains(got, " 9 | \n") {
		t.Errorf("Render past the end:\n%s", got)
	}
}

func TestDialectFor(t *testing.T) {
	if DialectFor("a.sonde") != DialectSonde || DialectFor("a.HURL") != DialectHurl || DialectFor("a") != DialectHurl {
		t.Error("DialectFor")
	}
}
