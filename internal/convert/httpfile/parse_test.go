// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"strings"
	"testing"
	"time"
)

func TestParseDocumentBareURL(t *testing.T) {
	doc := parseDocument([]byte("https://example.com/ping\n"))
	if len(doc.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doc.requests))
	}
	r := doc.requests[0]
	if r.method != "GET" || r.url != "https://example.com/ping" {
		t.Errorf("method=%q url=%q", r.method, r.url)
	}
}

// TestParseDocumentCustomMethod checks M5: a method outside the old fixed
// list (GET/POST/PUT/...) is still recognized as a method, not folded into
// the URL as "GET PROPFIND url".
func TestParseDocumentCustomMethod(t *testing.T) {
	for _, tc := range []struct{ method, url string }{
		{"PROPFIND", "https://a.test/dav"},
		{"GRAPHQL", "https://a.test/graphql"},
		{"WEBSOCKET", "wss://a.test/socket"},
	} {
		doc := parseDocument([]byte(tc.method + " " + tc.url + "\n"))
		r := doc.requests[0]
		if r.method != tc.method || r.url != tc.url {
			t.Errorf("%s: method=%q url=%q", tc.method, r.method, r.url)
		}
	}
}

// TestParseDocumentBareURLNotMistakenForMethod checks that a bare URL
// (defaulting to GET) is never mistaken for "method + URL" just because it
// contains a space-free run before a placeholder or a path segment.
func TestParseDocumentBareURLNotMistakenForMethod(t *testing.T) {
	for _, url := range []string{
		"https://a.test/x/y",
		"{{base}}/widgets",
		"/relative/path",
	} {
		doc := parseDocument([]byte(url + "\n"))
		r := doc.requests[0]
		if r.method != "GET" || r.url != url {
			t.Errorf("%q: method=%q url=%q", url, r.method, r.url)
		}
	}
}

func TestParseDocumentMultipleRequestsNoLeadingSeparator(t *testing.T) {
	src := "GET https://a.test/one\n\n###\n\nGET https://a.test/two\n"
	doc := parseDocument([]byte(src))
	if len(doc.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(doc.requests))
	}
	if doc.requests[0].url != "https://a.test/one" || doc.requests[1].url != "https://a.test/two" {
		t.Errorf("urls = %q, %q", doc.requests[0].url, doc.requests[1].url)
	}
}

func TestParseDocumentSeparatorName(t *testing.T) {
	doc := parseDocument([]byte("### List things\nGET https://a.test/things\n"))
	if got := doc.requests[0].name; got != "List things" {
		t.Errorf("name = %q", got)
	}
}

func TestParseDocumentAtNameOverridesSeparator(t *testing.T) {
	src := "### List things\n# @name list-things\nGET https://a.test/things\n"
	doc := parseDocument([]byte(src))
	if got := doc.requests[0].name; got != "list-things" {
		t.Errorf("name = %q", got)
	}
}

func TestParseDocumentSlashSlashName(t *testing.T) {
	src := "// @name login\nPOST https://a.test/login\n"
	doc := parseDocument([]byte(src))
	if got := doc.requests[0].name; got != "login" {
		t.Errorf("name = %q", got)
	}
}

func TestParseDocumentHeadersAndBody(t *testing.T) {
	src := "POST https://a.test/x\nContent-Type: text/plain\nX-Foo: bar\n\nhello\nworld\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if len(r.headers) != 2 || r.headers[0].key != "Content-Type" || r.headers[1].value != "bar" {
		t.Fatalf("headers = %+v", r.headers)
	}
	if r.body == nil || r.body.kind != bodyText || r.body.text != "hello\nworld" {
		t.Fatalf("body = %+v", r.body)
	}
}

func TestParseDocumentURLVersionSuffix(t *testing.T) {
	doc := parseDocument([]byte("GET https://a.test/x HTTP/1.1\n"))
	if got := doc.requests[0].url; got != "https://a.test/x" {
		t.Errorf("url = %q", got)
	}
}

func TestParseDocumentURLContinuation(t *testing.T) {
	src := "GET https://a.test/x\n  ?a=1\n  &b=2\nAccept: */*\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if r.url != "https://a.test/x?a=1&b=2" {
		t.Errorf("url = %q", r.url)
	}
	if len(r.headers) != 1 {
		t.Errorf("headers = %+v", r.headers)
	}
}

func TestParseDocumentFileVars(t *testing.T) {
	src := "@host = https://a.test\n@n=1\nGET {{host}}/x\n"
	doc := parseDocument([]byte(src))
	if len(doc.fileVars) != 2 || doc.fileVars[0].name != "host" || doc.fileVars[1].rawValue != "1" {
		t.Fatalf("fileVars = %+v", doc.fileVars)
	}
	if len(doc.requests) != 1 {
		t.Fatalf("requests = %d", len(doc.requests))
	}
}

func TestParseDocumentPreRequestScriptInline(t *testing.T) {
	src := "< {%\n  console.log(1);\n%}\nGET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if r.preScript == nil || strings.TrimSpace(r.preScript.inline) != "console.log(1);" {
		t.Fatalf("preScript = %+v", r.preScript)
	}
}

func TestParseDocumentPreRequestScriptOneLine(t *testing.T) {
	src := "< {% console.log(1); %}\nGET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if r.preScript == nil || r.preScript.inline != "console.log(1);" {
		t.Fatalf("preScript = %+v", r.preScript)
	}
}

func TestParseDocumentPreRequestScriptFile(t *testing.T) {
	src := "< ./pre.js\nGET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if r.preScript == nil || r.preScript.external != "./pre.js" {
		t.Fatalf("preScript = %+v", r.preScript)
	}
}

