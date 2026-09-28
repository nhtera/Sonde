// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nhtera/sonde/internal/netpolicy"
)

// syncBuffer is an audit log written by the server and read by the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// tempRoot returns a temporary directory with symbolic links resolved.
func tempRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// connect starts a server for cfg and returns a client session.
func connect(t *testing.T, cfg Config) *sdk.ClientSession {
	t.Helper()
	srv, closer, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Wait()
		_ = closer.Close()
	})
	return cs
}

// call calls a tool and decodes its structured output into out.
func call(t *testing.T, cs *sdk.ClientSession, tool string, args map[string]any, out any) *sdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if out != nil && res.StructuredContent != nil {
		data, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatal(err)
		}
	}
	return res
}

func resultText(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text + "\n")
		}
	}
	return b.String()
}

func TestToolGating(t *testing.T) {
	root := tempRoot(t)
	names := func(cs *sdk.ClientSession) []string {
		res, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, tool := range res.Tools {
			out = append(out, tool.Name)
		}
		slices.Sort(out)
		return out
	}
	cs := connect(t, Config{Root: root})
	if got := names(cs); !slices.Equal(got, []string{"sonde_check", "sonde_list"}) {
		t.Fatalf("tools without --allow-run = %v", got)
	}
	if _, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "sonde_run", Arguments: map[string]any{"path": "a.hurl"}}); err == nil {
		t.Fatal("sonde_run must not exist without --allow-run")
	}
	hosts, _ := netpolicy.Parse([]string{"example.com"})
	cs = connect(t, Config{Root: root, AllowRun: true, Hosts: hosts})
	if got := names(cs); !slices.Equal(got, []string{"sonde_check", "sonde_list", "sonde_run"}) {
		t.Fatalf("tools with --allow-run = %v", got)
	}
	if _, _, err := New(Config{Root: root, AllowRun: true}); err == nil {
		t.Fatal("--allow-run without hosts must be refused")
	}
}

func TestList(t *testing.T) {
	root := tempRoot(t)
	outside := tempRoot(t)
	writeFiles(t, outside, map[string]string{"private.hurl": "GET http://x\n"})
	writeFiles(t, root, map[string]string{
		"a.hurl":         "GET http://a\n",
		"sub/b.sonde":    "GET http://b\n",
		".hidden/c.hurl": "GET http://c\n",
		"notes.txt":      "x",
		"sonde.yaml":     "version: 1\nenvironments:\n  staging: {}\n  local: {}\ndefaults:\n  env: local\n",
		"bad/sonde.yaml": "version: 7\n",
	})
	if err := os.Symlink(filepath.Join(outside, "private.hurl"), filepath.Join(root, "link.hurl")); err != nil {
		t.Fatal(err)
	}
	log := &syncBuffer{}
	cs := connect(t, Config{Root: root, Log: log})
	var out listOutput
	res := call(t, cs, "sonde_list", nil, &out)
	if res.IsError {
		t.Fatalf("list: %s", resultText(res))
	}
	var paths []string
	for _, f := range out.Files {
		paths = append(paths, f.Path+":"+f.Dialect)
	}
	if !slices.Equal(paths, []string{"a.hurl:hurl", "sub/b.sonde:sonde"}) {
		t.Errorf("files = %v", paths)
	}
	if len(out.Projects) != 2 || out.Projects[1].Path != "sonde.yaml" ||
		!slices.Equal(out.Projects[1].Environments, []string{"local", "staging"}) || out.Projects[1].DefaultEnv != "local" {
		t.Errorf("projects = %+v", out.Projects)
	}
	if out.Projects[0].Error == "" || strings.Contains(out.Projects[0].Error, root) {
		t.Errorf("a broken sonde.yaml: %+v", out.Projects[0])
	}
	if !strings.Contains(log.String(), "sonde_list") {
		t.Errorf("audit log = %q", log.String())
	}

	out = listOutput{}
	call(t, cs, "sonde_list", map[string]any{"dir": "sub"}, &out)
	if len(out.Files) != 1 || out.Files[0].Path != "sub/b.sonde" {
		t.Errorf("sub = %+v", out.Files)
	}
	for _, dir := range []string{"..", outside, "a.hurl", "missing"} {
		if res := call(t, cs, "sonde_list", map[string]any{"dir": dir}, nil); !res.IsError {
			t.Errorf("dir %q: want an error", dir)
		}
	}
}

