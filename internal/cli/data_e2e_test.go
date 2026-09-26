// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// echoServer answers "<u>:<p>" from the query parameters u and p.
func echoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Set-Cookie", "session="+r.URL.Query().Get("p"))
		_, _ = io.WriteString(w, r.URL.Query().Get("u")+":"+r.URL.Query().Get("p")) //nolint:gosec // G705: test server echoing its input
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestE2EDataCSV(t *testing.T) {
	srv := echoServer(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"?u={{user}}&p={{data_row}}\nHTTP 200\n")
	data := writeTemp(t, "rows.csv", "user\nalice\nbob\n")
	code, out, errOut := runArgs(t, file, "--data", data)
	if code != ExitOK {
		t.Fatalf("exit code = %d; stderr=%s", code, errOut)
	}
	if out != "alice:1bob:2" {
		t.Errorf("stdout = %q", out)
	}
}

func TestE2EDataJSONTypes(t *testing.T) {
	srv := echoServer(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"?u={{user}}\nHTTP 200\n[Asserts]\n"+
		"variable \"age\" isInteger\nvariable \"admin\" isBoolean\nvariable \"tags\" count == 2\n")
	data := writeTemp(t, "rows.json", `[{"user": "alice", "age": 30, "admin": true, "tags": ["a", "b"]}]`)
	code, _, errOut := runArgs(t, file, "--data", data)
	if code != ExitOK {
		t.Fatalf("exit code = %d; stderr=%s", code, errOut)
	}
}

func TestE2EDataCSVTypes(t *testing.T) {
	srv := echoServer(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"\nHTTP 200\n[Asserts]\n"+
		"variable \"n\" isInteger\nvariable \"f\" isFloat\nvariable \"b\" isBoolean\n"+
		"variable \"s\" isString\nvariable \"q\" == \"42\"\nvariable \"e\" == \"\"\nvariable \"data_row\" == 1\n")
	data := writeTemp(t, "rows.csv", "n,f,b,s,q,e\n42,1.5,true,abc,\"\"\"42\"\"\",\n")
	code, _, errOut := runArgs(t, file, "--data", data)
	if code != ExitOK {
		t.Fatalf("exit code = %d; stderr=%s", code, errOut)
	}
}

func TestE2EDataPrecedence(t *testing.T) {
	srv := echoServer(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"?u={{user}}&p={{pass}}\nHTTP 200\n")
	data := writeTemp(t, "rows.csv", "user,pass\nrow-user,row-pass\n")
	vars := writeTemp(t, "vars.env", "user=file-user\npass=file-pass\n")
	t.Setenv("SONDE_VARIABLE_user", "env-user")
	code, out, errOut := runArgs(t, file, "--data", data, "--variables-file", vars, "--variable", "pass=flag-pass")
	if code != ExitOK {
		t.Fatalf("exit code = %d; stderr=%s", code, errOut)
	}
	// Rows win over env variables and variables files; --variable wins
	// over rows.
	if out != "row-user:flag-pass" {
		t.Errorf("stdout = %q", out)
	}
}

func TestE2EDataTestModeLabels(t *testing.T) {
	srv := echoServer(t)
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"?u={{user}}\nHTTP 200\n[Asserts]\nbody startsWith \"alice\"\n")
	data := writeTemp(t, "rows.csv", "user\nalice\nbob\n")
	code, _, errOut := runArgs(t, "--test", "--jobs", "2", file, "--data", data)
	if code != ExitAssert {
		t.Fatalf("exit code = %d; stderr=%s", code, errOut)
	}
	for _, want := range []string{"Success " + file + "#row-1", "Failure " + file + "#row-2", "Executed files:    2"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errOut)
		}
	}
}

// TestE2EDataSecretsNeverLeak runs rows whose secret column is echoed
// back in the body and a cookie, with every output enabled, and checks no
// secret value appears anywhere.
func TestE2EDataSecretsNeverLeak(t *testing.T) {
	srv := echoServer(t)
	dir := t.TempDir()
	file := writeTemp(t, "a.hurl", "GET "+srv.URL+"?u={{user}}&p={{password}}\nHTTP 200\n[Asserts]\nbody == \"nope\"\n")
	data := writeTemp(t, "rows.csv", "user,password\nalice,row-secret-alpha\nbob,row-secret-bravo\n")
	for _, args := range [][]string{
		{"--test", "--jobs", "2", "-v"},
		{"--very-verbose", "--error-format", "long"},
		{"--json"},
	} {
		args = append(args, file, "--data", data, "--data-secret", "password",
			"--report-junit", filepath.Join(dir, "junit.xml"), "--report-tap", filepath.Join(dir, "r.tap"),
			"--report-json", filepath.Join(dir, "json"), "--report-html", filepath.Join(dir, "html"),
			"--curl", filepath.Join(dir, "curl.txt"), "--cookie-jar", filepath.Join(dir, "cookies.txt"))
		code, out, errOut := runArgs(t, args...)
		if code != ExitAssert {
			t.Fatalf("%v: exit code = %d; stderr=%s", args[:2], code, errOut)
		}
		all := out + errOut
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(path) //nolint:gosec // G304: test temp dir
			all += string(b)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"row-secret-alpha", "row-secret-bravo"} {
			if strings.Contains(all, secret) {
				t.Errorf("%v: output leaks a row secret", args[:2])
			}
		}
		if !strings.Contains(all, "alice") {
			t.Errorf("%v: output lacks the non-secret column", args[:2])
		}
	}
}

