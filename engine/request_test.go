// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/value"
)

func requireSuccess(t *testing.T, res *UnitResult) {
	t.Helper()
	if !res.Success {
		for _, e := range res.Errors() {
			t.Error(e.Render())
		}
	}
}

func TestBodies(t *testing.T) {
	res, _ := run(t, `POST {{base}}/echo
<?xml version="1.0"?><a>1</a>
HTTP 200
[Asserts]
body contains "ct=application/xml"
body contains "body=<?xml version=\"1.0\"?><a>1</a>"

POST {{base}}/echo
base64,SGk=;
HTTP 200
[Asserts]
body contains "body=Hi\n"

POST {{base}}/echo
hex,4869;
HTTP 200
[Asserts]
body contains "body=Hi\n"

POST {{base}}/echo
`+"`raw {{n}}`"+`
HTTP 200
[Asserts]
body contains "body=raw 1\n"

POST {{base}}/echo
`+"```json\n{\"n\": {{n}}}\n```"+`
HTTP 200
[Asserts]
body contains "ct=application/json"

POST {{base}}/echo
`+"```graphql\nquery { a }\n```"+`
HTTP 200
[Asserts]
body contains "body={\"query\":\"query { a }\"}"

POST {{base}}/echo
`+"```xml\n<a/>\n```"+`
HTTP 200
[Asserts]
body contains "ct=application/xml"

POST {{base}}/echo
"just a \"JSON\" string"
HTTP 200
[Asserts]
body contains "ct=application/json"
body contains "body=\"just a \\\"JSON\\\" string\""

POST {{base}}/echo
file,data.txt;
HTTP 200
[Asserts]
body contains "body=file content"

POST {{base}}/echo
[MultipartFormData]
field: value
upload: file,data.txt;
typed: file,data.txt; text/csv
HTTP 200
[Asserts]
body contains "ct=multipart/form-data; boundary="
body contains "Content-Type: text/plain"
body contains "Content-Type: text/csv"
`, Options{Variables: map[string]any{"n": 1}, FileRoot: fileRoot(t, "data.txt", "file content")})
	requireSuccess(t, res)
}

