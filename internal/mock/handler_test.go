// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/openapi"
)

func loadSpec(t testing.TB, name string) *openapi.Spec {
	t.Helper()
	s, err := openapi.Load(context.Background(), "../../testdata/openapi/"+name, openapi.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// syncBuffer is a log shared by concurrent handlers.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

var paramRe = regexp.MustCompile(`\{[^}]+\}`)

// operationURLs returns a URL path of each operation of the spec, under
// its first server's base path, path parameters set to "1".
func operationURLs(t testing.TB, s *openapi.Spec) [][2]string {
	t.Helper()
	base := ""
	if servers := s.Servers(); len(servers) > 0 {
		u, err := url.Parse(servers[0])
		if err != nil {
			t.Fatal(err)
		}
		base = strings.TrimRight(u.Path, "/")
	}
	var out [][2]string
	for _, op := range s.Operations() {
		method, template, _ := strings.Cut(op, " ")
		out = append(out, [2]string{method, base + paramRe.ReplaceAllString(template, "1")})
	}
	return out
}

type result struct {
	status int
	header http.Header
	body   string
}

func do(t testing.TB, h http.Handler, method, target string, body io.Reader, header ...string) result {
	t.Helper()
	r := httptest.NewRequest(method, target, body)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Add(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	w.Header().Del("Date")
	return result{w.Code, w.Header(), w.Body.String()}
}

// TestFixturesServeValidResponses answers every operation of each fixture
// and checks the response with the --openapi validator: petstore bodies
// are valid, and a body the generator cannot make valid is logged.
func TestFixturesServeValidResponses(t *testing.T) {
	for _, name := range []string{"petstore-3.0.yaml", "petstore-3.1.yaml", "swagger-2.0.yaml", "edge-cases-3.1.yaml"} {
		t.Run(name, func(t *testing.T) {
			spec := loadSpec(t, name)
			var log syncBuffer
			h := Handler(spec.Mock(""), Options{Log: &log})
			v := spec.Validator(openapi.Options{Strict: true})
			for _, op := range operationURLs(t, spec) {
				r := do(t, h, op[0], op[1], nil)
				if r.status >= 400 {
					t.Errorf("%s %s: %d %s", op[0], op[1], r.status, r.body)
					continue
				}
				var hs exchange.Headers
				for k, vs := range r.header {
					for _, x := range vs {
						hs = append(hs, exchange.Header{Name: k, Value: x})
					}
				}
				vs := v.ValidateResponse(context.Background(), &exchange.Request{Method: op[0], URL: "https://api.example.com" + op[1]},
					&exchange.Response{Status: r.status, Headers: hs, Body: []byte(r.body)})
				switch {
				case strings.HasPrefix(name, "petstore") && len(vs) > 0:
					t.Errorf("%s %s: %d %s: %+v", op[0], op[1], r.status, r.body, vs)
				case len(vs) > 0 && !strings.Contains(log.String(), "warning: the body generated for"):
					t.Errorf("%s %s: invalid body %s without a warning: %+v", op[0], op[1], r.body, vs)
				}
			}
			if strings.HasPrefix(name, "petstore") && strings.Contains(log.String(), "warning") {
				t.Errorf("warnings:\n%s", log.String())
			}
		})
	}
}

// TestDeterministicUnderConcurrency sends the requests of every operation
// from 8 goroutines in different orders: each gets the same bytes.
func TestDeterministicUnderConcurrency(t *testing.T) {
	spec := loadSpec(t, "petstore-3.1.yaml")
	h := Handler(spec.Mock(""), Options{ValidateRequests: true, Log: io.Discard})
	ops := operationURLs(t, spec)
	want := map[[2]string]result{}
	for _, op := range ops {
		want[op] = do(t, h, op[0], op[1], nil, "Prefer", "code=200")
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 25 * len(ops) {
				op := ops[(i*(g+1)+g)%len(ops)]
				if got := do(t, h, op[0], op[1], nil, "Prefer", "code=200"); !reflect.DeepEqual(got, want[op]) {
					t.Errorf("%s %s: %+v, want %+v", op[0], op[1], got, want[op])
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestHandler(t *testing.T) {
	spec := loadSpec(t, "petstore-3.0.yaml")
	var log syncBuffer
	h := Handler(spec.Mock(""), Options{Log: &log})
	problem := func(status int, detail string) string {
		return fmt.Sprintf(`{"type":"about:blank","title":%q,"status":%d,"detail":%q}`, http.StatusText(status), status, detail)
	}
	for _, tc := range []struct {
		name, method, target string
		body                 io.Reader
		header               []string
		status               int
		ct, allow, want      string
	}{
		{name: "404", method: "GET", target: "/v1/nope", status: 404, ct: "application/problem+json",
			want: problem(404, "no operation of the OpenAPI spec matches GET /v1/nope")},
		{name: "405", method: "PATCH", target: "/v1/pets", status: 405, ct: "application/problem+json", allow: "GET, HEAD, POST",
			want: problem(405, "the OpenAPI spec has no PATCH /pets operation")},
		{name: "Prefer", method: "GET", target: "/v1/pets/1", header: []string{"Prefer", `respond-async, code="404"; x=y`},
			status: 404, ct: "application/json", want: `{"code":1,"message":"string"}`},
		{name: "Prefer bad", method: "GET", target: "/v1/pets/1", header: []string{"Prefer", "code=999"},
			status: 400, ct: "application/problem+json", want: problem(400, "Prefer code=999 is not a final HTTP status (200-599)")},
		{name: "HEAD", method: "HEAD", target: "/v1/pets/1", status: 200, ct: "application/json"},
		{name: "invalid body passes without --validate-requests", method: "POST", target: "/v1/pets", body: strings.NewReader("{"),
			header: []string{"Content-Type", "text/plain"}, status: 201, ct: "application/json", want: `{"id":1,"name":"string"}`},
		{name: "413", method: "POST", target: "/v1/pets", body: bytes.NewReader(make([]byte, maxBodyBytes+1)), status: 413,
			ct: "application/problem+json", want: problem(413, fmt.Sprintf("the request body is larger than %d bytes", maxBodyBytes))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := do(t, h, tc.method, tc.target, tc.body, tc.header...)
			if r.status != tc.status || r.header.Get("Content-Type") != tc.ct || r.header.Get("Allow") != tc.allow || r.body != tc.want {
				t.Errorf("got %d %q allow %q %s\nwant %d %q allow %q %s", r.status, r.header.Get("Content-Type"), r.header.Get("Allow"), r.body,
					tc.status, tc.ct, tc.allow, tc.want)
			}
			if r.header.Get("Access-Control-Allow-Origin") != "" {
				t.Error("CORS headers without --cors")
			}
		})
	}
	if r := do(t, h, "HEAD", "/v1/pets/1", nil); r.header.Get("Content-Length") != "24" {
		t.Errorf("HEAD Content-Length = %q", r.header.Get("Content-Length"))
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) != 8 || !regexp.MustCompile(`^GET /v1/nope 404 \S+$`).MatchString(lines[0]) {
		t.Errorf("log:\n%s", log.String())
	}
	if strings.Contains(log.String(), `"code"`) || strings.Contains(log.String(), "string") {
		t.Error("the log contains a body")
	}
}

func TestHandlerValidateRequests(t *testing.T) {
	h := Handler(loadSpec(t, "petstore-3.0.yaml").Mock(""), Options{ValidateRequests: true})
	r := do(t, h, "POST", "/v1/pets", strings.NewReader(`{"id":"x"}`), "Content-Type", "application/json")
	want := `{"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"the request does not match POST /pets",` +
		`"violations":["body /id: value must be an integer (type)","body /name: property \"name\" is missing (required)"]}`
	if r.status != 422 || r.body != want {
		t.Errorf("got %d %s\nwant %s", r.status, r.body, want)
	}
	if r := do(t, h, "POST", "/v1/pets", strings.NewReader(`{"id":1,"name":"rex"}`), "Content-Type", "application/json"); r.status != 201 {
		t.Errorf("valid request: %d %s", r.status, r.body)
	}
	if r := do(t, h, "POST", "/v1/pets", strings.NewReader("rex"), "Content-Type", "text/plain"); r.status != 415 {
		t.Errorf("undocumented content type: %d %s", r.status, r.body)
	}
}

func TestHandlerCORS(t *testing.T) {
	h := Handler(loadSpec(t, "petstore-3.0.yaml").Mock(""), Options{CORS: true, ValidateRequests: true})
	r := do(t, h, "OPTIONS", "/v1/pets", nil, "Origin", "http://localhost:3000",
		"Access-Control-Request-Method", "POST", "Access-Control-Request-Headers", "content-type, x-api-key")
	want := http.Header{
		"Access-Control-Allow-Origin":  {"http://localhost:3000"},
		"Access-Control-Allow-Methods": {"POST"},
		"Access-Control-Allow-Headers": {"content-type, x-api-key"},
		"Access-Control-Max-Age":       {"600"},
		"Vary":                         {"Origin"},
	}
	if r.status != 204 || !reflect.DeepEqual(r.header, want) || r.body != "" {
		t.Errorf("preflight: %d %v %q", r.status, r.header, r.body)
	}
	r = do(t, h, "GET", "/v1/pets", nil)
	if r.status != 200 || r.header.Get("Access-Control-Allow-Origin") != "*" || r.header.Get("Access-Control-Expose-Headers") != "*" {
		t.Errorf("simple request: %d %v", r.status, r.header)
	}
	// Without the preflight headers OPTIONS is an ordinary request.
	if r := do(t, h, "OPTIONS", "/v1/pets", nil); r.status != 405 {
		t.Errorf("plain OPTIONS: %d", r.status)
	}
}

func TestParsePrefer(t *testing.T) {
	for _, tc := range []struct {
		in            []string
		code, example string
	}{
		{nil, "", ""},
		{[]string{"code=404"}, "404", ""},
		{[]string{`Code = "500" , Example=bad-name`}, "500", "bad-name"},
		{[]string{"return=minimal", "example=a;foo=bar"}, "", "a"},
		{[]string{"code=200", "code=201"}, "200", ""},
		{[]string{`example="a,b; c", code=404`}, "404", "a,b; c"},
	} {
		code, example := parsePrefer(tc.in)
		if code != tc.code || example != tc.example {
			t.Errorf("%q = %q %q, want %q %q", tc.in, code, example, tc.code, tc.example)
		}
	}
}

func TestServeShutsDown(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	errc := make(chan error, 1)
	go func() { errc <- Serve(stop, ln, Handler(loadSpec(t, "petstore-3.0.yaml").Mock(""), Options{})) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/v1/pets") //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status %d", resp.StatusCode)
	}
	close(stop)
	if err := <-errc; err != nil {
		t.Errorf("Serve = %v", err)
	}
	if _, err := net.Dial("tcp", ln.Addr().String()); err == nil { //nolint:noctx // test
		t.Error("still listening after shutdown")
	}
}

// BenchmarkHandler measures one mocked request (report-only).
func BenchmarkHandler(b *testing.B) {
	spec := loadSpec(b, "petstore-3.0.yaml")
	for _, validate := range []bool{false, true} {
		b.Run(fmt.Sprintf("validate=%v", validate), func(b *testing.B) {
			h := Handler(spec.Mock(""), Options{ValidateRequests: validate})
			for b.Loop() {
				do(b, h, "POST", "/v1/pets", strings.NewReader(`{"id":1,"name":"rex"}`), "Content-Type", "application/json")
			}
		})
	}
}