func TestCheck(t *testing.T) {
	root := tempRoot(t)
	outside := tempRoot(t)
	writeFiles(t, outside, map[string]string{"x.hurl": "GET http://x\n"})
	writeFiles(t, root, map[string]string{
		"ok.sonde":    "# a comment\nGET {{base}}/users\n[Options]\nretry: 2\nHTTP 200\n[Asserts]\nstatus == 200\n\nPOST {{base}}/users\n",
		"bad.hurl":    "GET http://x\nHTTP 200\n[Assertz]\n",
		"notes.txt":   "token=hunter2",
		"dir/in.hurl": "GET http://in\n",
	})
	if err := os.Symlink(filepath.Join(outside, "x.hurl"), filepath.Join(root, "link.hurl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	cs := connect(t, Config{Root: root})
	var out checkOutput
	paths := []any{"ok.sonde", "bad.hurl", "dir/../dir/in.hurl", "../x.hurl", filepath.Join(outside, "x.hurl"),
		"link.hurl", "linkdir/x.hurl", "notes.txt", "missing.hurl", "a\x00.hurl"}
	call(t, cs, "sonde_check", map[string]any{"paths": paths}, &out)
	if len(out.Files) != len(paths) {
		t.Fatalf("files = %d", len(out.Files))
	}
	ok := out.Files[0]
	if !ok.OK || len(ok.Entries) != 2 || ok.Entries[0].URL != "{{base}}/users" || ok.Entries[0].Line != 2 ||
		!slices.Equal(ok.Entries[0].Options, []string{"retry"}) || !slices.Equal(ok.Entries[0].Sections, []string{"Options", "Asserts"}) ||
		ok.Entries[1].Method != "POST" {
		t.Errorf("ok.sonde = %+v", ok)
	}
	bad := out.Files[1]
	if bad.OK || bad.Error == nil || bad.Error.Line != 3 {
		t.Errorf("bad.hurl = %+v", bad)
	}
	if !out.Files[2].OK || out.Files[2].Path != "dir/in.hurl" {
		t.Errorf("dir/../dir/in.hurl = %+v", out.Files[2])
	}
	for _, f := range out.Files[3:] {
		if f.OK || f.Problem == "" || f.Entries != nil {
			t.Errorf("%q: want a problem, got %+v", f.Path, f)
		}
	}
	data, _ := json.Marshal(out)
	if strings.Contains(string(data), "hunter2") {
		t.Error("the content of a file that is not a request file leaked")
	}
	if res := call(t, cs, "sonde_check", map[string]any{"paths": []any{}}, nil); !res.IsError {
		t.Error("no paths: want an error")
	}
}

// runFixture is a root with a sonde.yaml, a secrets file and request
// files, and a server that echoes what it receives.
type runFixture struct {
	root   string
	host   string // host:port of the server
	secret string
	log    *syncBuffer
}

func newRunFixture(t *testing.T) *runFixture {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow":
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		case "/away":
			http.Redirect(w, r, "http://denied.test/", http.StatusFound)
		case "/echo":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			tok := r.Header.Get("X-Token")
			_, _ = w.Write([]byte("token " + tok + " b64 " + base64.StdEncoding.EncodeToString([]byte(tok)) + " url " + url.QueryEscape(tok) + " q " + r.URL.Query().Get("who"))) //nolint:gosec // G705: plain-text echo in a test server
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	f := &runFixture{root: tempRoot(t), host: strings.TrimPrefix(srv.URL, "http://"), secret: "s3cr3t/t0ken+value", log: &syncBuffer{}}
	writeFiles(t, f.root, map[string]string{
		"sonde.yaml": "version: 1\nenvironments:\n  local:\n    variables:\n      base: " + srv.URL + "\n    secrets_files:\n      - local.secrets\n" +
			"  other:\n    variables:\n      base: http://denied.test\ndefaults:\n  env: local\n",
		"local.secrets": "token=" + f.secret + "\n",
		"ok.hurl":       "GET {{base}}/\nHTTP 200\n[Asserts]\njsonpath \"$.ok\" == true\n",
		"echo.hurl":     "GET {{base}}/echo?who={{who}}\nX-Token: {{token}}\nHTTP 200\n",
		"slow.hurl":     "GET {{base}}/slow\nHTTP 200\n",
		"away.hurl":     "GET {{base}}/away\n[Options]\nlocation: true\nHTTP 200\n",
		"proxy.hurl":    "GET {{base}}/\n[Options]\nproxy: {{base}}\nHTTP 200\n",
		"output.hurl":   "GET {{base}}/\n[Options]\noutput: out.json\nHTTP 200\n",
		"broken.hurl":   "GET {{base}}/\nHTTP 200\n[Assertz]\n",
		"server.hurl":   "GET {{base}}/\nX-Api: {{api_key}}\nHTTP 200\n",
	})
	return f
}

func (f *runFixture) connect(t *testing.T, cfg Config) *sdk.ClientSession {
	t.Helper()
	hosts, err := netpolicy.Parse([]string{f.host})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Root, cfg.AllowRun, cfg.Hosts, cfg.Log = f.root, true, hosts, f.log
	return connect(t, cfg)
}

func TestRun(t *testing.T) {
	f := newRunFixture(t)
	cs := f.connect(t, Config{Secrets: map[string]string{"api_key": "server-side-key-123"}})

	var out runOutput
	res := call(t, cs, "sonde_run", map[string]any{"path": "ok.hurl"}, &out)
	if res.IsError || !out.Success || out.Env != "local" || out.Result == nil || len(out.Failures) != 0 {
		t.Fatalf("ok.hurl: %s", resultText(res))
	}
	if r, _ := out.Result.(map[string]any); r["filename"] != "ok.hurl" {
		t.Errorf("result filename = %v", r["filename"])
	}

	out = runOutput{}
	res = call(t, cs, "sonde_run", map[string]any{"path": "echo.hurl", "variables": map[string]any{"who": 42}}, &out)
	if !res.IsError || out.Success || len(out.Failures) != 1 {
		t.Fatalf("echo.hurl: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "echo.hurl: failed at entry 1: Assert status code") {
		t.Errorf("summary = %s", resultText(res))
	}
	fl := out.Failures[0]
	if fl.Status != 500 || !strings.Contains(fl.Body, "q 42") || !strings.Contains(fl.Body, "***") || fl.ContentType != "text/plain" {
		t.Errorf("failure = %+v", fl)
	}
	// Assert messages in the result name the file relative to the root, as
	// the failures do.
	if strings.Contains(resultText(res), f.root) {
		t.Errorf("absolute path in output: %s", resultText(res))
	}
	// No secret, raw or encoded, anywhere in the output or the audit log.
	all := resultText(res) + f.log.String()
	for _, leak := range []string{f.secret, base64.StdEncoding.EncodeToString([]byte(f.secret)), url.QueryEscape(f.secret)} {
		if strings.Contains(all, leak) {
			t.Errorf("secret form %q leaked", leak)
		}
	}

	out = runOutput{}
	res = call(t, cs, "sonde_run", map[string]any{"path": "server.hurl"}, &out)
	if res.IsError || strings.Contains(resultText(res), "server-side-key-123") {
		t.Errorf("server.hurl: %s", resultText(res))
	}

	out = runOutput{}
	res = call(t, cs, "sonde_run", map[string]any{"path": "broken.hurl"}, &out)
	if !res.IsError || out.Error == "" || out.Result != nil || strings.Contains(out.Error, f.root) {
		t.Errorf("broken.hurl: %+v", out)
	}
}

func TestRunDenied(t *testing.T) {
	f := newRunFixture(t)
	cs := f.connect(t, Config{})
	for _, tt := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"path": "ok.hurl", "env": "other"}, "not in the host allowlist"},
		{map[string]any{"path": "away.hurl"}, "not in the host allowlist"},
		{map[string]any{"path": "proxy.hurl"}, `option "proxy" is not available`},
		{map[string]any{"path": "output.hurl"}, `option "output" is not available`},
	} {
		var out runOutput
		res := call(t, cs, "sonde_run", tt.args, &out)
		if !res.IsError || len(out.Failures) != 1 || !strings.Contains(strings.Join(out.Failures[0].Errors, "\n"), tt.want) {
			t.Errorf("%v: %s", tt.args, resultText(res))
		}
	}
	if _, err := os.Stat(filepath.Join(f.root, "out.json")); err == nil {
		t.Error("output.hurl wrote a file")
	}

	for _, args := range []map[string]any{
		{"path": "../x.hurl"},
		{"path": "sonde.yaml"},
		{"path": "ok.hurl", "env": "missing"},
		{"path": "ok.hurl", "variables": map[string]any{"bad name": "x"}},
		{"path": "ok.hurl", "variables": map[string]any{"obj": map[string]any{}}},
		{"path": "ok.hurl", "variables": map[string]any{"token": "mine"}},
		{"path": "ok.hurl", "variables": map[string]any{"long": strings.Repeat("x", maxVariableBytes+1)}},
	} {
		if res := call(t, cs, "sonde_run", args, nil); !res.IsError {
			t.Errorf("%v: want an error", args)
		}
	}
	if !strings.Contains(f.log.String(), "refused") {
		t.Errorf("audit log = %q", f.log.String())
	}
}