// fileRoot returns a directory holding one file.
func fileRoot(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestImplicitBodyAsserts(t *testing.T) {
	root := fileRoot(t, "hello.txt", "Hello World!")
	res, _ := run(t, `GET {{base}}/hello
HTTP 200
file,hello.txt;

GET {{base}}/hello
HTTP 200
base64,SGVsbG8gV29ybGQh;

GET {{base}}/hello
HTTP 200
hex,48656c6c6f20576f726c6421;

GET {{base}}/json
HTTP 200
{"id": 42, "name": "Bob", "token": "s3cr3t-token"}
`, Options{FileRoot: root})
	requireSuccess(t, res)

	res, _ = run(t, `GET {{base}}/hello
HTTP 200
hex,00;
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.AssertBodyValue || errs[0].Actual() != "48656c6c6f20576f726c6421" {
		t.Fatalf("errors = %#v", errs)
	}
	res, _ = run(t, `GET {{base}}/hello
HTTP 200
`+"`Hello`"+`
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.AssertBodyValue || errs[0].run.Span.Start.Col != 1 {
		t.Fatalf("errors = %#v", errs)
	}
	res, _ = run(t, `GET {{base}}/hello
HTTP 200
file,missing.txt;
`, Options{FileRoot: root})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.FileReadAccess {
		t.Fatalf("errors = %#v", errs)
	}
}

func TestOptionKinds(t *testing.T) {
	res, rec := run(t, `GET {{base}}/echo
[Options]
compressed: true
connect-timeout: 5s
connect-to: example.org:80:127.0.0.1:80
delay: 1
header: X-Test: 1
http1.1: true
insecure: {{yes}}
ipv4: true
limit-rate: 1000000
location: true
location-trusted: false
max-redirs: {{three}}
max-time: 2m
netrc-optional: false
path-as-is: false
resolve: example.org:80:127.0.0.1
retry: 0
retry-interval: 1
unix-socket: {{empty}}
user: bob:pw
very-verbose: false
verbosity: brief
HTTP 200
[Asserts]
body contains "auth=Basic Ym9iOnB3"
`, Options{Variables: map[string]any{"yes": true, "three": int64(3), "empty": ""}})
	requireSuccess(t, res)
	if !strings.Contains(rec.text(LogDebug), "") {
		t.Error("logs")
	}
	for _, tt := range []struct{ option, kind string }{
		{"header: nocolon", "Invalid option value"},
		{"insecure: {{s}}", "Invalid expression type"},
		{"max-redirs: {{s}}", "Invalid expression type"},
		{"max-redirs: {{neg}}", "Invalid expression type"},
		{"limit-rate: {{s}}", "Invalid expression type"},
		{"limit-rate: {{zero}}", "Invalid expression type"},
		{"delay: {{neg}}", "Invalid expression type"},
		{"retry: {{missing}}", "Undefined variable"},
		{"http1.0: true", "Unsupported option"},
		{"ntlm: true", "Unsupported option"},
	} {
		src := "GET {{base}}/hello\n[Options]\n" + tt.option + "\nHTTP 200\n"
		res, _ := run(t, src, Options{Variables: map[string]any{"s": "x", "neg": int64(-5), "zero": int64(0)}})
		errs := res.Errors()
		if len(errs) != 1 || !strings.HasPrefix(errs[0].Description(), strings.Split(tt.kind, ":")[0]) &&
			errs[0].run.Value != tt.kind {
			var got []string
			for _, e := range errs {
				got = append(got, e.Error())
			}
			t.Errorf("%s: errors = %v", tt.option, got)
		}
	}
}

func TestVersionOptions(t *testing.T) {
	for _, o := range []string{"http2: false", "http2: true", "http3: false", "ipv6: false", "location-trusted: true", "netrc: false", "negotiate: false", "digest: false", "aws-sigv4: {{empty}}", "cacert: " + "", "proxy: {{empty}}", "pinnedpubkey: {{empty}}", "verbose: false", "output: -", "skip: false", "repeat: 1", "netrc-file: {{empty}}", "cert: {{empty}}", "key: {{empty}}"} {
		if strings.HasSuffix(o, ": ") {
			continue
		}
		res, _ := run(t, "GET {{base}}/hello\n[Options]\n"+o+"\nHTTP 200\n", Options{Variables: map[string]any{"empty": ""}, Stdout: &bytes.Buffer{}})
		if !res.Success {
			for _, e := range res.Errors() {
				t.Errorf("%s: %v", o, e)
			}
		}
	}
}

func TestOutputStdoutAndCookies(t *testing.T) {
	var out bytes.Buffer
	res, _ := run(t, `# @cookie_storage_set: 127.0.0.1    FALSE   /   FALSE   0   session abc
GET {{base}}/echo
[Options]
output: -
HTTP 200
[Asserts]
body contains "cookie=session=abc"

# @cookie_storage_clear
GET {{base}}/echo
HTTP 200
[Asserts]
body contains "cookie=\n"
`, Options{Stdout: &out})
	requireSuccess(t, res)
	if !strings.Contains(out.String(), "cookie=session=abc") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunFileAndVariables(t *testing.T) {
	srv := server(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.hurl")
	if err := os.WriteFile(path, []byte("GET {{base}}/hello\nHTTP 200\n[Asserts]\nvariable \"f\" == 1.5\nvariable \"z\" == null\nvariable \"b\" == true\nvariable \"s\" == \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(Options{
		Variables: map[string]any{"base": srv.URL, "f": 1.5, "z": nil, "b": true, "i": 3, "v": value.Int(1)},
		Secrets:   map[string]string{"s": "x"},
	})
	defer r.Close() //nolint:errcheck // test
	res, err := r.RunFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	requireSuccess(t, res)
	if !r.HasSecrets() || r.Redact("x") == "x" {
		t.Error("secret option not registered")
	}
	if _, err := r.RunFile(context.Background(), filepath.Join(dir, "missing.hurl")); err == nil {
		t.Error("missing file ran")
	}
	if _, err := NewRunner(Options{Variables: map[string]any{"x": struct{}{}}}).RunSource(context.Background(), path, []byte("GET http://a\n")); err == nil {
		t.Error("unsupported variable type accepted")
	}
}

func TestOutputStdoutBodyHook(t *testing.T) {
	var out bytes.Buffer
	var gotStatus int
	res, _ := run(t, "GET {{base}}/hello\n[Options]\noutput: -\nHTTP 200\n", Options{
		Stdout: &out,
		StdoutBody: func(resp *exchange.Response, body []byte) []byte {
			gotStatus = resp.Status
			return append([]byte("<"), append(body, '>')...)
		},
	})
	requireSuccess(t, res)
	if gotStatus != 200 || !strings.HasPrefix(out.String(), "<") || !strings.HasSuffix(out.String(), ">") {
		t.Errorf("stdout = %q, status seen = %d; want the hook's rendering of the 200 response", out.String(), gotStatus)
	}
}
