// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"strings"
	"time"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// testSecret is embedded in every fixture that should exercise redaction:
// in a URL, a header and a response body. redactTestSecret stands in for
// a run's secret registry.
const testSecret = "s3cr3t-token"

func redactTestSecret(s string) string {
	return strings.ReplaceAll(s, testSecret, "***")
}

func span(line, startCol, endCol int) syntax.Span {
	return syntax.Span{Start: syntax.Pos{Line: line, Col: startCol}, End: syntax.Pos{Line: line, Col: endCol}}
}

// runErr is err raised by the entry at line 1 of file, with content
// src.
func runErr(err *runerr.Error, file, src string) *engine.Error {
	return enginex.RunError(err, file, []byte(src), 1).(*engine.Error)
}

// valueOf is v as a capture value.
func valueOf(v value.Value) engine.Value {
	return enginex.Value(v).(engine.Value)
}

// fixedTime is used for every fixture's Timestamp so goldens are stable.
var fixedTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// successResult is a single passing GET, with a cookie, a header and a
// capture, and the run's secret embedded in its URL, a request header and
// the response body.
func successResult(file string) *engine.UnitResult {
	content := "GET http://example.com/hello?token=" + testSecret + "\n" +
		"HTTP 200\n" +
		"[Captures]\n" +
		"id: header \"X-Id\"\n"
	return &engine.UnitResult{
		File:      file,
		Source:    []byte(content),
		Success:   true,
		Duration:  42 * time.Millisecond,
		Timestamp: fixedTime,
		Cookies: []engine.Cookie{
			{Domain: "example.com", Path: "/", Name: "session", Value: testSecret, HTTPS: true},
		},
		Entries: []*engine.EntryResult{{
			Index: 1, Line: 1,
			Curl: "curl 'http://example.com/hello?token=" + testSecret + "'",
			Calls: []engine.Call{{
				Request: exchange.Request{
					Method: "GET",
					URL:    "http://example.com/hello?token=" + testSecret,
					Headers: exchange.Headers{
						{Name: "Authorization", Value: "Bearer " + testSecret},
					},
				},
				Response: &exchange.Response{
					Status: 200, Version: "HTTP/1.1",
					Headers: exchange.Headers{
						{Name: "X-Id", Value: "abc123"},
						{Name: "Set-Cookie", Value: "session=" + testSecret + "; Path=/; HttpOnly; Secure"},
					},
					Body: []byte("hello, " + testSecret),
				},
			}},
			Captures: []engine.Capture{{Name: "id", Value: valueOf(value.String("abc123"))}},
			Asserts:  []engine.Assert{{Line: 2}},
		}},
	}
}

// failureResult is a single GET whose body assert fails; the assert
// message (rendered from the real body) carries the run's secret.
func failureResult(file string) *engine.UnitResult {
	content := "GET http://example.com/hello\n" +
		"HTTP 200\n" +
		"[Asserts]\n" +
		"`Hello World`\n"
	err := runerr.New(span(4, 1, 14), runerr.AssertBodyValue, true)
	err.Actual = "hello, " + testSecret
	err.Expected = "Hello World"
	e := runErr(err, file, content)
	return &engine.UnitResult{
		File:      file,
		Source:    []byte(content),
		Success:   false,
		Duration:  7 * time.Millisecond,
		Timestamp: fixedTime,
		Entries: []*engine.EntryResult{{
			Index: 1, Line: 1,
			Curl: "curl 'http://example.com/hello'",
			Calls: []engine.Call{{
				Request:  exchange.Request{Method: "GET", URL: "http://example.com/hello"},
				Response: &exchange.Response{Status: 200, Version: "HTTP/1.1", Body: []byte("hello, " + testSecret)},
			}},
			Asserts: []engine.Assert{{Line: 4, Err: e}},
			Errors:  []*engine.Error{e},
		}},
	}
}

// errorResult is a single GET that never got a response (a runtime
// error), with the run's secret in the URL that failed to connect.
func errorResult(file string) *engine.UnitResult {
	content := "GET http://" + testSecret + ".invalid\n" + "HTTP 200\n"
	err := runerr.New(span(1, 5, 10), runerr.HTTP, false)
	err.Value, err.Reason = "HTTP connection", "Could not resolve host: "+testSecret+".invalid"
	return &engine.UnitResult{
		File:      file,
		Source:    []byte(content),
		Success:   false,
		Duration:  3 * time.Millisecond,
		Timestamp: fixedTime,
		Entries: []*engine.EntryResult{{
			Index: 1, Line: 1,
			Errors: []*engine.Error{runErr(err, file, content)},
		}},
	}
}

// parseErrorResult is a file that never parsed.
func parseErrorResult(file string) *engine.UnitResult {
	content := "GET http://example.com\nHTTP 200\nbase64,\n"
	return &engine.UnitResult{
		File:       file,
		Source:     []byte(content),
		Success:    false,
		Duration:   0,
		Timestamp:  fixedTime,
		ParseError: enginex.ParseError(&syntax.Error{Pos: syntax.Pos{Line: 3, Col: 8}, Kind: syntax.ErrExpecting, Arg: ";"}, file, []byte(content)).(*engine.Error),
	}
}

// interruptedResult is a 2-entry file stopped after its first entry (a
// Ctrl-C, or a sibling's fatal parse error under RunAll): its second
// entry never ran, so it has no error of its own to show for the stop —
// only Interrupted (and, with it, Success=false) says why it is not a
// pass.
func interruptedResult(file string) *engine.UnitResult {
	content := "GET http://example.com/first\nHTTP 200\n\nGET http://example.com/second\nHTTP 200\n"
	return &engine.UnitResult{
		File:        file,
		Source:      []byte(content),
		Success:     false,
		Interrupted: true,
		Duration:    5 * time.Millisecond,
		Timestamp:   fixedTime,
		Entries: []*engine.EntryResult{{
			Index: 1, Line: 1,
			Curl:    "curl 'http://example.com/first'",
			Asserts: []engine.Assert{{Line: 2}},
		}},
	}
}

// invalidXMLCharResult is a failing assert whose rendered message quotes
// a response body containing bytes illegal in XML 1.0: an ESC control
// character (U+001B, legal UTF-8, illegal in XML) and an invalid UTF-8
// byte sequence.
func invalidXMLCharResult(file string) *engine.UnitResult {
	content := "GET http://example.com/hello\nHTTP 200\n[Asserts]\n`Hello World`\n"
	actual := "hello, \x1b[31mred\x1b[0m \xff\xfe world"
	err := runerr.New(span(4, 1, 14), runerr.AssertBodyValue, true)
	err.Actual = actual
	err.Expected = "Hello World"
	e := runErr(err, file, content)
	return &engine.UnitResult{
		File:      file,
		Source:    []byte(content),
		Success:   false,
		Duration:  1 * time.Millisecond,
		Timestamp: fixedTime,
		Entries: []*engine.EntryResult{{
			Index: 1, Line: 1,
			Curl:    "curl 'http://example.com/hello'",
			Asserts: []engine.Assert{{Line: 4, Err: e}},
			Errors:  []*engine.Error{e},
		}},
	}
}
