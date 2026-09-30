// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runsvc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/desktop/internal/view"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/cookiejar"
	"github.com/nhtera/sonde/internal/sandbox"
)

const token = "tok-sentinel-8841" //nolint:gosec // G101: test sentinel

// shop is a small API: login returns a token and a session cookie; the
// other routes echo what they received.
type shop struct {
	mu   sync.Mutex
	seen []string // "METHOD path auth=<Authorization> cookie=<Cookie>"
	slow chan struct{}
}

func (s *shop) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.seen = append(s.seen, r.Method+" "+r.URL.Path+" auth="+r.Header.Get("Authorization")+" cookie="+r.Header.Get("Cookie"))
	s.mu.Unlock()
	switch r.URL.Path {
	case "/login":
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "s-1", Path: "/"}) //nolint:gosec // test cookie
		_, _ = io.WriteString(w, `{"token":"`+token+`","user":7}`)
	case "/slow":
		<-s.slow
	default:
		reply := `{"ok":true,"auth":"` + r.Header.Get("Authorization") + `"}`
		_, _ = io.WriteString(w, reply) //nolint:gosec // G705: a test echo server
	}
}

func (s *shop) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

const flow = `POST {{base}}/login
HTTP 200
[Captures]
tok: jsonpath "$.token" redact
user: jsonpath "$.user"

GET {{base}}/users/{{user}}
Authorization: Bearer {{tok}}
HTTP 200

GET {{base}}/carts
Authorization: Bearer {{tok}}
HTTP 200
`

type fixture struct {
	runs *Runs
	rec  *emit.Recorder
	shop *shop
	dir  string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	sh := &shop{slow: make(chan struct{})}
	srv := httptest.NewServer(sh)
	t.Cleanup(func() { close(sh.slow); srv.Close() })
	dir := t.TempDir()
	yaml := "version: 1\nenvironments:\n  local:\n    variables:\n      base: " + srv.URL + "\n  other:\n    variables:\n      base: " + srv.URL + "\n"
	for name, text := range map[string]string{"sonde.yaml": yaml, "flow.hurl": flow, "slow.hurl": "GET {{base}}/slow\nHTTP 200\n", "bad.hurl": "GET\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	env := config.Env{"HOME": home, "XDG_CONFIG_HOME": home}
	rec := &emit.Recorder{}
	r := New(rec, func() *sandbox.Root { return root }, env, "test", &memBodies{m: map[string][]byte{}}, handles.New())
	return &fixture{runs: r, rec: rec, shop: sh, dir: dir}
}

type memBodies struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memBodies) Put(data, _ []byte, _ string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := strconv.Itoa(len(b.m))
	b.m[id] = data
	return id
}

