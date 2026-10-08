// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Hello World!"))
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":1}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestE2ESuccessBody(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	code, out, errOut := runArgs(t, file)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "Hello World!" {
		t.Errorf("stdout = %q, want %q", out, "Hello World!")
	}
}

func TestE2EInclude(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	code, out, _ := runArgs(t, file, "-i")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if !strings.HasPrefix(out, "HTTP/1.1 200") {
		t.Errorf("stdout does not start with a status line:\n%s", out)
	}
	if !strings.Contains(out, "Content-Type: text/plain") {
		t.Errorf("stdout missing headers:\n%s", out)
	}
	if !strings.HasSuffix(out, "Hello World!") {
		t.Errorf("stdout missing body:\n%s", out)
	}
}

func TestE2ENoOutput(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	code, out, _ := runArgs(t, file, "--no-output")
	if code != ExitOK || out != "" {
		t.Errorf("code=%d out=%q, want %d and empty", code, out, ExitOK)
	}
}

func TestE2EAssertFailureExitCode(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "bad.hurl", "GET "+srv.URL+"/hello\nHTTP 404\n")
	code, out, errOut := runArgs(t, file)
	if code != ExitAssert {
		t.Fatalf("exit code = %d, want %d", code, ExitAssert)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty on a failed file", out)
	}
	if !strings.Contains(errOut, "error: Assert status code") {
		t.Errorf("stderr missing the assert error:\n%s", errOut)
	}
}

func TestE2ERuntimeErrorExitCode(t *testing.T) {
	file := writeTemp(t, "bad.hurl", "GET http://127.0.0.1:1\nHTTP 200\n")
	if code, _, _ := runArgs(t, file); code != ExitRuntime {
		t.Errorf("exit code = %d, want %d", code, ExitRuntime)
	}
}

func TestE2EVariable(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET {{host}}/hello\nHTTP 200\n")
	code, out, errOut := runArgs(t, file, "--variable", "host="+srv.URL)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "Hello World!" {
		t.Errorf("stdout = %q", out)
	}
}

func TestE2EJSONOutputShape(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	code, out, errOut := runArgs(t, file, "--json")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("--json output does not parse: %v\n%s", err, out)
	}
	for _, key := range []string{"cookies", "entries", "filename", "success", "time"} {
		if _, ok := result[key]; !ok {
			t.Errorf("--json output missing key %q:\n%s", key, out)
		}
	}
	if result["filename"] != file {
		t.Errorf("filename = %v, want %q", result["filename"], file)
	}
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
	entries, _ := result["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries has %d items, want 1", len(entries))
	}
	entry := entries[0].(map[string]any)
	if entry["curl_cmd"] == "" {
		t.Error("curl_cmd is empty")
	}
	calls, _ := entry["calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("calls has %d items, want 1", len(calls))
	}
	call := calls[0].(map[string]any)
	resp := call["response"].(map[string]any)
	if resp["status"] != float64(200) {
		t.Errorf("response.status = %v, want 200", resp["status"])
	}
	timings := call["timings"].(map[string]any)
	for _, key := range []string{"app_connect", "begin_call", "connect", "end_call", "name_lookup", "pre_transfer", "start_transfer", "total"} {
		if _, ok := timings[key]; !ok {
			t.Errorf("timings missing key %q", key)
		}
	}
}

func TestE2EPretty(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/json\nHTTP 200\n")
	code, out, _ := runArgs(t, file, "--pretty")
	if code != ExitOK {
		t.Fatalf("exit code = %d", code)
	}
	if out == `{"a":1}` {
		t.Error("--pretty did not change the compact body")
	}
	if !strings.Contains(out, "\n") {
		t.Errorf("--pretty output has no newline:\n%q", out)
	}
}

// TestE2ESSLNoRevoke checks that --ssl-no-revoke is accepted and the
// HTTPS request made as without it (no revocation is checked anywhere).
func TestE2ESSLNoRevoke(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"\nHTTP 200\n")
	if code, out, errOut := runArgs(t, "--ssl-no-revoke", "--insecure", file); code != ExitOK || out != "ok" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestE2EStdin(t *testing.T) {
	srv := testServer(t)
	src := "GET " + srv.URL + "/hello\nHTTP 200\n"

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(src); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	code, out, errOut := runArgs(t)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if out != "Hello World!" {
		t.Errorf("stdout = %q", out)
	}
}

func TestE2ERunSubcommand(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	code, out, _ := runArgs(t, "run", file)
	if code != ExitOK || out != "Hello World!" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestE2EMultipleFilesExitCodeAggregation(t *testing.T) {
	srv := testServer(t)
	ok := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")
	bad := writeTemp(t, "bad.hurl", "GET "+srv.URL+"/hello\nHTTP 404\n")
	code, _, _ := runArgs(t, ok, bad)
	if code != ExitAssert {
		t.Errorf("exit code = %d, want %d (assert failure wins over one success)", code, ExitAssert)
	}
}

// TestE2ERepeatEnvVar checks that HURL_REPEAT/SONDE_REPEAT are actually
// read: resolveCount used to be called only when --repeat was given on
// the command line, so the env var fallback inside it was dead code.
func TestE2ERepeatEnvVar(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")

	t.Run("HURL_REPEAT applies", func(t *testing.T) {
		t.Setenv("HURL_REPEAT", "3")
		code, out, errOut := runArgs(t, file)
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
		}
		if want := strings.Repeat("Hello World!", 3); out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	})

	t.Run("SONDE_REPEAT wins over HURL_REPEAT", func(t *testing.T) {
		t.Setenv("HURL_REPEAT", "5")
		t.Setenv("SONDE_REPEAT", "2")
		code, out, errOut := runArgs(t, file)
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
		}
		if want := strings.Repeat("Hello World!", 2); out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	})

	t.Run("--repeat wins over HURL_REPEAT", func(t *testing.T) {
		t.Setenv("HURL_REPEAT", "5")
		code, out, errOut := runArgs(t, file, "--repeat", "1")
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
		}
		if out != "Hello World!" {
			t.Errorf("stdout = %q, want %q", out, "Hello World!")
		}
	})
}
