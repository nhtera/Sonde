// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/value"
)

// server is a test server with a few endpoints.
func server(t *testing.T) *httptest.Server {
	t.Helper()
	var flaky atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Add("X-Multi", "a")
		w.Header().Add("X-Multi", "b")
		_, _ = io.WriteString(w, "Hello World!")
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id": 42, "name": "Bob", "token": "s3cr3t-token"}`)
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%s %s\nct=%s\nauth=%s\ncookie=%s\nq=%s\nbody=%s\n", r.Method, r.URL.Path, //nolint:gosec // G705: plain-text echo in a test server
			r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.Header.Get("Cookie"),
			r.URL.RawQuery, body)
	})
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, _ *http.Request) {
		if flaky.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("/lines", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "line 1\nline two\nline 3\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type recorder struct {
	logs []Log
}

func (r *recorder) on(ev Event) {
	if l, ok := ev.(Log); ok {
		r.logs = append(r.logs, l)
	}
}

func (r *recorder) text(level LogLevel) string {
	var b strings.Builder
	for _, l := range r.logs {
		if l.Level == level {
			b.WriteString(l.Text + "\n")
		}
	}
	return b.String()
}

// run runs src (with {{base}} set to the server URL and {{host}} to its
// host and port) in a temp directory.
func run(t *testing.T, src string, opt Options) (*UnitResult, *recorder) {
	t.Helper()
	srv := server(t)
	if opt.Variables == nil {
		opt.Variables = map[string]any{}
	}
	opt.Variables["base"] = srv.URL
	opt.Variables["host"] = strings.TrimPrefix(srv.URL, "http://")
	rec := &recorder{}
	opt.OnEvent = rec.on
	if opt.RetryInterval == 0 {
		opt.RetryInterval = time.Millisecond
	}
	dir := t.TempDir()
	r := NewRunner(opt)
	res, err := r.RunSource(context.Background(), filepath.Join(dir, "test.hurl"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return res, rec
}

func TestRunSuccess(t *testing.T) {
	res, _ := run(t, `GET {{base}}/hello
HTTP 200
Content-Type: text/plain
X-Multi: b
[Captures]
greeting: body
[Asserts]
body == "Hello World!"
variable "greeting" startsWith "Hello"
`+"`Hello World!`"+`

GET {{base}}/json
HTTP *
[Asserts]
jsonpath "$.id" == 42
`, Options{})
	if !res.Success || len(res.Entries) != 2 {
		t.Fatalf("success=%v entries=%d errors=%v", res.Success, len(res.Entries), res.Errors())
	}
	e := res.Entries[0]
	if len(e.Captures) != 1 || e.Captures[0].Value.v != value.String("Hello World!") {
		t.Errorf("captures = %#v", e.Captures)
	}
	if len(e.Asserts) != 7 {
		t.Errorf("asserts = %d", len(e.Asserts))
	}
	if e.Calls[0].Response.Status != 200 || e.Line != 1 {
		t.Errorf("call = %#v line %d", e.Calls[0].Response, e.Line)
	}
}

func TestRunAssertFailures(t *testing.T) {
	res, rec := run(t, `GET {{base}}/hello
HTTP 201
`, Options{})
	errs := res.Errors()
	if res.Success || len(errs) != 1 || errs[0].run.Kind != runerr.AssertStatus || !errs[0].Assert() {
		t.Fatalf("errors = %v", errs)
	}
	if !strings.Contains(rec.text(LogError), "Assert status code\n  --> ") ||
		!strings.Contains(rec.text(LogError), "^^^ actual value is <200>") {
		t.Errorf("error log:\n%s", rec.text(LogError))
	}

	res, rec = run(t, `GET {{base}}/lines
HTTP 200
`+"```"+`
line 1
line 2
line 3
`+"```"+`
`, Options{})
	errs = res.Errors()
	if len(errs) != 1 || errs[0].run.Kind != runerr.AssertBodyDiff || errs[0].run.Span.Start.Line != 5 {
		t.Fatalf("diff errors = %#v", errs)
	}
	if !strings.Contains(rec.text(LogError), "   |   -line 2\n   |   +line two\n") {
		t.Errorf("diff log:\n%s", rec.text(LogError))
	}

	res, _ = run(t, `GET {{base}}/hello
HTTP 200
X-Multi: c
X-None: x
[Asserts]
header "X-Multi" count == 3
`, Options{})
	errs = res.Errors()
	if len(errs) != 3 || errs[0].run.Kind != runerr.AssertHeaderValue || errs[0].Actual() != `["a", "b"]` ||
		errs[1].run.Kind != runerr.QueryHeaderNotFound || errs[2].run.Kind != runerr.AssertFailure {
		t.Fatalf("header errors = %#v", errs)
	}
}

func TestRunRuntimeErrors(t *testing.T) {
	res, _ := run(t, `GET {{missing}}/x
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.UndefinedVariable || errs[0].Assert() {
		t.Fatalf("errors = %#v", errs)
	}
	res, _ = run(t, `GET ftp://x
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.InvalidURL {
		t.Fatalf("errors = %#v", errs)
	}
	res, _ = run(t, `GET http://127.0.0.1:1/
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.HTTP {
		t.Fatalf("errors = %#v", errs)
	}
	res, _ = run(t, `GET {{base}}/hello
HTTP 200
[Captures]
x: jsonpath "$.a"
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.QueryInvalidJSON {
		t.Fatalf("capture errors = %#v", errs)
	}
	res, _ = run(t, `GET {{base}}/json
HTTP 200
[Captures]
x: jsonpath "$.nope"
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.NoQueryResult {
		t.Fatalf("no result = %#v", errs)
	}
}

func TestRunRetryAndContinue(t *testing.T) {
	res, _ := run(t, `GET {{base}}/flaky
[Options]
retry: 5
HTTP 200
`, Options{})
	if !res.Success || len(res.Entries) != 3 {
		t.Fatalf("success=%v attempts=%d", res.Success, len(res.Entries))
	}
	res, _ = run(t, `GET {{base}}/hello
HTTP 404
GET {{base}}/json
HTTP 200
`, Options{ContinueOnError: true})
	if res.Success || len(res.Entries) != 2 {
		t.Fatalf("continue: success=%v entries=%d", res.Success, len(res.Entries))
	}
	res, _ = run(t, `GET {{base}}/hello
HTTP 404
GET {{base}}/json
HTTP 200
`, Options{})
	if len(res.Entries) != 1 {
		t.Fatalf("stop: entries=%d", len(res.Entries))
	}
}

func TestRunEntryOptions(t *testing.T) {
	res, _ := run(t, `GET {{base}}/hello
[Options]
variable: n=3
variable: name=Bob{{n}}
repeat: 2
HTTP 200

GET {{base}}/json
[Options]
skip: true
HTTP 500

GET {{base}}/echo
[Query]
who: {{name}}
HTTP 200
[Asserts]
body contains "q=who=Bob3"
`, Options{})
	if !res.Success || len(res.Entries) != 3 {
		t.Fatalf("success=%v entries=%d errors=%v", res.Success, len(res.Entries), res.Errors())
	}
	res, _ = run(t, `GET {{base}}/hello
[Options]
delay: {{bad}}
HTTP 200
`, Options{Variables: map[string]any{"bad": "x"}})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.ExpressionInvalidType {
		t.Fatalf("errors = %#v", errs)
	}
}

func TestRunRequestBuilding(t *testing.T) {
	res, _ := run(t, `POST {{base}}/echo?a=1
[BasicAuth]
bob: secret
[Query]
b: 2
[Cookies]
c1: v1
[Form]
f: x y
HTTP 200
[Asserts]
body contains "POST /echo"
body contains "ct=application/x-www-form-urlencoded"
body contains "auth=Basic Ym9iOnNlY3JldA=="
body contains "cookie=c1=v1"
body contains "q=a=1&b=2"
body contains "body=f=x%20y"

POST {{base}}/echo
{"name": "{{who}}", "n": {{n}}}
HTTP 200
[Asserts]
body contains "ct=application/json"
body contains "body={\"name\": \"a\\\"b\", \"n\": 1}"
`, Options{Variables: map[string]any{"who": `a"b`, "n": 1}})
	if !res.Success {
		for _, e := range res.Errors() {
			t.Error(e.Render())
		}
	}
}

func TestRunRedactCapture(t *testing.T) {
	res, rec := run(t, `GET {{base}}/json
HTTP 200
[Captures]
token: jsonpath "$.token" redact
[Asserts]
variable "token" == "wrong"
`, Options{})
	if res.Success {
		t.Fatal("expected failure")
	}
	log := rec.text(LogError)
	if strings.Contains(log, "s3cr3t-token") || !strings.Contains(log, "***") {
		t.Errorf("secret not redacted:\n%s", log)
	}
	res, _ = run(t, `GET {{base}}/json
HTTP 200
[Captures]
id: jsonpath "$.id" redact
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.UnsupportedSecretType {
		t.Fatalf("errors = %#v", errs)
	}
	res, _ = run(t, `GET {{base}}/json
HTTP 200
[Captures]
token: jsonpath "$.token" redact
`, Options{Verbosity: Verbose})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.PossibleLoggedSecret {
		t.Fatalf("verbose errors = %#v", errs)
	}
}

func TestRunOutputAndSandbox(t *testing.T) {
	srv := server(t)
	dir := t.TempDir()
	src := `GET {{base}}/hello
[Options]
output: out/hello.txt
HTTP 200

GET {{base}}/hello
[Options]
output: ../escape.txt
HTTP 200
`
	if err := os.Mkdir(filepath.Join(dir, "out"), 0o750); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(Options{Variables: map[string]any{"base": srv.URL}})
	res, err := r.RunSource(context.Background(), filepath.Join(dir, "t.hurl"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "out", "hello.txt")); err != nil || string(b) != "Hello World!" {
		t.Errorf("output = %q, %v", b, err)
	}
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.UnauthorizedFileAccess {
		t.Fatalf("errors = %#v", errs)
	}
	res, _ = run(t, `POST {{base}}/echo
file,../../etc/passwd;
HTTP 200
`, Options{})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.UnauthorizedFileAccess {
		t.Fatalf("body file errors = %#v", errs)
	}
}

func TestRunParseErrorAndEmpty(t *testing.T) {
	r := NewRunner(Options{})
	res, err := r.RunSource(context.Background(), "t.hurl", []byte("GET\n"))
	if err != nil || res.ParseError == nil {
		t.Fatalf("parse error = %v, %v", res, err)
	}
	res, err = r.RunSource(context.Background(), filepath.Join(t.TempDir(), "t.hurl"), []byte("# nothing\n"))
	if err != nil || !res.Success || len(res.Entries) != 0 {
		t.Fatalf("empty = %#v, %v", res, err)
	}
}

func TestRunCancel(t *testing.T) {
	srv := server(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := NewRunner(Options{Variables: map[string]any{"base": srv.URL}})
	res, err := r.RunSource(ctx, filepath.Join(t.TempDir(), "t.hurl"), []byte("GET {{base}}/hello\n"))
	if err != nil || len(res.Entries) != 0 {
		t.Fatalf("cancelled run = %#v, %v", res, err)
	}
}

func TestVerboseLogs(t *testing.T) {
	_, rec := run(t, `GET {{base}}/hello
[Options]
verbose: true
HTTP 200
[Captures]
g: body
`, Options{})
	debug := rec.text(LogDebug) + rec.text(LogDebugImportant)
	for _, want := range []string{"Executing entry 1", "Request:", "GET http", "verbose: true"} {
		if !strings.Contains(debug, want) {
			t.Errorf("debug log misses %q:\n%s", want, debug)
		}
	}
	if !strings.Contains(rec.text(LogResponseLine), "HTTP/1.1 200") {
		t.Errorf("response log:\n%s", rec.text(LogResponseLine))
	}
}

// A failed repeat iteration followed by a passing one of the same entry
// does not fail the file, as the reference decides (like a retry); a
// failure followed by another entry, or last, does.
func TestRunRepeatFailureCounts(t *testing.T) {
	res, _ := run(t, `GET {{base}}/flaky
[Options]
repeat: 3
HTTP 200
`, Options{ContinueOnError: true})
	if len(res.Entries) != 3 || !res.Success || len(res.Errors()) != 0 {
		t.Errorf("entries=%d success=%v errors=%d, want 3, true, 0", len(res.Entries), res.Success, len(res.Errors()))
	}
	res, _ = run(t, "GET {{base}}/flaky\nHTTP 200\nGET {{base}}/hello\nHTTP 200\n", Options{ContinueOnError: true})
	if res.Success {
		t.Error("a failed entry followed by another entry passes the file")
	}
}

// A secret password in [BasicAuth], the user option or the URL is also
// redacted in its Base64 form, whatever the length of the user name.
func TestRunBasicAuthSecretRedacted(t *testing.T) {
	const pw = "pw-secret-9c41" //nolint:gosec // G101: fake test value
	for _, src := range []string{
		"GET {{base}}/hello\n[BasicAuth]\nbob: {{pw}}\nHTTP 200\n",
		"GET {{base}}/hello\n[Options]\nuser: bob:{{pw}}\nHTTP 200\n",
		"GET http://bob:{{pw}}@{{host}}/hello\nHTTP 200\n",
	} {
		_, rec := run(t, src, Options{Secrets: map[string]string{"pw": pw}, Verbosity: Verbose})
		encoded := base64.StdEncoding.EncodeToString([]byte("bob:" + pw))
		log := rec.text(LogRequest)
		if !strings.Contains(log, "Authorization") {
			t.Fatalf("no Authorization header logged:\n%s", log)
		}
		if strings.Contains(log, encoded) || strings.Contains(log, pw) {
			t.Errorf("credentials not redacted in:\n%s", src)
		}
	}
}

// A redact capture is refused in verbose mode unless logs are buffered.
func TestRunRedactCaptureVerbose(t *testing.T) {
	const src = "GET {{base}}/json\nHTTP 200\n[Captures]\ntoken: jsonpath \"$.token\" redact\n"
	res, _ := run(t, src, Options{Verbosity: Verbose})
	if errs := res.Errors(); len(errs) != 1 || errs[0].run.Kind != runerr.PossibleLoggedSecret {
		t.Errorf("immediate logs: errors = %v", errs)
	}
	res, _ = run(t, src, Options{Verbosity: Verbose, BufferedLogs: true})
	if !res.Success {
		t.Errorf("buffered logs: errors = %v", res.Errors())
	}
}