func (f *fixture) run(t *testing.T, id string) *Summary {
	t.Helper()
	s, err := f.runs.Run(context.Background(), RunRequest{RunID: id, File: "flow.hurl", Source: flow, Env: "local"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return s
}

// events decodes the run's batches in order.
func (f *fixture) events(t *testing.T, runID string) (items []Item, done *Done) {
	t.Helper()
	for _, ev := range f.rec.Events() {
		switch d := ev.Data.(type) {
		case Batch:
			if ev.Topic == topic(runID) {
				items = append(items, d.Items...)
			}
		case Done:
			if ev.Topic == doneTopic(runID) {
				if done != nil {
					t.Error("two Done events")
				}
				dd := d
				done = &dd
			}
		}
	}
	return items, done
}

func TestRunBatchesThenDone(t *testing.T) {
	f := setup(t)
	s := f.run(t, "r1")
	if s.Outcome != Passed || s.Files != 1 || s.Requests != 3 {
		t.Fatalf("summary %+v", s)
	}
	items, done := f.events(t, "r1")
	if done == nil || len(items) == 0 {
		t.Fatalf("items %d, done %v", len(items), done)
	}
	for i, it := range items {
		if it.Seq != int64(i+1) {
			t.Fatalf("item %d has seq %d", i, it.Seq)
		}
	}
	if done.LastSeq != items[len(items)-1].Seq || done.Summary.Outcome != Passed {
		t.Errorf("done %+v", done)
	}
	events := f.rec.Events()
	if last := events[len(events)-1]; last.Topic != doneTopic("r1") {
		t.Errorf("last event %s, want Done after every batch", last.Topic)
	}
	var finished int
	for _, it := range items {
		var e struct{ Type string }
		_ = json.Unmarshal(it.Event, &e)
		if e.Type == view.TypeEntryFinished {
			finished++
		}
	}
	if finished != 3 {
		t.Errorf("%d entryFinished events, want 3", finished)
	}
}

// TestSendParity: Send(3) after a run sends what the full run sent: the
// captured token and the session cookie, with keep cookies off.
func TestSendParity(t *testing.T) {
	f := setup(t)
	f.run(t, "r1")
	full := f.shop.requests()
	s, err := f.runs.Send(context.Background(), SendRequest{RunID: "s1", File: "flow.hurl", Source: flow, Env: "local", Entry: 3})
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != "send" || s.Outcome != Passed || s.Requests != 1 || s.BaseRunAt == nil {
		t.Fatalf("send %+v", s)
	}
	all := f.shop.requests()
	if len(all) != len(full)+1 {
		t.Fatalf("send made %d requests", len(all)-len(full))
	}
	if got, want := all[len(all)-1], full[2]; got != want {
		t.Errorf("send sent %q, the full run %q", got, want)
	}
	if !strings.Contains(all[len(all)-1], "cookie=sid=s-1") || !strings.Contains(all[len(all)-1], token) {
		t.Errorf("send lacks the session cookie or token: %q", all[len(all)-1])
	}
	// A second Send keeps the captures of entry 1.
	if _, err := f.runs.Send(context.Background(), SendRequest{RunID: "s2", File: "flow.hurl", Source: flow, Env: "local", Entry: 2}); err != nil {
		t.Fatal(err)
	}
	if got := f.shop.requests(); !strings.Contains(got[len(got)-1], "/users/7") {
		t.Errorf("second send %q", got[len(got)-1])
	}
	items, _ := f.events(t, "s1")
	for _, it := range items {
		redactcheck.AssertNoSecret(t, "send event", it.Event, token)
	}
}

func TestSendWithoutRunRunsFirstEntries(t *testing.T) {
	f := setup(t)
	s, err := f.runs.Send(context.Background(), SendRequest{RunID: "s1", File: "flow.hurl", Source: flow, Env: "local", Entry: 2})
	if err != nil {
		t.Fatal(err)
	}
	if s.Requests != 2 || s.BaseRunAt != nil || len(f.shop.requests()) != 2 {
		t.Fatalf("send %+v, %d requests", s, len(f.shop.requests()))
	}
	// That run is a base for the next Send.
	if s, err := f.runs.Send(context.Background(), SendRequest{RunID: "s2", File: "flow.hurl", Source: flow, Env: "local", Entry: 2}); err != nil || s.BaseRunAt == nil {
		t.Fatalf("second send %+v %v", s, err)
	}
}

func TestSendRefusesAnotherVersion(t *testing.T) {
	f := setup(t)
	f.run(t, "r1")
	edited := strings.Replace(flow, "POST {{base}}/login", "POST {{base}}/login?v=2", 1)
	sum, err := f.runs.Send(context.Background(), SendRequest{RunID: "s1", File: "flow.hurl", Source: edited, Env: "local", Entry: 3})
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != apperr.Stale {
		t.Errorf("edited entry 1: %v", err)
	}
	// The summary the page receives (with Done) says why, coded.
	if sum == nil || sum.ErrorCode != apperr.Stale || sum.Outcome != Errored {
		t.Errorf("summary %+v, want the stale code", sum)
	}
	_, err = f.runs.Send(context.Background(), SendRequest{RunID: "s2", File: "flow.hurl", Source: flow, Env: "other", Entry: 3})
	if !errors.As(err, &e) || e.Code != apperr.Stale {
		t.Errorf("other env: %v", err)
	}
	// Editing the sent entry itself is what Send is for.
	tweaked := strings.Replace(flow, "GET {{base}}/carts", "GET {{base}}/carts?page=2", 1)
	if _, err := f.runs.Send(context.Background(), SendRequest{RunID: "s3", File: "flow.hurl", Source: tweaked, Env: "local", Entry: 3}); err != nil {
		t.Errorf("edited sent entry: %v", err)
	}
	if _, done := f.events(t, "s1"); done == nil || done.Summary.Outcome != Errored {
		t.Errorf("a refused Send still ends with Done: %+v", done)
	}
}

func TestCanceledRunIsNotStoredAndBusy(t *testing.T) {
	f := setup(t)
	src := "GET {{base}}/login\nHTTP 200\n\nGET {{base}}/slow\nHTTP 200\n"
	errc := make(chan error, 1)
	var sum *Summary
	go func() {
		s, err := f.runs.Run(context.Background(), RunRequest{RunID: "r1", File: "slow.hurl", Source: src, Env: "local"})
		sum = s
		errc <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for len(f.shop.requests()) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("slow request never arrived")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, err := f.runs.Run(context.Background(), RunRequest{RunID: "r2", File: "slow.hurl", Source: src, Env: "local"})
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != apperr.Busy {
		t.Errorf("second run of a busy file: %v", err)
	}
	f.runs.Cancel("r1")
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if sum.Outcome != Canceled {
		t.Errorf("canceled run outcome %q", sum.Outcome)
	}
	if f.runs.session("slow.hurl", 0) != nil {
		t.Error("a canceled run was stored for Send")
	}
}

func TestRunTest(t *testing.T) {
	f := setup(t)
	s, err := f.runs.RunTest(context.Background(), TestRequest{RunID: "t1", Files: []string{"flow.hurl", "bad.hurl"}, Env: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != "test" || s.Files != 2 || s.Succeeded != 1 || s.Outcome != Errored {
		t.Fatalf("test %+v", s)
	}
	var bad *Unit
	for i := range s.Units {
		if s.Units[i].File == "bad.hurl" {
			bad = &s.Units[i]
		}
	}
	if bad == nil || bad.ParseError == nil || bad.ParseError.Line != 1 {
		t.Errorf("bad.hurl unit %+v", bad)
	}
	if _, err := f.runs.RunTest(context.Background(), TestRequest{RunID: "t2", Files: []string{"../x.hurl"}}); err == nil {
		t.Error("a path outside the project ran")
	}
}

func TestRunData(t *testing.T) {
	f := setup(t)
	csv := filepath.Join(t.TempDir(), "rows.csv")
	if err := os.WriteFile(csv, []byte("id\n1\n2\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, _ := f.runs.handles.Put(csv, handles.OpenFile)
	src := "GET {{base}}/items/{{id}}\nHTTP 200\n"
	s, err := f.runs.RunData(context.Background(), DataRequest{RunID: "d1", File: "slow.hurl", Source: src, Env: "local", DataHandle: h, Rows: []int{1, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Files != 2 || s.Outcome != Passed {
		t.Fatalf("data %+v", s)
	}
	got := f.shop.requests()
	if len(got) != 2 || !strings.Contains(got[0], "/items/1") || !strings.Contains(got[1], "/items/3") {
		t.Errorf("rows run: %v", got)
	}
	if _, err := f.runs.RunData(context.Background(), DataRequest{RunID: "d2", File: "slow.hurl", Source: src, DataHandle: h}); err == nil {
		t.Error("a used data handle ran again")
	}
}

func TestPlanningErrorEndsRun(t *testing.T) {
	f := setup(t)
	_, err := f.runs.Run(context.Background(), RunRequest{RunID: "r1", File: "flow.hurl", Source: flow, Env: "nope"})
	if err == nil {
		t.Fatal("unknown env ran")
	}
	if _, done := f.events(t, "r1"); done == nil || done.Summary.Error == "" {
		t.Errorf("done %+v", done)
	}
}

// TestKeepCookies: with keep cookies on, a full run starts with the jar
// the previous one kept.
func TestKeepCookies(t *testing.T) {
	f := setup(t)
	jar := filepath.Join(t.TempDir(), "jar.txt")
	f.runs.Hooks.KeptJar = func(string) string {
		if _, err := os.Stat(jar); err != nil {
			return ""
		}
		return jar
	}
	f.runs.Hooks.KeepCookies = func(file string, cookies []engine.Cookie) {
		_ = os.WriteFile(jar, cookiejar.Format(file, cookies, func(s string) string { return s }), 0o600)
	}
	src := "POST {{base}}/login\nHTTP 200\n"
	if _, err := f.runs.Run(context.Background(), RunRequest{RunID: "r1", File: "flow.hurl", Source: src, Env: "local"}); err != nil {
		t.Fatal(err)
	}
	src2 := "GET {{base}}/me\nHTTP 200\n"
	if _, err := f.runs.Run(context.Background(), RunRequest{RunID: "r2", File: "flow.hurl", Source: src2, Env: "local"}); err != nil {
		t.Fatal(err)
	}
	got := f.shop.requests()
	if !strings.Contains(got[len(got)-1], "cookie=sid=s-1") {
		t.Errorf("second run did not send the kept cookie: %q", got[len(got)-1])
	}
}

// TestSendNeedsEarlierEntries: a run that stopped at entry 1 does not
// back a Send of entry 3 (entry 2 never ran); an edited sonde.yaml does
// not match either.
func TestSendNeedsEarlierEntries(t *testing.T) {
	f := setup(t)
	if _, err := f.runs.Run(context.Background(), RunRequest{RunID: "r1", File: "flow.hurl", Source: flow, Env: "local", To: 1}); err != nil {
		t.Fatal(err)
	}
	var e *apperr.Error
	_, err := f.runs.Send(context.Background(), SendRequest{RunID: "s1", File: "flow.hurl", Source: flow, Env: "local", Entry: 3})
	if !errors.As(err, &e) || e.Code != apperr.Stale {
		t.Errorf("send 3 after a run of 1: %v", err)
	}
	if _, err := f.runs.Send(context.Background(), SendRequest{RunID: "s2", File: "flow.hurl", Source: flow, Env: "local", Entry: 2}); err != nil {
		t.Errorf("send 2 after a run of 1: %v", err)
	}
	f.run(t, "r2")
	y := filepath.Join(f.dir, "sonde.yaml")
	data, _ := os.ReadFile(y)
	edited := strings.Replace(string(data), "  local:\n    variables:\n", "  local:\n    variables:\n      extra: 1\n", 1)
	if err := os.WriteFile(y, []byte(edited), 0o600); err != nil { //nolint:gosec // G703: the test's project
		t.Fatal(err)
	}
	_, err = f.runs.Send(context.Background(), SendRequest{RunID: "s3", File: "flow.hurl", Source: flow, Env: "local", Entry: 3})
	if !errors.As(err, &e) || e.Code != apperr.Stale {
		t.Errorf("send after sonde.yaml changed: %v", err)
	}
}
