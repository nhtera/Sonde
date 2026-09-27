// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/syntax"
)

// capturedRequest is the semantic dump this test compares: method, the
// final URL's path and query, headers (excluding ones a real transport
// adds or that are expected to differ only in incidental formatting) and
// the raw body.
type capturedRequest struct {
	method string
	target string // path + "?" + query
	header http.Header
	body   []byte
}

func captureServer(t *testing.T, dst *capturedRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		h := r.Header.Clone()
		h.Del("Content-Length") // transport-computed, expected to match trivially; excluded to avoid noise
		*dst = capturedRequest{method: r.Method, target: r.URL.RequestURI(), header: h, body: body}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runEntry(t *testing.T, src []byte) {
	t.Helper()
	runner := engine.NewRunner(engine.Options{})
	res, err := runner.RunSource(context.Background(), "<roundtrip>", src)
	if err != nil {
		t.Fatalf("RunSource: %v\n%s", err, src)
	}
	if res.ParseError != nil {
		t.Fatalf("parse error: %v\n%s", res.ParseError, src)
	}
	if !res.Success {
		var msgs []string
		for _, e := range res.Entries {
			for _, er := range e.Errors {
				msgs = append(msgs, er.Message())
			}
		}
		t.Fatalf("run failed: %v\n%s", msgs, src)
	}
}

// normalizeBoundary replaces a multipart request's randomly generated
// boundary (in its Content-Type header and body) with a fixed
// placeholder, so two independently sent multipart requests with the same
// parts compare equal.
func normalizeBoundary(r capturedRequest) capturedRequest {
	ct := r.header.Get("Content-Type")
	_, boundary, ok := strings.Cut(ct, "boundary=")
	if !ok || boundary == "" {
		return r
	}
	h := r.header.Clone()
	h.Set("Content-Type", strings.ReplaceAll(ct, boundary, "BOUNDARY"))
	body := []byte(strings.ReplaceAll(string(r.body), boundary, "BOUNDARY"))
	return capturedRequest{method: r.method, target: r.target, header: h, body: body}
}

func diffCaptured(t *testing.T, name string, want, got capturedRequest) {
	t.Helper()
	want, got = normalizeBoundary(want), normalizeBoundary(got)
	if want.method != got.method {
		t.Errorf("%s: method = %q, want %q", name, got.method, want.method)
	}
	if want.target != got.target {
		t.Errorf("%s: target = %q, want %q", name, got.target, want.target)
	}
	if string(want.body) != string(got.body) {
		t.Errorf("%s: body = %q, want %q", name, got.body, want.body)
	}
	var wantNames, gotNames []string
	for n := range want.header {
		wantNames = append(wantNames, n)
	}
	for n := range got.header {
		gotNames = append(gotNames, n)
	}
	sort.Strings(wantNames)
	sort.Strings(gotNames)
	if strings.Join(wantNames, ",") != strings.Join(gotNames, ",") {
		t.Errorf("%s: header names = %v, want %v", name, gotNames, wantNames)
		return
	}
	for _, n := range wantNames {
		wv, gv := want.header.Values(n), got.header.Values(n)
		if strings.Join(wv, "|") != strings.Join(gv, "|") {
			t.Errorf("%s: header %s = %v, want %v", name, n, gv, wv)
		}
	}
}

// TestRoundtrip exercises the full curl loop: an original request-file
// entry is (1) run for real against an echo server, then (2) exported to a curl
// command with engine.RenderCurl, (3) imported back with Import, and (4)
// that new entry is run for real against the same server; the two
// captured requests must be identical (method, path+query, headers,
// body). This is a curated set covering every request shape this
// importer claims to support (headers, basic auth, a raw body, a JSON
// body, a form-urlencoded body, multipart with a text field, and a query
// string) rather than the whole conformance corpus: matching every
// existing testdata/conformance/hurl/**/*.curl golden line to the entry
// that produced it (some represent retries or redirects of the same
// entry, not one line per entry) is significant extra bookkeeping this
// pass did not have time for; see the phase report for the follow-up.
func TestRoundtrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string // fmt-templated with the server URL as %s
	}{
		{"get-headers", "GET %s/greet\nX-Foo: bar\nAccept: text/plain\n"},
		{"post-raw-body", "POST %s/submit\nContent-Type: text/plain\n`hello world`\n"},
		{"post-json-body", `POST %s/submit
{
  "a": 1,
  "b": "two"
}
`},
		{"post-form", "POST %s/submit\n[Form]\nfoo: bar\nbaz: a b\n"},
		{"query", "GET %s/search\n[Query]\nq: hello world\nlimit: 10\n"},
		{"basic-auth", "GET %s/secure\n[BasicAuth]\nalice: s3cr3t\n"},
		{"multipart-text", "POST %s/upload\n[Multipart]\nfield1: value1\nfield2: value2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want, got capturedRequest
			srv := captureServer(t, &want)
			original := []byte(fmt.Sprintf(tc.body, srv.URL))
			if _, err := syntax.Parse("<roundtrip>", original, syntax.DialectHurl); err != nil {
				t.Fatalf("original does not parse: %v\n%s", err, original)
			}
			runEntry(t, original)

			entries, err := engine.NewRunner(engine.Options{}).RenderCurl(context.Background(), "roundtrip.hurl", original)
			if err != nil {
				t.Fatalf("RenderCurl: %v", err)
			}
			if len(entries) != 1 {
				t.Fatalf("RenderCurl returned %d entries, want 1", len(entries))
			}
			if entries[0].Err != nil {
				t.Fatalf("RenderCurl entry 1: %v", entries[0].Err)
			}

			res, err := Import([]byte(entries[0].Command), syntax.DialectHurl)
			if err != nil {
				t.Fatalf("Import(%q): %v", entries[0].Command, err)
			}
			if len(res.Skipped) > 0 {
				t.Fatalf("Import(%q) skipped: %v", entries[0].Command, res.Skipped)
			}

			srv2 := captureServer(t, &got)
			t.Cleanup(func() {}) // srv2's own Cleanup(srv2.Close) is already registered
			reimported := strings.Replace(string(syntax.Format(res.File)), srv.URL, srv2.URL, 1)
			runEntry(t, []byte(reimported))

			diffCaptured(t, tc.name, want, got)
		})
	}
}
