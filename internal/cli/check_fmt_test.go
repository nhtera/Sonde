// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	formatted   = "GET http://a\nHTTP 200\n"
	unformatted = "GET   http://a\nHTTP  200"
	invalid     = "GET http://a\nHTTP abc\n"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheck(t *testing.T) {
	good := writeTemp(t, "good.hurl", formatted)
	bad := writeTemp(t, "bad.hurl", invalid)
	sonde := writeTemp(t, "a.sonde", unformatted)

	if code, out, errOut := runArgs(t, "check", good, sonde); code != ExitOK || out != "" || errOut != "" {
		t.Errorf("valid files: code %d, stdout %q, stderr %q", code, out, errOut)
	}

	missing := filepath.Join(t.TempDir(), "missing.hurl")
	code, _, errOut := runArgs(t, "check", good, bad, missing)
	if code != ExitParse {
		t.Errorf("exit code = %d, want %d", code, ExitParse)
	}
	for _, want := range []string{
		"error: Parsing status code\n  --> " + bad + ":2:6\n",
		" 2 | HTTP abc\n",
		"^ HTTP status code is not valid\n",
		"error: Input file " + missing + " can not be read - ",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "error: exit code") {
		t.Errorf("silent exit printed a generic error:\n%s", errOut)
	}
}

func TestCheckInvalidUTF8(t *testing.T) {
	path := writeTemp(t, "bin.hurl", "GET http://a\n\xff\n")
	code, _, errOut := runArgs(t, "check", path)
	want := "error: Input file " + path + " can not be read - invalid utf-8 sequence of 1 bytes from index 13\n"
	if code != ExitParse || errOut != want {
		t.Errorf("code %d, stderr %q, want %q", code, errOut, want)
	}
}

func TestCheckRequiresFiles(t *testing.T) {
	if code, _, _ := runArgs(t, "check"); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
}

func TestFmtStdout(t *testing.T) {
	path := writeTemp(t, "a.hurl", unformatted)
	code, out, _ := runArgs(t, "fmt", path)
	if code != ExitOK || out != formatted {
		t.Errorf("code %d, stdout %q, want %q", code, out, formatted)
	}
	if b, _ := os.ReadFile(path); string(b) != unformatted {
		t.Error("fmt without --write modified the file")
	}
}

func TestFmtCheck(t *testing.T) {
	good := writeTemp(t, "good.hurl", formatted)
	bad := writeTemp(t, "bad.hurl", unformatted)
	if code, out, _ := runArgs(t, "fmt", "--check", good); code != ExitOK || out != "" {
		t.Errorf("formatted: code %d, stdout %q", code, out)
	}
	code, out, _ := runArgs(t, "fmt", "--check", good, bad)
	if want := "would reformat: " + bad + "\n1 file would be reformatted\n"; code != ExitUsage || out != want {
		t.Errorf("unformatted: code %d, stdout %q, want %d and %q", code, out, ExitUsage, want)
	}
	broken := writeTemp(t, "broken.hurl", invalid)
	if code, _, _ := runArgs(t, "fmt", "--check", bad, broken); code != ExitParse {
		t.Errorf("parse error wins: code %d, want %d", code, ExitParse)
	}
}

