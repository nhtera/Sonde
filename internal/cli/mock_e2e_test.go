// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a stderr written by the command while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

var mockAddrRe = regexp.MustCompile(`at (http://\S+) \(Ctrl-C to stop\)`)

// startMock runs `sonde mock args...` in process and returns its base URL
// and a function stopping it (Ctrl-C) that returns the exit code and
// stderr.
func startMock(t *testing.T, args ...string) (string, func() (int, string)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var errOut lockedBuffer
	done := make(chan int, 1)
	go func() { done <- run(ctx, append([]string{"mock", "--port", "0"}, args...), io.Discard, &errOut) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if m := mockAddrRe.FindStringSubmatch(errOut.String()); m != nil {
			return m[1], func() (int, string) { cancel(); return <-done, errOut.String() }
		}
		select {
		case code := <-done:
			cancel()
			t.Fatalf("mock exited %d:\n%s", code, errOut.String())
		case <-time.After(10 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("mock did not start:\n%s", errOut.String())
		}
	}
}

// TestE2EMock serves a spec, runs the suite imported from it against the
// mock with contract validation, and stops it with Ctrl-C.
func TestE2EMock(t *testing.T) {
	specPath := petSpec(t)
	base, stop := startMock(t, specPath, "--validate-requests", "--server", "http://localhost/api")
	dir := filepath.Join(t.TempDir(), "out")
	if code, _, errOut := runArgs(t, "import", "openapi", specPath, "-o", dir); code != ExitOK {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.hurl"))
	if err != nil || len(files) != 5 {
		t.Fatalf("files = %v, %v", files, err)
	}
	args := append([]string{"--test", "--openapi", specPath, "--openapi-server", base + "/api", "--openapi-strict",
		"--variable", "base_url=" + base + "/api", "--variable", "username=u", "--secret", "password=p4ssw0rd"}, files...)
	if code, _, errOut := runArgs(t, args...); code != ExitOK {
		t.Fatalf("run: exit %d\n%s", code, errOut)
	}
	file := writeTemp(t, "prefer.hurl", "GET "+base+"/api/pets/1\nPrefer: code=404\nHTTP 404\n[Asserts]\njsonpath \"$.code\" == 1\n"+
		"GET "+base+"/api/pets/abc\nHTTP 422\n[Asserts]\nheader \"Content-Type\" == \"application/problem+json\"\n")
	if code, _, errOut := runArgs(t, "--test", file); code != ExitOK {
		t.Fatalf("prefer: exit %d\n%s", code, errOut)
	}
	code, errOut := stop()
	if code != ExitInterrupted {
		t.Errorf("exit %d, want %d", code, ExitInterrupted)
	}
	for _, want := range []string{"serving 5 operation(s)", "GET /api/pets/1 404 ", "GET /api/pets/abc 422 "} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "p4ssw0rd") {
		t.Error("the log leaks a request credential")
	}
}

func TestMockErrors(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck // test
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	for _, tc := range []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"mock"}, ExitUsage, "accepts 1 arg"},
		{[]string{"mock", petSpec(t), "--port", "70000"}, ExitUsage, "invalid value '70000' for '--port <PORT>'"},
		{[]string{"mock", "missing.yaml"}, ExitUsage, "missing.yaml"},
		{[]string{"mock", petSpec(t), "--host", ""}, ExitUsage, "invalid value '' for '--host <HOST>'"},
		{[]string{"mock", "https://example.com/spec.yaml"}, ExitUsage, "use --openapi-allow-remote"},
		{[]string{"mock", petSpec(t), "--port", port}, ExitRuntime, "bind"},
	} {
		code, _, errOut := runArgs(t, tc.args...)
		if code != tc.code || !strings.Contains(errOut, tc.msg) {
			t.Errorf("%q: exit %d %q, want %d %q", tc.args, code, errOut, tc.code, tc.msg)
		}
	}
}
