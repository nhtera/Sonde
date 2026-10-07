// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// TestIsTerminalNonTTYs: pipes, regular files, the null device and
// in-memory writers are never terminals. The terminal side is covered by
// the conformance pty lane (unix only).
func TestIsTerminalNonTTYs(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // test cleanup.
	defer w.Close() //nolint:errcheck // test cleanup.
	file, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close() //nolint:errcheck // test cleanup.
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close() //nolint:errcheck // test cleanup.

	for name, wr := range map[string]io.Writer{"pipe": w, "file": file, "devnull": null, "buffer": &bytes.Buffer{}} {
		if isTerminal(wr) {
			t.Errorf("%s: isTerminal = true, want false", name)
		}
	}
}

func TestIsBinary(t *testing.T) {
	at := func(i int) []byte {
		b := bytes.Repeat([]byte("a"), 3000)
		b[i] = 0
		return b
	}
	for _, tc := range []struct {
		name string
		body []byte
		want bool
	}{
		{"empty", nil, false},
		{"text", []byte("Hello World!"), false},
		{"latin-1", []byte("caf\xe9 \xe0 la cr\xe8me"), false},
		{"nul first", at(0), true},
		{"nul at 1999", at(1999), true},
		{"nul at 2000", at(2000), false},
		{"gzip", []byte("\x1f\x8b\x08\x00\xed\x0c\x84\x5f\x00\x03"), true},
	} {
		if got := isBinary(tc.body); got != tc.want {
			t.Errorf("%s: isBinary = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestE2ENoANSIInRedirectedStreams: with stdout sent to the null device and
// stderr to a file, nothing is coloured unless --color forces it.
func TestE2ENoANSIInRedirectedStreams(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "fail.hurl", "GET "+srv.URL+"/hello\nHTTP 404\n")
	for _, tc := range []struct {
		args []string
		ansi bool
	}{
		{[]string{file}, false},
		{[]string{"--color", file}, true},
	} {
		null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		logPath := filepath.Join(t.TempDir(), "log")
		log, err := os.Create(logPath)
		if err != nil {
			t.Fatal(err)
		}
		code := run(context.Background(), tc.args, null, log)
		_ = null.Close()
		_ = log.Close()
		got, err := os.ReadFile(logPath) //nolint:gosec // G304: test temp file.
		if err != nil {
			t.Fatal(err)
		}
		if code != ExitAssert || len(got) == 0 {
			t.Fatalf("%v: exit %d, stderr %q; want an assert failure on stderr", tc.args, code, got)
		}
		if hasANSI := strings.Contains(string(got), "\x1b["); hasANSI != tc.ansi {
			t.Errorf("%v: stderr has ANSI = %v, want %v:\n%q", tc.args, hasANSI, tc.ansi, got)
		}
	}
}

// TestE2EOutputDashRendering: an `output: -` entry gets the same pretty
// and colour rendering as the default output, empty containers are one
// coloured token, and without --pretty the body is written as received.
func TestE2EOutputDashRendering(t *testing.T) {
	const body = `{"a":[],"b":{},"c":[1]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	file := writeTemp(t, "out.hurl", "GET "+srv.URL+"/\n[Options]\noutput: -\nHTTP 200\n")

	_, raw, _ := runArgs(t, "--no-output", file)
	if raw != body {
		t.Errorf("without --pretty: stdout = %q, want the body unchanged", raw)
	}

	_, pretty, _ := runArgs(t, "--no-output", "--pretty", "--color", file)
	want := "\x1b[1;39m{\x1b[0m\n" +
		"  \x1b[1;34m\"a\"\x1b[0m\x1b[1;39m:\x1b[0m \x1b[1;39m[]\x1b[0m\x1b[1;39m,\x1b[0m\n" +
		"  \x1b[1;34m\"b\"\x1b[0m\x1b[1;39m:\x1b[0m \x1b[1;39m{}\x1b[0m\x1b[1;39m,\x1b[0m\n" +
		"  \x1b[1;34m\"c\"\x1b[0m\x1b[1;39m:\x1b[0m \x1b[1;39m[\x1b[0m\n" +
		"    \x1b[0;36m1\x1b[0m\n" +
		"  \x1b[1;39m]\x1b[0m\n" +
		"\x1b[1;39m}\x1b[0m\n"
	if pretty != want {
		t.Errorf("--pretty --color:\n got %q\nwant %q", pretty, want)
	}
}

// TestWriteFileOutputBinaryToTerminal: a binary body bound for a terminal
// stdout is refused (nothing written, coloured error when stderr is), while
// --output -, a file target and a non-terminal stdout all get the bytes.
func TestWriteFileOutputBinaryToTerminal(t *testing.T) {
	src := "GET http://localhost/bin\nHTTP 200\n"
	resp := &exchange.Response{Status: 200, Body: []byte("\x1f\x8b\x08\x00")}
	res := &engine.UnitResult{
		File: "b.hurl", Source: []byte(src), Success: true,
		Entries: []*engine.EntryResult{{Line: 1, Calls: []engine.Call{{Response: resp}}}},
	}
	for _, tc := range []struct {
		name    string
		rc      runContext
		refused bool
	}{
		{"tty", runContext{stdoutTTY: true}, true},
		{"tty with -i", runContext{stdoutTTY: true, include: true}, true},
		{"tty --output -", runContext{stdoutTTY: true, output: "-"}, false},
		{"pipe", runContext{}, false},
	} {
		var stdout bytes.Buffer
		err := writeFileOutput(&tc.rc, newOutputSink(tc.rc.output, &stdout), nil, res)
		var oe *outputError
		if refused := errors.As(err, &oe); refused != tc.refused {
			t.Errorf("%s: err = %v, want refused = %v", tc.name, err, tc.refused)
			continue
		}
		if tc.refused {
			if stdout.Len() != 0 || !strings.Contains(oe.rendered, "Binary output") {
				t.Errorf("%s: stdout %q, error %q; want nothing written and a Binary output error", tc.name, stdout.String(), oe.rendered)
			}
		} else if !bytes.HasSuffix(stdout.Bytes(), resp.Body) {
			t.Errorf("%s: stdout = %q, want the body", tc.name, stdout.String())
		}
	}

	rc := runContext{stdoutTTY: true, colorErr: true}
	err := writeFileOutput(&rc, newOutputSink("", &bytes.Buffer{}), nil, res)
	var oe *outputError
	if !errors.As(err, &oe) || !oe.color || !strings.Contains(oe.rendered, "\x1b[") {
		t.Errorf("coloured stderr: err = %v, want a colour-rendered error", err)
	}
}
