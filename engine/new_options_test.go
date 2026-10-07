// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/runerr"
)

// streamLog records what a run writes to Stdout and the errors it logs,
// in order.
type streamLog struct{ items []string }

func (s *streamLog) Write(p []byte) (int, error) {
	s.items = append(s.items, "stdout:"+string(p))
	return len(p), nil
}

func (s *streamLog) on(ev Event) {
	if l, ok := ev.(Log); ok && (l.Level == LogError || l.Level == LogDebugError) {
		first, _, _ := strings.Cut(l.Text, "\n")
		s.items = append(s.items, "error:"+first)
	}
}

func runStreams(t *testing.T, src string, opt Options) (*UnitResult, *streamLog, string) {
	t.Helper()
	log := &streamLog{}
	opt.Stdout = log
	srv := server(t)
	opt.Variables = map[string]any{"base": srv.URL}
	opt.OnEvent = log.on
	dir := t.TempDir()
	res, err := NewRunner(opt).RunSource(t.Context(), filepath.Join(dir, "t.hurl"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return res, log, dir
}

// TestFailWithBody checks that the body of a failed entry is written
// before its errors, followed by a newline when it has none, to the
// entry's output when it has one; only the attempt that is not retried
// writes it, and a successful entry writes nothing.
func TestFailWithBody(t *testing.T) {
	const failing = "GET {{base}}/hello\nHTTP 200\n[Asserts]\nbody == \"x\"\n"
	tests := []struct {
		name string
		src  string
		opt  Options
		want []string
	}{
		{"flag", failing, Options{FailWithBody: true},
			[]string{"stdout:Hello World!\n", "error:Assert failure"}},
		{"body ending with a newline", "GET {{base}}/lines\nHTTP 200\n[Asserts]\nbody == \"x\"\n", Options{FailWithBody: true},
			[]string{"stdout:line 1\nline two\nline 3\n", "error:Assert failure"}},
		{"entry option", "GET {{base}}/hello\n[Options]\nfail-with-body: true\nHTTP 200\n[Asserts]\nbody == \"x\"\n", Options{},
			[]string{"stdout:Hello World!\n", "error:Assert failure"}},
		{"entry option off", "GET {{base}}/hello\n[Options]\nfail-with-body: false\nHTTP 200\n[Asserts]\nbody == \"x\"\n", Options{FailWithBody: true},
			[]string{"error:Assert failure"}},
		{"retried", failing, Options{FailWithBody: true, Retry: 2, RetryInterval: 1, Verbosity: Verbose},
			[]string{"error:Assert failure", "error:Assert failure", "stdout:Hello World!\n", "error:Assert failure"}},
		{"retried, not verbose", failing, Options{FailWithBody: true, Retry: 1, RetryInterval: 1},
			[]string{"stdout:Hello World!\n", "error:Assert failure"}},
		{"success", "GET {{base}}/hello\nHTTP 200\n", Options{FailWithBody: true}, nil},
		{"without the option", failing, Options{}, []string{"error:Assert failure"}},
		{"unauthorized output", "GET {{base}}/hello\n[Options]\noutput: ../out.txt\nHTTP 200\n[Asserts]\nbody == \"x\"\n", Options{FailWithBody: true},
			[]string{"error:Assert failure", "error:Unauthorized file access"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, log, _ := runStreams(t, tt.src, tt.opt)
			if strings.Join(log.items, "|") != strings.Join(tt.want, "|") {
				t.Errorf("got %q\nwant %q", log.items, tt.want)
			}
			if tt.name == "unauthorized output" {
				errs := res.Errors()
				if len(errs) != 2 || errs[1].run.Kind != runerr.UnauthorizedFileAccess {
					t.Errorf("errors %v", errs)
				}
			}
		})
	}

	// A retry cut short reports the attempt's error as final, with its
	// body.
	log := &streamLog{}
	srv := server(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opt := Options{FailWithBody: true, Retry: 3, RetryInterval: time.Hour, Stdout: log, Variables: map[string]any{"base": srv.URL}}
	opt.OnEvent = func(ev Event) {
		log.on(ev)
		if _, ok := ev.(EntryFinished); ok {
			cancel()
		}
	}
	if _, err := NewRunner(opt).RunSource(ctx, filepath.Join(t.TempDir(), "t.hurl"), []byte(failing)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"stdout:Hello World!\n", "error:Assert failure"}; strings.Join(log.items, "|") != strings.Join(want, "|") {
		t.Errorf("interrupted retry: %q, want %q", log.items, want)
	}

	_, log, dir := runStreams(t, "GET {{base}}/hello\n[Options]\noutput: out.txt\nHTTP 200\n[Asserts]\nbody == \"x\"\n", Options{FailWithBody: true})
	if b, err := os.ReadFile(filepath.Join(dir, "out.txt")); err != nil || string(b) != "Hello World!" {
		t.Errorf("output file = %q, %v", b, err)
	}
	if len(log.items) != 1 || log.items[0] != "error:Assert failure" {
		t.Errorf("with an output file: %q", log.items)
	}
}

// TestNoJSONPathCoercion checks that without coercion a jsonpath query or
// filter gives the list of matches, from the option or an entry's.
func TestNoJSONPathCoercion(t *testing.T) {
	asserts := `HTTP 200
[Captures]
ids: jsonpath "$.id"
[Asserts]
jsonpath "$.id" count == 1
jsonpath "$.id" first == 42
jsonpath "$.missing" count == 0
jsonpath "$.missing" exists
body jsonpath "$.name" first == "Bob"
variable "ids" count == 1
`
	res, rec := run(t, "GET {{base}}/json\n"+asserts, Options{NoJSONPathCoercion: true})
	if errs := res.Errors(); len(errs) != 0 {
		t.Errorf("option: %v\n%s", errs, rec.text(LogError))
	}
	res, rec = run(t, "GET {{base}}/json\n[Options]\nno-jsonpath-coercion: true\n"+asserts, Options{})
	if errs := res.Errors(); len(errs) != 0 {
		t.Errorf("entry option: %v\n%s", errs, rec.text(LogError))
	}
	// The default coerces: one match is the value, none is no value.
	res, rec = run(t, "GET {{base}}/json\nHTTP 200\n[Asserts]\njsonpath \"$.id\" == 42\njsonpath \"$.missing\" not exists\n", Options{})
	if errs := res.Errors(); len(errs) != 0 {
		t.Errorf("default: %v\n%s", errs, rec.text(LogError))
	}
}

// TestCurlCommandNewOptions checks the curl command of the options added
// in 8.1.0, and of a body holding a NUL byte, which is piped in.
func TestCurlCommandNewOptions(t *testing.T) {
	res, _ := run(t, "POST {{base}}/echo\nX-Drop: 1\nX-Keep: 2\n[Options]\nno-header: x-drop\nno-header: User-Agent\nhttp2-prior-knowledge: true\nhex,00ff;\n", Options{
		HTTP: HTTPOptions{Proxy: "http://p:1", ProxyHeaders: []string{"X-P: v"}},
	})
	want := "curl --header 'X-Keep: 2' --header 'Content-Type: application/octet-stream' --header 'x-drop:' --header 'User-Agent:' " +
		"--data-binary @- --http2-prior-knowledge --proxy 'http://p:1' --proxy-header 'X-P: v' '"
	curl := res.Entries[0].Curl
	if !strings.HasPrefix(curl, `printf '\x00\xff' | `+want) {
		t.Errorf("curl command\n%s\nwant prefix\nprintf '\\x00\\xff' | %s", curl, want)
	}
}

// TestDisplaySafe checks the escaping of headers shown in the logs.
func TestDisplaySafe(t *testing.T) {
	for in, want := range map[string]string{
		"X: plain\tvalue é":            "X: plain\tvalue é",
		"X: \x1b]52;c;aGk=\x07":        `X: \x1b]52;c;aGk=\x07`,
		"X: \u009b31m":                 `X: \u009b31m`,
		"X: \xff\xfe":                  `X: \xff\xfe`,
		"<script>alert(1)</script>: v": "<script>alert(1)</script>: v",
	} {
		if got := displaySafe(in); got != want {
			t.Errorf("displaySafe(%q) = %q, want %q", in, got, want)
		}
	}
}