// TestRunProjectAboveRoot: a sonde.yaml above the root is never used.
func TestRunProjectAboveRoot(t *testing.T) {
	f := newRunFixture(t)
	sub := filepath.Join(f.root, "sub")
	writeFiles(t, sub, map[string]string{"a.hurl": "GET http://" + f.host + "/\nHTTP 200\n"})
	hosts, _ := netpolicy.Parse([]string{f.host})
	cs := connect(t, Config{Root: sub, AllowRun: true, Hosts: hosts})
	if res := call(t, cs, "sonde_run", map[string]any{"path": "a.hurl", "env": "local"}, nil); !res.IsError ||
		!strings.Contains(resultText(res), "no sonde.yaml under the server root") {
		t.Errorf("env from above the root: %s", resultText(res))
	}
	var out runOutput
	if res := call(t, cs, "sonde_run", map[string]any{"path": "a.hurl"}, &out); res.IsError || out.Env != "" {
		t.Errorf("without env: %s", resultText(res))
	}
}

func TestRunTimeoutAndCancel(t *testing.T) {
	f := newRunFixture(t)
	cs := f.connect(t, Config{RunTimeout: 200 * time.Millisecond})
	var out runOutput
	start := time.Now()
	res := call(t, cs, "sonde_run", map[string]any{"path": "slow.hurl"}, &out)
	if !res.IsError || !out.TimedOut || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout: %s", resultText(res))
	}

	cs = f.connect(t, Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start = time.Now()
	_, _ = cs.CallTool(ctx, &sdk.CallToolParams{Name: "sonde_run", Arguments: map[string]any{"path": "slow.hurl"}})
	// The server must be free again soon: the next run starts at once.
	res = call(t, cs, "sonde_run", map[string]any{"path": "ok.hurl"}, nil)
	if res.IsError || time.Since(start) > 3*time.Second {
		t.Fatalf("after cancel (%s): %s", time.Since(start), resultText(res))
	}
}

