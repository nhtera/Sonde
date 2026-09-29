// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/copyas"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/fixture"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
	"github.com/nhtera/sonde/desktop/internal/view"
)

// shop is the shop-api project and its fixture server, wired as the app.
type shop struct {
	h    *Host
	rec  *emit.Recorder
	api  *fixture.Server
	url  string
	dir  string
	data string // app data folder
}

// newShop copies testdata/shop-api into a temporary project, starts
// shop-api and wires the app over them (base_url overridden to it).
func newShop(t *testing.T) *shop {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join("testdata", "shop-api")
	err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if fi.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o750)
		}
		data, err := os.ReadFile(p) //nolint:gosec // test data
		if err != nil {
			return err
		}
		perm := os.FileMode(0o600)
		return os.WriteFile(filepath.Join(dir, rel), data, perm) //nolint:gosec // G703: copying test data
	})
	if err != nil {
		t.Fatal(err)
	}
	api := fixture.New()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	data := t.TempDir()
	dirs, err := appdirs.Open(filepath.Join(data, "config"), filepath.Join(data, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dirs.Close() })
	t.Setenv("HOME", t.TempDir()) // no user CLI config file
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	rec := &emit.Recorder{}
	h := &Host{Mode: ModeServer, Root: dir, Dirs: dirs, Emit: rec}
	if err := h.setup(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Workspace.Close)
	if err := h.Envs.SetOverride("local", "base_url", srv.URL); err != nil {
		t.Fatal(err)
	}
	return &shop{h: h, rec: rec, api: api, url: srv.URL, dir: dir, data: data}
}

func (s *shop) source(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, file))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (s *shop) run(t *testing.T, id, file string) *runsvc.Summary {
	t.Helper()
	sum, err := s.h.Runs.Run(context.Background(), runsvc.RunRequest{RunID: id, File: file, Source: s.source(t, file), Env: "local"})
	if err != nil {
		t.Fatalf("run %s: %v", file, err)
	}
	return sum
}

// entries returns the finished entries of run id, by unit.
func (s *shop) entries(t *testing.T, id string) []view.Entry {
	t.Helper()
	var out []view.Entry
	for _, ev := range s.rec.Events() {
		b, ok := ev.Data.(runsvc.Batch)
		if !ok || b.RunID != id {
			continue
		}
		for _, it := range b.Items {
			if e, ok := finished(t, it.Event); ok {
				out = append(out, e)
			}
		}
	}
	return out
}

// TestCheckoutFirstFailure: the checkout's first failure is its assert on
// $.status (the order stays pending).
func TestCheckoutFirstFailure(t *testing.T) {
	s := newShop(t)
	sum := s.run(t, "r1", "checkout.hurl")
	if sum.Outcome != runsvc.Failed || sum.Requests != 5 {
		t.Fatalf("summary %+v", sum)
	}
	var first *view.Entry
	entries := s.entries(t, "r1")
	for i := range entries {
		if !entries[i].Success {
			first = &entries[i]
			break
		}
	}
	line := 1 + slices.Index(strings.Split(s.source(t, "checkout.hurl"), "\n"), `jsonpath "$.status" == "paid"`)
	if first == nil || first.Index != 5 || len(first.Errors) != 1 || !first.Errors[0].Assert || first.Errors[0].Line != line {
		t.Fatalf("first failure (want line %d) %+v", line, first)
	}
}

// TestSendReusesTokenAndCookie: Send(5) after a run is authorized (the
// token capture and the session cookie), with keep cookies off; a second
// Send keeps the first's captures; another environment is refused.
func TestSendReusesTokenAndCookie(t *testing.T) {
	s := newShop(t)
	s.run(t, "r1", "checkout.hurl")
	src := s.source(t, "checkout.hurl")
	send := func(id string, n int, env string) (*runsvc.Summary, error) {
		return s.h.Runs.Send(context.Background(), runsvc.SendRequest{RunID: id, File: "checkout.hurl", Source: src, Env: env, Entry: n})
	}
	if _, err := send("s1", 5, "local"); err != nil {
		t.Fatal(err)
	}
	e := s.entries(t, "s1")
	if len(e) != 1 || e[0].Calls[0].Response.Status != 200 {
		t.Fatalf("send 5: %+v", e)
	}
	if _, err := send("s2", 3, "local"); err != nil { // a new cart: c2
		t.Fatal(err)
	}
	if _, err := send("s3", 5, "local"); err != nil {
		t.Fatal(err)
	}
	reqs := s.api.Requests()
	if reqs[len(reqs)-1] != "POST /carts/c2/checkout" {
		t.Errorf("the second send's capture was not kept: %v", reqs[len(reqs)-3:])
	}
	var ae *apperr.Error
	if _, err := send("s4", 5, "staging"); !errors.As(err, &ae) || ae.Code != apperr.Stale {
		t.Errorf("another env: %v", err)
	}
}

