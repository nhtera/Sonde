// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"bytes"
	"compress/gzip"
	"errors"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

// parseQuery parses `<query> exists` as an assert and returns its query.
func parseQuery(t *testing.T, q string) *syntax.Query {
	t.Helper()
	src := "GET http://a\nHTTP 200\n[Asserts]\n" + q + " exists\n"
	f, err := syntax.Parse("t.hurl", []byte(src), syntax.DialectHurl)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return f.Entries[0].Response.Sections[0].Asserts[0].Query
}

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testEnv() *template.Env {
	vars := template.Vars{}
	vars.Set("name", value.String("X-Custom"))
	vars.Set("count", value.Int(2))
	return &template.Env{Vars: vars}
}

type queryCase struct {
	query string
	want  value.Value // nil: no value
	err   string
}

func run(t *testing.T, ctx *Context, cases []queryCase) {
	t.Helper()
	for _, tt := range cases {
		got, err := ctx.Eval(parseQuery(t, tt.query))
		if tt.err != "" {
			if err == nil || err.Error() != tt.err {
				t.Errorf("%s: error %v, want %q", tt.query, err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tt.query, err)
			continue
		}
		if (got == nil) != (tt.want == nil) || got != nil && !value.Equal(got, tt.want) {
			t.Errorf("%s = %#v, want %#v", tt.query, got, tt.want)
		}
	}
}

func TestJSONResponse(t *testing.T) {
	body := `{"books": [{"title": "Dune", "price": 9.5}, {"title": "Emma", "price": 12}], "id": 123456789012345678901234567890}`
	resp := &exchange.Response{
		Version: "HTTP/1.1", Status: 200, URL: "http://a/books", IP: "127.0.0.1", Duration: 1500 * time.Microsecond,
		Headers: exchange.Headers{
			{Name: "Content-Type", Value: "application/json"}, {Name: "Content-Encoding", Value: "gzip"},
			{Name: "X-Custom", Value: "one"}, {Name: "x-custom", Value: "two"}, {Name: "X-Single", Value: "s"},
		},
		Body: gzipped(t, body),
	}
	ctx := NewContext([]*exchange.Response{resp}, testEnv())
	run(t, ctx, []queryCase{
		{"status", value.Int(200), ""},
		{"version", value.String("1.1"), ""},
		{"url", value.String("http://a/books"), ""},
		{"ip", value.String("127.0.0.1"), ""},
		{"duration", value.Int(1), ""},
		{`header "X-Single"`, value.String("s"), ""},
		{`header "{{name}}"`, value.List{value.String("one"), value.String("two")}, ""},
		{`header "Missing"`, nil, ""},
		{`header "{{nope}}"`, nil, "Undefined variable: you must set the variable nope"},
		{"body", value.String(body), ""},
		{"bytes", value.Bytes(body), ""},
		{"rawbytes", value.Bytes(resp.Body), ""},
		{`jsonpath "$.books[0].title"`, value.String("Dune"), ""},
		{`jsonpath "$.books[*].price"`, value.List{value.Float(9.5), value.Int(12)}, ""},
		{`jsonpath "$.id"`, value.BigInt("123456789012345678901234567890"), ""},
		{`jsonpath "$.books[{{count}}]"`, nil, ""},
		{`jsonpath "$.nothing"`, nil, ""},
		{`jsonpath "$["`, nil, "Invalid JSONPath: JSONPath expression '$[' is not valid"},
		{`regex "\"title\": \"(\\w+)\""`, value.String("Dune"), ""},
		{`regex /"id": (\d+)/`, value.String("123456789012345678901234567890"), ""},
		{`regex /nomatch(x)/`, nil, ""},
		{`regex "("`, nil, "Invalid regex: regex expression is not valid"},
		{`variable "count"`, value.Int(2), ""},
		{`variable "nope"`, nil, ""},
		{`xpath "//a"`, nil, "Invalid XML: HTTP response is not a valid XML"},
		{"certificate \"Subject\"", nil, ""},
		{"redirects", value.List{}, ""},
	})
	sha, _ := ctx.Eval(parseQuery(t, "sha256"))
	if b, ok := sha.(value.Bytes); !ok || len(b) != 32 {
		t.Errorf("sha256 = %v", sha)
	}
	md, _ := ctx.Eval(parseQuery(t, "md5"))
	if b, ok := md.(value.Bytes); !ok || len(b) != 16 {
		t.Errorf("md5 = %v", md)
	}
}

func TestHashes(t *testing.T) {
	resp := &exchange.Response{Status: 200, Body: []byte("Hello World!")}
	run(t, NewContext([]*exchange.Response{resp}, testEnv()), []queryCase{
		{"sha256", mustHex(t, "7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069"), ""},
		{"md5", mustHex(t, "ed076287532e86365e841e92bfc50d8c"), ""},
	})
}

func mustHex(t *testing.T, s string) value.Bytes {
	t.Helper()
	b := make([]byte, len(s)/2)
	for i := range b {
		var v byte
		for _, c := range []byte(s[2*i : 2*i+2]) {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= c - '0'
			default:
				v |= c - 'a' + 10
			}
		}
		b[i] = v
	}
	return value.Bytes(b)
}