func TestRunOutputCaps(t *testing.T) {
	body := strings.Repeat("é", maxFailureBody)
	o := runOutput{Failures: []failure{{Body: body}}}
	o.fit()
	if o.Truncated {
		t.Error("a small output must not be truncated")
	}
	cut, truncated := truncate(body, maxFailureBody)
	if !truncated || len(cut) > maxFailureBody || !strings.HasPrefix(body, cut) || !utf8.ValidString(cut) {
		t.Errorf("body cut: %d bytes, truncated=%v", len(cut), truncated)
	}
	big := runOutput{Result: strings.Repeat("x", maxResultBytes), Failures: []failure{{Body: "b"}}}
	big.fit()
	if !big.Truncated || big.Result != nil || big.Failures[0].Body != "" {
		t.Errorf("big output: truncated=%v result kept=%v", big.Truncated, big.Result != nil)
	}
}

func TestCallVariables(t *testing.T) {
	got, err := callVariables(map[string]any{"n": 42.0, "f": 1.5, "s": "x", "b": true})
	if err != nil {
		t.Fatal(err)
	}
	if got["n"] != int64(42) || got["f"] != 1.5 || got["s"] != "x" || got["b"] != true {
		t.Errorf("variables = %#v", got)
	}
	many := map[string]any{}
	for i := range maxVariables + 1 {
		many["v"+strings.Repeat("x", i)] = "x"
	}
	if _, err := callVariables(many); err == nil {
		t.Error("too many variables: want an error")
	}
}