func TestE2EDataErrors(t *testing.T) {
	file := writeTemp(t, "a.hurl", "GET http://localhost:1\n")
	for _, tc := range []struct {
		name, content string
		args          []string
		want          string
	}{
		{"rows.txt", "a\n1\n", nil, "unsupported data file format"},
		{"rows.csv", "a,a\n1,2\n", nil, `duplicate column "a"`},
		{"rows.csv", "a\n1\n", []string{"--data-secret", "b"}, `no column "b"`},
		{"rows.csv", "data_row\n1\n", nil, `column "data_row" is reserved`},
		{"rows.csv", "token\n1\n", []string{"--secret", "token=abcd"}, `column "token" is already defined as a secret`},
		{"rows.csv", "a\n\"\"\"x\"\n", nil, `rows.csv:2: column "a": Value should end with a double quote`},
		{"rows.json", `[{"s": {"k": 1}}]`, []string{"--data-secret", "s"}, "a secret must be a string"},
	} {
		data := writeTemp(t, tc.name, tc.content)
		args := append([]string{file, "--data", data}, tc.args...)
		code, _, errOut := runArgs(t, args...)
		if code != ExitUsage || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s %q: exit %d, stderr %q; want exit 1 and %q", tc.name, tc.content, code, errOut, tc.want)
		}
	}
	code, _, errOut := runArgs(t, file, "--data-secret", "a")
	if code != ExitUsage || !strings.Contains(errOut, "--data-secret requires --data") {
		t.Errorf("--data-secret alone: exit %d, stderr %q", code, errOut)
	}
}

func TestE2EDataNoRows(t *testing.T) {
	file := writeTemp(t, "a.hurl", "GET http://localhost:1\n")
	data := writeTemp(t, "rows.csv", "a\n")
	code, _, errOut := runArgs(t, "--test", file, "--data", data)
	if code != ExitOK || !strings.Contains(errOut, "no data rows") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

// TestE2EDataNoRowsInfiniteRepeat checks that --repeat -1 with a data file
// without rows ends instead of spinning.
func TestE2EDataNoRowsInfiniteRepeat(t *testing.T) {
	file := writeTemp(t, "a.hurl", "GET http://localhost:1\n")
	data := writeTemp(t, "rows.csv", "a\n")
	done := make(chan int, 1)
	go func() {
		code, _, _ := runArgs(t, "--test", "--repeat", "-1", file, "--data", data)
		done <- code
	}()
	select {
	case code := <-done:
		if code != ExitOK {
			t.Errorf("exit code = %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("--repeat -1 with no data rows did not end")
	}
}

func TestE2EDataColumnClashesWithSondeYAMLSecret(t *testing.T) {
	dir, file := writeProjectFile(t, "sonde.yaml",
		"version: 1\nenvironments:\n  local:\n    secrets_files: [s.env]\ndefaults:\n  env: local\n",
		"a.hurl", "GET http://localhost:1\n")
	if err := os.WriteFile(filepath.Join(dir, "s.env"), []byte("token=abcdefgh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := writeTemp(t, "rows.csv", "token\nx\n")
	code, _, errOut := runArgs(t, file, "--data", data)
	if code != ExitUsage || !strings.Contains(errOut, `data column "token" is already defined as a secret`) {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestE2EDataParseErrorReportedOnce(t *testing.T) {
	file := writeTemp(t, "a.hurl", "GET\n")
	data := writeTemp(t, "rows.csv", "a\n1\n2\n3\n4\n5\n6\n7\n8\n")
	code, _, errOut := runArgs(t, "--test", "--jobs", "4", file, "--data", data)
	if code != ExitParse {
		t.Fatalf("exit code = %d; stderr=%s", code, errOut)
	}
	if n := strings.Count(errOut, "error: "); n != 1 {
		t.Errorf("parse error printed %d times:\n%s", n, errOut)
	}
}
