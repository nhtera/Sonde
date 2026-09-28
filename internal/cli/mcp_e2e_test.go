// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestE2EMCP drives `sonde mcp` through its standard input and output:
// the tools work, and standard output carries nothing but protocol
// messages, even for a file with `output: -`.
func TestE2EMCP(t *testing.T) {
	t.Chdir(t.TempDir()) // sonde mcp changes into its root; restored after the test
	const secret = "env-secret-value-9f3"
	t.Setenv("SONDE_SECRET_tok", secret)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := r.Header.Get("X-Tok"); tok != "" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("echo " + tok)) //nolint:gosec // G705: plain-text echo in a test server
			return
		}
		_, _ = w.Write([]byte("PLAIN BODY"))
	}))
	t.Cleanup(srv.Close)
	root := t.TempDir()
	for name, content := range map[string]string{
		"ok.hurl":     "GET " + srv.URL + "/\nHTTP 200\n",
		"stdout.hurl": "GET " + srv.URL + "/\n[Options]\noutput: -\nHTTP 200\n",
		"secret.hurl": "GET " + srv.URL + "/\nX-Tok: {{tok}}\nHTTP 200\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	wire := &lockedBuffer{}
	var stderr lockedBuffer
	cmd := newRootCmd(io.MultiWriter(outW, wire), &stderr)
	cmd.SetIn(inR)
	cmd.SetArgs([]string{"mcp", "--root", root, "--allow-run", "--allow-host", strings.TrimPrefix(srv.URL, "http://")})
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(context.Background())
		_ = outW.Close()
	}()

	ctx := context.Background()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).
		Connect(ctx, &sdk.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 3 {
		t.Fatalf("tools = %v, %v", tools, err)
	}
	for _, path := range []string{"ok.hurl", "stdout.hurl", "secret.hurl"} {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "sonde_run", Arguments: map[string]any{"path": path}})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError != (path != "ok.hurl") {
			t.Errorf("%s: IsError = %v", path, res.IsError)
		}
	}
	_ = cs.Close()
	if err := <-done; err != nil {
		t.Fatalf("sonde mcp: %v\nstderr: %s", err, stderr.String())
	}

	if strings.Contains(wire.String(), "PLAIN BODY\n") || strings.HasPrefix(wire.String(), "PLAIN") {
		t.Error("a response body reached standard output")
	}
	sc := bufio.NewScanner(strings.NewReader(wire.String()))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var msg struct {
			JSONRPC string `json:"jsonrpc"`
		}
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil || msg.JSONRPC != "2.0" {
			t.Errorf("standard output line is not a JSON-RPC message: %q", sc.Text())
		}
	}
	if !strings.Contains(wire.String(), "echo ***") || strings.Contains(wire.String()+stderr.String(), secret) {
		t.Error("the SONDE_SECRET_ value was not redacted")
	}
	log := stderr.String()
	if !strings.Contains(log, "sonde mcp: serving") || strings.Count(log, "sonde mcp: sonde_run") != 3 {
		t.Errorf("stderr = %s", log)
	}
}

func TestMCPUsageErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"mcp", "--allow-run"},
		{"mcp", "--allow-host", "example.com"},
		{"mcp", "--allow-run", "--allow-host", "http://example.com"},
		{"mcp", "--allow-run", "--allow-host", "127.1"},
		{"mcp", "--root", filepath.Join(dir, "missing")},
		{"mcp", "--root", file},
		{"mcp", "--run-timeout", "11m"},
		{"mcp", "--run-timeout", "1ms"},
		{"mcp", "extra"},
	} {
		code, out, errOut := runArgs(t, args...)
		if code != ExitUsage || out != "" || !strings.HasPrefix(errOut, "error: ") {
			t.Errorf("%v: code=%d stdout=%q stderr=%q", args, code, out, errOut)
		}
	}
}

// TestMCPStopsOnFirstCtrlC: the first Ctrl-C (the stop channel) ends the
// server at once, while stdin is still open.
func TestMCPStopsOnFirstCtrlC(t *testing.T) {
	t.Chdir(t.TempDir())
	inR, inW := io.Pipe()
	defer inW.Close() //nolint:errcheck // test pipe
	var stderr lockedBuffer
	cmd := newRootCmd(io.Discard, &stderr)
	cmd.SetIn(inR)
	cmd.SetArgs([]string{"mcp", "--root", t.TempDir()})
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(withStop(context.Background(), stop)) }()
	for !strings.Contains(stderr.String(), "serving") {
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sonde mcp: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sonde mcp did not stop on the first Ctrl-C")
	}
}
