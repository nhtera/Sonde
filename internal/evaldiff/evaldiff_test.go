// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package evaldiff compares assert evaluation with recorded reference
// results. The reference results (testdata/eval/diff/*.json) are produced by
// running the reference implementation on the same files against the same
// server: `scripts/diff-hurl.sh` or
// `go test ./internal/evaldiff -reference=/path/to/binary`.
package evaldiff

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/filter"
	"github.com/nhtera/sonde/internal/predicate"
	"github.com/nhtera/sonde/internal/query"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

var reference = flag.String("reference", "", "reference binary: rewrite the expected results with its output")

const dir = "../../testdata/eval/diff"

// assertResult is the outcome of one explicit assert.
type assertResult struct {
	Line    int    `json:"line"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type fixture struct {
	Status  int        `json:"status"`
	Headers [][]string `json:"headers"`
	Body    string     `json:"body"`
	Base64  string     `json:"base64"`
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	var fixtures map[string]fixture
	b, err := os.ReadFile(filepath.Join(dir, "responses.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := fixtures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		for _, h := range f.Headers {
			w.Header().Add(h[0], h[1])
		}
		body := []byte(f.Body)
		if f.Base64 != "" {
			body, _ = base64.StdEncoding.DecodeString(f.Base64)
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.WriteHeader(max(f.Status, 200))
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// knownDifferences reads known-differences.txt: FILE:LINE → reason.
func knownDifferences(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "known-differences.txt"))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]string{}
	for line := range strings.SplitSeq(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		loc, reason, _ := strings.Cut(line, " ")
		known[loc] = reason
	}
	return known
}

func TestDiff(t *testing.T) {
	srv := newServer(t)
	known := knownDifferences(t)
	files, _ := filepath.Glob(filepath.Join(dir, "*.hurl"))
	if len(files) == 0 {
		t.Fatal("no case files")
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			golden := strings.TrimSuffix(path, ".hurl") + ".json"
			if *reference != "" {
				writeReference(t, name, golden, srv.URL)
			}
			var want []assertResult
			b, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &want); err != nil {
				t.Fatal(err)
			}
			got := evaluate(t, path, srv.URL)
			if len(got) != len(want) {
				t.Fatalf("%d asserts evaluated, %d expected", len(got), len(want))
			}
			for i := range want {
				loc := fmt.Sprintf("%s:%d", name, want[i].Line)
				if reason, ok := known[loc]; ok {
					if got[i] == want[i] {
						t.Errorf("%s no longer differs (%s): remove it from known-differences.txt", loc, reason)
					}
					continue
				}
				if got[i] != want[i] {
					t.Errorf("line %d:\ngot:  %v\n%s\nwant: %v\n%s", want[i].Line, got[i].Success, got[i].Message, want[i].Success, want[i].Message)
				}
			}
		})
	}
}

func variables(base string) []string {
	return []string{"base=" + base, "one=1", "onef=1.0"}
}

// writeReference runs the reference binary and records its explicit
// assert results.
func writeReference(t *testing.T, name, golden, base string) {
	t.Helper()
	args := []string{"--json", "--no-color", "--continue-on-error"}
	for _, v := range variables(base) {
		args = append(args, "--variable", v)
	}
	cmd := exec.Command(*reference, append(args, name)...) //nolint:gosec // G204: test-only, binary given by the developer
	cmd.Dir = dir
	// Output goes through a file: the reference binary can hang on exit
	// when writing to a pipe.
	outFile := filepath.Join(t.TempDir(), "out.json")
	f, err := os.Create(outFile)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = f
	_ = cmd.Run() // failing asserts exit non-zero
	_ = f.Close()
	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Entries []struct {
			Asserts []assertResult `json:"asserts"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("reference output: %v\n%s", err, out)
	}
	src, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	explicit := explicitLines(t, name, src)
	results := []assertResult{}
	for _, e := range report.Entries {
		for _, a := range e.Asserts {
			if explicit[a.Line] {
				results = append(results, a)
			}
		}
	}
	b, _ := json.MarshalIndent(results, "", " ")
	if err := os.WriteFile(golden, append(b, '\n'), 0o644); err != nil { //nolint:gosec // G306: committed fixture
		t.Fatal(err)
	}
}

// explicitLines returns the lines of the explicit asserts of a file.
func explicitLines(t *testing.T, name string, src []byte) map[int]bool {
	t.Helper()
	f, err := syntax.Parse(name, src, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	lines := map[int]bool{}
	for _, e := range f.Entries {
		for _, a := range asserts(e) {
			lines[a.Predicate.Func.Span.Start.Line] = true
		}
	}
	return lines
}

func asserts(e *syntax.Entry) []*syntax.Assert {
	if e.Response == nil {
		return nil
	}
	var as []*syntax.Assert
	for _, s := range e.Response.Sections {
		as = append(as, s.Asserts...)
	}
	return as
}

// evaluate runs every entry of a file and evaluates its explicit asserts.
func evaluate(t *testing.T, path, base string) []assertResult {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	f, err := syntax.Parse(name, src, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	vars := template.Vars{}
	for _, kv := range variables(base) {
		k, v, _ := strings.Cut(kv, "=")
		switch v {
		case "1":
			vars.Set(k, value.Int(1))
		case "1.0":
			vars.Set(k, value.Float(1))
		default:
			vars.Set(k, value.String(v))
		}
	}
	env := &template.Env{Vars: vars}
	client := &http.Client{
		Transport:     &http.Transport{DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	results := []assertResult{}
	for _, e := range f.Entries {
		url, err := env.Render(e.Request.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp := fetch(t, client, url)
		ctx := query.NewContext([]*exchange.Response{resp}, env)
		for _, a := range asserts(e) {
			r := assertResult{Line: a.Predicate.Func.Span.Start.Line, Success: true}
			if err := evalAssert(ctx, a); err != nil {
				re, ok := err.(*runerr.Error)
				if !ok {
					t.Fatalf("line %d: %v", r.Line, err)
				}
				r.Success = false
				r.Message = re.Render(name, string(src), e.Request.Span.Start.Line)
			}
			results = append(results, r)
		}
	}
	return results
}

// evalAssert evaluates query, filters and predicate of an assert.
func evalAssert(ctx *query.Context, a *syntax.Assert) error {
	v, err := ctx.Eval(a.Query)
	if err != nil {
		return err
	}
	if len(a.Filters) > 0 {
		if v, err = filter.Apply(a.Filters, v, ctx.Env, true); err != nil {
			return err
		}
	}
	return predicate.Eval(a.Predicate, v, ctx.Env)
}

func fetch(t *testing.T, client *http.Client, url string) *exchange.Response {
	t.Helper()
	start := time.Now()
	resp, err := client.Get(url) //nolint:noctx // test against a local server
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	r := &exchange.Response{
		Version: resp.Proto, Status: resp.StatusCode, Body: body, URL: url,
		IP: "127.0.0.1", Duration: time.Since(start),
	}
	for name, values := range resp.Header {
		for _, v := range values {
			r.Headers = append(r.Headers, exchange.Header{Name: name, Value: v})
		}
	}
	return r
}