func TestParseDocumentResponseHandlerAndOutput(t *testing.T) {
	src := "GET https://a.test/x\n\n> {%\n  client.log(1);\n%}\n>> ./out.json\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if r.responseScript == nil || strings.TrimSpace(r.responseScript.inline) != "client.log(1);" {
		t.Fatalf("responseScript = %+v", r.responseScript)
	}
	if r.outputFile == nil || r.outputFile.path != "./out.json" || r.outputFile.overwrite {
		t.Fatalf("outputFile = %+v", r.outputFile)
	}
}

func TestParseDocumentOutputOverwrite(t *testing.T) {
	src := "GET https://a.test/x\n\n>>! ./out.json\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if r.outputFile == nil || !r.outputFile.overwrite {
		t.Fatalf("outputFile = %+v", r.outputFile)
	}
}

func TestParseDocumentResponseHandlerFile(t *testing.T) {
	src := "GET https://a.test/x\n\n> ./script.js\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if r.responseScript == nil || r.responseScript.external != "./script.js" {
		t.Fatalf("responseScript = %+v", r.responseScript)
	}
}

func TestParseDocumentFileBodyRefs(t *testing.T) {
	for _, tc := range []struct {
		src  string
		kind bodyKind
		path string
	}{
		{"POST https://a.test/x\n\n< ./body.json\n", bodyFileRef, "./body.json"},
		{"POST https://a.test/x\n\n<@ ./body.json\n", bodyFileRefSubstituted, "./body.json"},
	} {
		doc := parseDocument([]byte(tc.src))
		r := doc.requests[0]
		if r.body == nil || r.body.kind != tc.kind || r.body.path != tc.path {
			t.Errorf("%q: body = %+v", tc.src, r.body)
		}
	}
}

func TestParseDocumentDirectives(t *testing.T) {
	// A bare number defaults to seconds; connection-timeout carries an
	// explicit unit.
	src := "# @no-redirect\n# @no-cookie-jar\n# @no-log\n# @timeout 100\n# @connection-timeout 200ms\nGET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if !r.hasTimeout || r.timeout != 100*time.Second {
		t.Errorf("timeout = %v %v", r.hasTimeout, r.timeout)
	}
	if !r.hasConnTimeout || r.connTimeout != 200*time.Millisecond {
		t.Errorf("connTimeout = %v %v", r.hasConnTimeout, r.connTimeout)
	}
	if len(r.comments) != 0 || len(r.unsupportedDirectives) != 0 {
		t.Errorf("no-op directives should leave no trace: comments=%v unsupported=%v", r.comments, r.unsupportedDirectives)
	}
}

func TestParseTimeoutValue(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"5", 5 * time.Second, true},
		{"500ms", 500 * time.Millisecond, true},
		{"5s", 5 * time.Second, true},
		{"2m", 2 * time.Minute, true},
		{" 5 ", 5 * time.Second, true},
		{"", 0, false},
		{"abc", 0, false},
		{"5x", 0, false},
	} {
		got, ok := parseTimeoutValue(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseTimeoutValue(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseDocumentUnsupportedDirective(t *testing.T) {
	src := "# @something-else foo\nGET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if len(r.unsupportedDirectives) != 1 || r.unsupportedDirectives[0] != "@something-else foo" {
		t.Errorf("unsupportedDirectives = %v", r.unsupportedDirectives)
	}
	if len(r.comments) != 1 {
		t.Errorf("comments = %v", r.comments)
	}
}

func TestParseDocumentGenericComment(t *testing.T) {
	src := "# just a note\n// another note\nGET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	r := doc.requests[0]
	if !contains(r.comments, "just a note") || !contains(r.comments, "another note") {
		t.Errorf("comments = %v", r.comments)
	}
}

func TestParseDocumentPromptDeclaresFileVar(t *testing.T) {
	src := "# @prompt username Enter your name\nGET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	if len(doc.fileVars) != 1 || doc.fileVars[0].name != "username" || !doc.fileVars[0].prompt {
		t.Fatalf("fileVars = %+v", doc.fileVars)
	}
}

func TestParseDocumentEmptySegmentNoRequest(t *testing.T) {
	doc := parseDocument([]byte("@host = x\n\n"))
	if len(doc.requests) != 0 {
		t.Errorf("requests = %d, want 0", len(doc.requests))
	}
}

func TestParseDocumentNeverPanics(t *testing.T) {
	for _, src := range []string{
		"", "#", "//", "@", "@=", "<", ">", "<@", ">>", ">>!",
		"< {%", "> {%", "###", "### \n### \n###", "GET", "GET ",
		"POST x\nfoo\n", "POST x\nfoo: \n\n<", "POST x\n\n<@",
		"POST x\nContent-Type: multipart/form-data\n\n--\n--",
		"POST x\nContent-Type: application/json\n\n{",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			parseDocument([]byte(src))
		}()
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestParseDocumentCRLF(t *testing.T) {
	src := "GET https://a.test/x\r\nAccept: */*\r\n"
	doc := parseDocument([]byte(src))
	if len(doc.requests) != 1 || len(doc.requests[0].headers) != 1 {
		t.Fatalf("requests = %+v", doc.requests)
	}
}

func TestParseDocumentBOM(t *testing.T) {
	src := string(rune(0xFEFF)) + "GET https://a.test/x\n"
	doc := parseDocument([]byte(src))
	if len(doc.requests) != 1 || !strings.HasPrefix(doc.requests[0].url, "https://") {
		t.Fatalf("requests = %+v", doc.requests)
	}
}