func TestFmtWrite(t *testing.T) {
	path := writeTemp(t, "a.hurl", unformatted)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o640); err != nil { //nolint:gosec // G302: must differ from the temp file default 0600
			t.Fatal(err)
		}
	}
	code, out, _ := runArgs(t, "fmt", "--write", path)
	if code != ExitOK || out != "" {
		t.Fatalf("code %d, stdout %q", code, out)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != formatted {
		t.Fatalf("file = %q, %v", b, err)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
	if code, _, _ := runArgs(t, "fmt", "--write", "--check", path); code != ExitUsage {
		t.Errorf("--write with --check: code %d, want %d", code, ExitUsage)
	}
}

// TestNoCompletionCommand checks that `completion` is not a generated
// cobra command: `sonde completion bash` is upstream-style dispatch, so it is
// read as two input files named "completion" and "bash", both missing.
func TestNoCompletionCommand(t *testing.T) {
	code, _, errOut := runArgs(t, "completion", "bash")
	if code != ExitUsage {
		t.Errorf("completion: code %d, want %d (treated as missing input files)", code, ExitUsage)
	}
	if !strings.Contains(errOut, "Cannot access 'completion'") {
		t.Errorf("stderr = %q, want it to report completion as a missing file", errOut)
	}
}

// superseded810 rewrites 8.0.1 parser-error oracles into what the
// reference prints since 8.1.0, which sonde follows.
var superseded810 = map[string]func(string) string{
	// 8.1.0 lists two more option names.
	"tests_error_parser/invalid_option.err": strings.NewReplacer(
		"http2, http3", "http2, http2-prior-knowledge, http3",
		"variable, verbose", "variable, variables-file, verbose",
	).Replace,
}

// TestCheckMatchesOracle runs `check` on the parser-error suite from the
// suite directory; stderr must equal the oracle's .err file exactly (as
// 8.1.0 changed it, see superseded810).
func TestCheckMatchesOracle(t *testing.T) {
	dir, err := filepath.Abs("../../testdata/conformance/hurl")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	errs, _ := filepath.Glob("tests_error_parser/*.err")
	compared := 0
	for _, errFile := range errs {
		hurlFile := filepath.ToSlash(strings.TrimSuffix(errFile, ".err") + ".hurl")
		want, err := os.ReadFile(errFile)
		if _, statErr := os.Stat(hurlFile); err != nil || statErr != nil {
			continue // multi-file scenarios are covered by the syntax tests
		}
		if rewrite, ok := superseded810[filepath.ToSlash(errFile)]; ok {
			want = []byte(rewrite(string(want)))
		}
		code, _, errOut := runArgs(t, "check", hurlFile)
		if code != ExitParse {
			t.Errorf("%s: code %d, want %d", hurlFile, code, ExitParse)
		}
		if errOut != string(want) {
			t.Errorf("%s: stderr differs from %s:\n%s\nwant:\n%s", hurlFile, errFile, errOut, want)
		}
		compared++
	}
	if compared < 40 {
		t.Errorf("compared only %d oracle files", compared)
	}
}

func TestFmtWriteFollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	target := writeTemp(t, "real.hurl", unformatted)
	link := filepath.Join(t.TempDir(), "link.hurl")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runArgs(t, "fmt", "-w", link); code != ExitOK {
		t.Fatalf("code %d", code)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link replaced by a regular file")
	}
	if b, _ := os.ReadFile(target); string(b) != formatted {
		t.Errorf("target = %q", b)
	}
}

func TestFmtWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions differ on Windows")
	}
	path := writeTemp(t, "a.hurl", unformatted)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // G302: read-only directory to force a write failure
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // G302: restore test dir
	code, _, errOut := runArgs(t, "fmt", "-w", path)
	if code != ExitUndefined || !strings.Contains(errOut, "error: Issue writing to "+path) {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("unexpected files left: %v", entries)
	}
}

func TestCheckTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.hurl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64<<20 + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	code, _, errOut := runArgs(t, "check", path)
	if code != ExitParse || !strings.Contains(errOut, "file is larger than 64 MiB") {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

func TestCheckRendersWithoutBOM(t *testing.T) {
	path := writeTemp(t, "bom.hurl", "\uFEFFget http://a\n")
	_, _, errOut := runArgs(t, "check", path)
	if !strings.Contains(errOut, " 1 | get http://a\n   | ^ ") {
		t.Errorf("stderr:\n%s", errOut)
	}
}

// withStdin replaces os.Stdin with src for the rest of the test.
func withStdin(t *testing.T, src string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(src); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
}

func TestFmtStdinAndColor(t *testing.T) {
	withStdin(t, "GET   http://localhost:8000/hello\n[Cookies]\na: b\n[Options]\ndelay: 5\n")
	code, out, _ := runArgs(t, "fmt")
	if want := "GET http://localhost:8000/hello\n[Options]\ndelay: 5ms\n[Cookies]\na: b\n"; code != ExitOK || out != want {
		t.Errorf("stdin: code %d, stdout %q, want %q", code, out, want)
	}

	withStdin(t, "GET http://localhost:8000/hello")
	code, out, _ = runArgs(t, "fmt", "--color")
	if want := "\x1b[33mGET\x1b[0m \x1b[32mhttp://localhost:8000/hello\x1b[0m\n"; code != ExitOK || out != want {
		t.Errorf("--color: code %d, stdout %q, want %q", code, out, want)
	}

	withStdin(t, "GET http://a\n")
	if code, _, errOut := runArgs(t, "fmt", "-w"); code != ExitParse || !strings.Contains(errOut, "Standard input") {
		t.Errorf("-w on stdin: code %d, stderr %q", code, errOut)
	}
}

func TestFmtColorParseError(t *testing.T) {
	path := writeTemp(t, "bad.hurl", "xxx\n")
	code, out, errOut := runArgs(t, "fmt", "--color", path)
	if code != ExitParse || !strings.HasPrefix(errOut, "\x1b[1;31merror\x1b[0m: \x1b[1mParsing method") {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
	// Like the reference formatter, stdout still ends with a newline.
	if out != "\n" {
		t.Errorf("stdout = %q, want a lone newline", out)
	}
}