// TestRunNotStarted: a run whose context ends before it starts is an
// error, not a crash. Where the clock is coarse (Windows), a 1ns timeout
// may expire only once the run has started, and then it times out, or
// only once the run is over, and then it succeeds.
func TestRunNotStarted(t *testing.T) {
	f := newRunFixture(t)
	cs := f.connect(t, Config{RunTimeout: time.Nanosecond})
	for range 3 {
		res := call(t, cs, "sonde_run", map[string]any{"path": "ok.hurl"}, nil)
		text := resultText(res)
		stopped := res.IsError && (strings.Contains(text, "did not start") || strings.Contains(text, "timed out"))
		if !stopped && (res.IsError || !strings.Contains(text, "ok.hurl: success")) {
			t.Fatalf("result: %s", text)
		}
	}
}

// TestRunSerialized: concurrent runs are answered one after the other.
func TestRunSerialized(t *testing.T) {
	f := newRunFixture(t)
	writeFiles(t, f.root, map[string]string{"pause.hurl": "GET {{base}}/slow\n[Options]\nmax-time: 300ms\nHTTP 200\n"})
	cs := f.connect(t, Config{})
	start := time.Now()
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, _ = cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "sonde_run", Arguments: map[string]any{"path": "pause.hurl"}})
		})
	}
	wg.Wait()
	if d := time.Since(start); d < 600*time.Millisecond {
		t.Fatalf("two 300ms runs took %s: not serialized", d)
	}
}

// TestRunSymlinks: files and project files reached through symbolic
// links out of the root are refused.
func TestRunSymlinks(t *testing.T) {
	f := newRunFixture(t)
	outside := tempRoot(t)
	writeFiles(t, outside, map[string]string{
		"x.hurl":     "GET {{base}}/\nHTTP 200\n",
		"sonde.yaml": "version: 1\nenvironments:\n  evil:\n    variables:\n      base: http://" + f.host + "\n",
	})
	if err := os.Symlink(filepath.Join(outside, "x.hurl"), filepath.Join(f.root, "link.hurl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(f.root, "proj")
	writeFiles(t, proj, map[string]string{"p.hurl": "GET {{base}}/\nHTTP 200\n"})
	if err := os.Symlink(filepath.Join(outside, "sonde.yaml"), filepath.Join(proj, "sonde.yaml")); err != nil {
		t.Fatal(err)
	}
	cs := f.connect(t, Config{})
	for _, args := range []map[string]any{
		{"path": "link.hurl"},
		{"path": "linkdir/x.hurl"},
		{"path": "proj/p.hurl", "env": "evil"},
		{"path": "proj/p.hurl"},
	} {
		res := call(t, cs, "sonde_run", args, nil)
		if !res.IsError || strings.Contains(resultText(res), "success") {
			t.Errorf("%v: %s", args, resultText(res))
		}
	}
	if !strings.Contains(f.log.String(), "not a regular file") {
		t.Errorf("audit log = %s", f.log.String())
	}
}

// TestRunAuditReason: the audit line says why a run failed.
func TestRunAuditReason(t *testing.T) {
	f := newRunFixture(t)
	cs := f.connect(t, Config{})
	call(t, cs, "sonde_run", map[string]any{"path": "proxy.hurl"}, nil)
	if !strings.Contains(f.log.String(), `failed at entry 1: Option not allowed: option "proxy"`) {
		t.Errorf("audit log = %s", f.log.String())
	}
}

func TestListCap(t *testing.T) {
	root := tempRoot(t)
	files := map[string]string{}
	for i := range maxListFiles + 5 {
		files[fmt.Sprintf("d%d/f%d.hurl", i%10, i)] = ""
	}
	writeFiles(t, root, files)
	cs := connect(t, Config{Root: root})
	var out listOutput
	call(t, cs, "sonde_list", nil, &out)
	if len(out.Files) != maxListFiles || !out.Truncated {
		t.Fatalf("files = %d, truncated = %v", len(out.Files), out.Truncated)
	}
}

func TestRecoverPanics(t *testing.T) {
	log := &syncBuffer{}
	h := recoverPanics(log)(func(context.Context, string, sdk.Request) (sdk.Result, error) {
		panic("boom")
	})
	res, err := h(context.Background(), "tools/call", nil)
	if res != nil || err == nil || !strings.Contains(log.String(), "internal error in tools/call: boom") {
		t.Fatalf("res=%v err=%v log=%s", res, err, log.String())
	}
}