func TestXPathQueries(t *testing.T) {
	html := &exchange.Response{
		Status: 200, Headers: exchange.Headers{{Name: "Content-Type", Value: "text/html; charset=utf-8"}},
		Body: []byte("<html><head><title>Café</title></head><body><p>a</p><p>b</body></html>"),
	}
	run(t, NewContext([]*exchange.Response{html}, testEnv()), []queryCase{
		{`xpath "string(//title)"`, value.String("Café"), ""},
		{`xpath "count(//p)"`, value.Float(2), ""},
		{`xpath "//p"`, value.Nodeset(2), ""},
		{`xpath "//["`, nil, "Invalid XPath expression: XPath expression is not valid"},
	})
	xml := &exchange.Response{
		Status: 200, Headers: exchange.Headers{{Name: "Content-Type", Value: "application/xml"}},
		Body: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>SVG</text></svg>`),
	}
	run(t, NewContext([]*exchange.Response{xml}, testEnv()), []queryCase{
		{`xpath "string(//_:svg/_:text)"`, value.String("SVG"), ""},
		{`jsonpath "$"`, nil, "Invalid JSON: HTTP response is not a valid JSON"},
		{`jsonpath "$.a"`, nil, "Invalid JSON: HTTP response is not a valid JSON"},
	})
}

func TestBodyErrors(t *testing.T) {
	bad := &exchange.Response{
		Status: 200, Headers: exchange.Headers{{Name: "Content-Type", Value: "text/plain; charset=utf-8"}, {Name: "Content-Encoding", Value: "gzip"}},
		Body: []byte("not gzip"),
	}
	ctx := NewContext([]*exchange.Response{bad}, testEnv())
	msg := "Decompression error: could not uncompress response with gzip"
	for _, q := range []string{"body", "bytes", "sha256", "md5", `jsonpath "$"`, `xpath "/"`, `regex "x"`} {
		_, err := ctx.Eval(parseQuery(t, q))
		var re *runerr.Error
		if !errors.As(err, &re) || err.Error() != msg || re.Assert || re.Span.Start.Col != 1 {
			t.Errorf("%s: %v", q, err)
		}
	}
	if v, err := ctx.Eval(parseQuery(t, "rawbytes")); err != nil || !value.Equal(v, value.Bytes("not gzip")) {
		t.Errorf("rawbytes = %v, %v", v, err)
	}
	latin := &exchange.Response{Headers: exchange.Headers{{Name: "Content-Type", Value: "text/plain; charset=nope"}}}
	if _, err := NewContext([]*exchange.Response{latin}, testEnv()).Eval(parseQuery(t, "body")); err == nil ||
		err.Error() != "Invalid charset: the charset 'nope' is not valid" {
		t.Errorf("charset: %v", err)
	}
}

func TestCookies(t *testing.T) {
	resp := &exchange.Response{Status: 200, Headers: exchange.Headers{
		{Name: "Set-Cookie", Value: "LSID=DQAAAKEaem_vYg; Path=/accounts; Expires=Wed, 13 Jan 2021 22:23:01 GMT; Secure; HttpOnly"},
		{Name: "Set-Cookie", Value: "HSID=AYQEVnDKrdst; Domain=.localhost; Expires=Wed, 13-Jan-2021 22:23:01 GMT; HttpOnly; Max-Age=60; SameSite=Lax"},
		{Name: "Set-Cookie", Value: "SSID=x; Expires=tomorrow"},
	}}
	jan13 := value.Date(time.Date(2021, 1, 13, 22, 23, 1, 0, time.UTC))
	run(t, NewContext([]*exchange.Response{resp}, testEnv()), []queryCase{
		{`cookie "LSID"`, value.String("DQAAAKEaem_vYg"), ""},
		{`cookie "LSID[Value]"`, value.String("DQAAAKEaem_vYg"), ""},
		{`cookie "LSID[Expires]"`, jan13, ""},
		{`cookie "HSID[Expires]"`, jan13, ""},
		{`cookie "LSID[Max-Age]"`, nil, ""},
		{`cookie "HSID[Max-Age]"`, value.Int(60), ""},
		{`cookie "LSID[Domain]"`, nil, ""},
		{`cookie "HSID[Domain]"`, value.String(".localhost"), ""},
		{`cookie "LSID[Path]"`, value.String("/accounts"), ""},
		{`cookie "LSID[Secure]"`, value.Unit{}, ""},
		{`cookie "HSID[Secure]"`, nil, ""},
		{`cookie "HSID[HttpOnly]"`, value.Unit{}, ""},
		{`cookie "HSID[SameSite]"`, value.String("Lax"), ""},
		{`cookie "LSID[SameSite]"`, nil, ""},
		{`cookie "SSID[Path]"`, nil, ""},
		{`cookie "NONE"`, nil, ""},
		{`cookie "SSID[Expires]"`, nil, "HTTP connection: could not parse Cookie Expires attribute value <tomorrow>"},
	})
}

func TestCertificateAndRedirects(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cert := &exchange.CertInfo{Subject: "CN=localhost", Issuer: "CN=ca", StartDate: start, SerialNumber: "01", Value: "-----BEGIN CERTIFICATE-----"}
	responses := []*exchange.Response{
		{Status: 301, URL: "http://a/1"},
		{Status: 302, URL: "http://a/2"},
		{Status: 200, URL: "http://a/3", Certificate: cert},
	}
	run(t, NewContext(responses, testEnv()), []queryCase{
		{`certificate "Subject"`, value.String("CN=localhost"), ""},
		{`certificate "Issuer"`, value.String("CN=ca"), ""},
		{`certificate "Start-Date"`, value.Date(start), ""},
		{`certificate "Expire-Date"`, nil, ""},
		{`certificate "Serial-Number"`, value.String("01"), ""},
		{`certificate "Subject-Alt-Name"`, nil, ""},
		{`certificate "Value"`, value.String("-----BEGIN CERTIFICATE-----"), ""},
		{"url", value.String("http://a/3"), ""},
		{"status", value.Int(200), ""},
	})
	v, err := NewContext(responses, testEnv()).Eval(parseQuery(t, "redirects"))
	want := "[Response(location=http://a/2, status=301),Response(location=http://a/3, status=302)]"
	if err != nil || value.Display(v) != want {
		t.Errorf("redirects = %s, %v", value.Display(v), err)
	}
}
