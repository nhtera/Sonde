// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/value"
)

// xssPayload is a single string carrying a script tag, an event-handler
// attribute break-out and a raw quote/angle mix, so one fixture proves
// escaping for the whole family of injections security-review §9 worries
// about (markup, attribute break-out, javascript: URLs) rather than just
// "<script>", which TestWriteHTML_NoScriptInjection already covers for
// the response body specifically.
const xssPayload = `"><script>alert(1)</script><img src=x onerror=alert(2)>`

// xssFile is a filename an attacker (a hostile request file, since
// request files are untrusted input per architecture.md §9) fully
// controls: it becomes both res.Label() (the report's "Filename" field)
// and the store's per-unit page name.
const xssFile = `xss` + xssPayload + `.hurl`

// TestWriteHTML_EscapesHostileFields is TestWriteHTML_NoScriptInjection's
// counterpart for every other attacker-influenced field the HTML report
// renders: the file name/label, the request URL, request and response
// headers, the rendered `curl` command, an assert's expected/actual text
// and a capture's value. None of these went through a hostile-string
// fixture before; only the response body (both tests above) and a secret
// value (TestWriteHTML_Redacts) did.
func TestWriteHTML_EscapesHostileFields(t *testing.T) {
	dir := t.TempDir()
	err := runerr.New(span(3, 1, 5), runerr.AssertBodyValue, true)
	err.Actual = "actual " + xssPayload
	err.Expected = "expected " + xssPayload
	src := "GET http://example.com/\nHTTP 200\n[Asserts]\n`x`\n"
	e := runErr(err, xssFile, src)
	res := &engine.UnitResult{
		File:      xssFile,
		Source:    []byte(src),
		Success:   false,
		Duration:  time.Millisecond,
		Timestamp: fixedTime,
		Entries: []*engine.EntryResult{{
			Index: 1, Line: 1,
			Curl: "curl 'http://example.com/" + xssPayload + "'",
			Calls: []engine.Call{{
				Request: exchange.Request{
					Method: "GET",
					URL:    "http://example.com/" + xssPayload,
					Headers: exchange.Headers{
						{Name: "X-Evil", Value: xssPayload},
						{Name: xssPayload, Value: "evil name"},
					},
				},
				Response: &exchange.Response{
					Status: 200, Version: "HTTP/1.1",
					Headers: exchange.Headers{
						{Name: "X-Resp-Evil", Value: xssPayload},
						{Name: "<script>alert('Hello')</script>", Value: "foo"},
					},
					Body: []byte("plain body"),
				},
			}},
			Captures: []engine.Capture{{Name: "cap", Value: valueOf(value.String(xssPayload))}},
			Asserts:  []engine.Assert{{Line: 3, Err: e}},
			Errors:   []*engine.Error{e},
		}},
	}

	if writeErr := WriteHTML(dir, []*engine.UnitResult{res}, identityRedact); writeErr != nil {
		t.Fatal(writeErr)
	}
	assertNoUnescapedScript(t, dir)

	// assertNoUnescapedScript only walks store/; the index page is the
	// one place res.Label() (the file name) is also rendered, as the
	// link text and the href built from sanitizeHTMLID, not the raw name.
	index, readErr := os.ReadFile(filepath.Join(dir, "index.html"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(index), "<script>") {
		t.Errorf("index.html has an unescaped <script> from the file name:\n%s", index)
	}
}

// identityRedact stands in for a run with no secrets: these fixtures
// test escaping, not redaction, so nothing here should be masked.
func identityRedact(s string) string { return s }