// TestNoSecretReachesThePage is the sentinel: the password (a project
// secret and a data secret), the token (a redact capture) and, in the
// history, the session cookie, never reach anything the page gets: events,
// bodies, the history, Copy as without reveal, the lists.
func TestNoSecretReachesThePage(t *testing.T) {
	s := newShop(t)
	for _, f := range []string{"checkout.hurl", "echoes.hurl", "users.hurl", "events.sonde", "ws.sonde"} {
		s.run(t, "run-"+f, f)
	}
	csv := filepath.Join(s.dir, "data", "logins.csv")
	hd, _ := s.h.Handles.Put(csv, handles.OpenFile)
	if _, err := s.h.Runs.RunData(context.Background(), runsvc.DataRequest{RunID: "data", File: "data-login.hurl",
		Source: s.source(t, "data-login.hurl"), Env: "local", DataHandle: hd, Secrets: []string{"pass"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.h.Runs.Send(context.Background(), runsvc.SendRequest{RunID: "send", File: "checkout.hurl",
		Source: s.source(t, "checkout.hurl"), Env: "local", Entry: 2}); err != nil {
		t.Fatal(err)
	}
	secrets := []string{fixture.Password, fixture.Token}

	events := s.rec.Events()
	if len(events) < 20 {
		t.Fatalf("only %d events", len(events))
	}
	for _, ev := range events {
		redactcheck.AssertNoSecret(t, "event "+ev.Topic, ev.Data, secrets...)
	}
	var bodies int
	for _, ev := range events {
		b, ok := ev.Data.(runsvc.Batch)
		if !ok {
			continue
		}
		for _, it := range b.Items {
			e, ok := finished(t, it.Event)
			if !ok {
				continue
			}
			for _, body := range e.Bodies {
				if body.ID == "" {
					continue
				}
				data, _, ok := s.h.Bodies.Redacted(body.ID)
				if !ok {
					t.Fatalf("body %s missing", body.ID)
				}
				redactcheck.AssertNoSecretBytes(t, "body", data, secrets...)
				bodies++
			}
		}
	}
	if bodies < 5 {
		t.Errorf("only %d bodies checked", bodies)
	}

	items, err := s.h.History.List()
	if err != nil || len(items) < 6 {
		t.Fatalf("history %d %v", len(items), err)
	}
	for _, it := range items {
		rec, err := s.h.History.Get(it.ID)
		if err != nil {
			t.Fatal(err)
		}
		redactcheck.AssertNoSecret(t, "history", rec, append(secrets, fixture.Session)...)
	}

	c := copyas.New(s.h.Runs, func() string { return s.dir })
	curl, err := c.Curl(context.Background(), copyas.Request{File: "checkout.hurl", Source: s.source(t, "checkout.hurl"), Env: "local"}, false)
	if err != nil {
		t.Fatal(err)
	}
	redactcheck.AssertNoSecret(t, "copy as curl", curl, secrets...)
	redactcheck.AssertNoSecret(t, "lists", mustList(t, s), secrets...)
}

// finished decodes an entryFinished event.
func finished(t *testing.T, raw json.RawMessage) (view.Entry, bool) {
	t.Helper()
	var head struct{ Type string }
	if err := json.Unmarshal(raw, &head); err != nil {
		t.Fatal(err)
	}
	if head.Type != view.TypeEntryFinished {
		return view.Entry{}, false
	}
	var e struct{ Entry view.Entry }
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	return e.Entry, true
}

func mustList(t *testing.T, s *shop) any {
	t.Helper()
	envs, err := s.h.Envs.List()
	if err != nil {
		t.Fatal(err)
	}
	jars, err := s.h.Jars.List()
	if err != nil {
		t.Fatal(err)
	}
	return []any{envs, jars, s.h.Envs.Overrides()}
}

// TestCopyAsSondeReproducesTheRun runs the copied `sonde run` command
// with the CLI in the project folder: it makes the requests the app's run
// made, and fails the same way.
func TestCopyAsSondeReproducesTheRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the POSIX command")
	}
	// The CLI, built before the test's own HOME hides the Go caches.
	bin := filepath.Join(t.TempDir(), "sonde")
	build := exec.Command("go", "build", "-o", bin, "./cmd/sonde")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sonde: %v\n%s", err, out)
	}
	s := newShop(t)
	s.run(t, "r1", "checkout.hurl")
	app := s.api.Requests()
	c := copyas.New(s.h.Runs, func() string { return s.dir })
	cmd, err := c.Sonde(copyas.Request{File: "checkout.hurl", Env: "local"}, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := exec.Command("sh", "-c", cmd.Text) //nolint:gosec // the command under test
	sh.Dir = s.dir
	sh.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := sh.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 4 {
		t.Fatalf("the CLI run must fail its assert (exit 4): %v\n%s\n%s", err, cmd.Text, out)
	}
	// Carts are numbered by the server: the CLI's is the next one.
	carts := regexp.MustCompile(`/carts/c[0-9]+`)
	norm := func(reqs []string) []string {
		out := make([]string, len(reqs))
		for i, r := range reqs {
			out[i] = carts.ReplaceAllString(r, "/carts/c#")
		}
		return out
	}
	cli := norm(s.api.Requests()[len(app):])
	if app = norm(app); !slices.Equal(cli, app) {
		t.Errorf("the CLI made %v, the app %v", cli, app)
	}
}
