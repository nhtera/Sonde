// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// slowServer answers /ok after a delay, tracking the peak number of
// requests in flight; /fail answers 500.
func slowServer(t *testing.T, delay time.Duration) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var inFlight, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(delay)
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = io.WriteString(w, r.URL.Path) //nolint:gosec // G705: test server echoing its own path
	}))
	t.Cleanup(srv.Close)
	return srv, &peak
}

func jobsOf(t *testing.T, srv *httptest.Server, paths ...string) iter.Seq[Job] {
	t.Helper()
	dir := t.TempDir()
	return func(yield func(Job) bool) {
		for i, p := range paths {
			job := Job{Name: dir + "/" + string(rune('a'+i)) + ".hurl", Source: []byte("GET " + srv.URL + p + "\n[Options]\noutput: -\nHTTP 200\n")}
			if !yield(job) {
				return
			}
		}
	}
}

func TestRunAllIsolationAndParallelism(t *testing.T) {
	srv, peak := slowServer(t, 50*time.Millisecond)
	r := NewRunner(Options{})
	var active, overlap atomic.Int32
	enter := func() {
		if active.Add(1) > 1 {
			overlap.Store(1)
		}
		time.Sleep(time.Millisecond)
		active.Add(-1)
	}
	var order []int
	success := map[int]bool{}
	outputs := map[int]*bytes.Buffer{}
	r.RunAll(context.Background(), jobsOf(t, srv, "/ok", "/fail", "/ok", "/ok"), RunAllOptions{
		Parallel: 2,
		Started: func(seq int, _ Job) (func(Event), io.Writer) {
			enter()
			outputs[seq] = &bytes.Buffer{}
			return func(Event) { enter() }, outputs[seq]
		},
		Finished: func(seq int, _ Job, res *UnitResult, err error) bool {
			enter()
			if err != nil {
				t.Errorf("job %d: %v", seq, err)
			}
			order = append(order, seq)
			success[seq] = res.Success
			return true
		},
	})
	slices.Sort(order)
	if !slices.Equal(order, []int{0, 1, 2, 3}) {
		t.Fatalf("finished jobs = %v", order)
	}
	if !success[0] || success[1] || !success[2] || !success[3] {
		t.Errorf("success = %v, want only job 1 failing", success)
	}
	if outputs[0].String() != "/ok" {
		t.Errorf("stdout of job 0 = %q", outputs[0])
	}
	if p := peak.Load(); p != 2 {
		t.Errorf("peak requests in flight = %d, want 2", p)
	}
	if overlap.Load() != 0 {
		t.Error("hooks or event handlers called concurrently")
	}
}

func TestRunAllFinishedStops(t *testing.T) {
	srv, _ := slowServer(t, 0)
	r := NewRunner(Options{})
	ran := 0
	r.RunAll(context.Background(), jobsOf(t, srv, "/ok", "/ok", "/ok"), RunAllOptions{
		Finished: func(int, Job, *UnitResult, error) bool { ran++; return false },
	})
	if ran != 1 {
		t.Errorf("ran %d jobs after the first asked to stop, want 1", ran)
	}
}

func TestRunAllStop(t *testing.T) {
	srv, _ := slowServer(t, 20*time.Millisecond)
	stop := make(chan struct{})
	close(stop)
	r := NewRunner(Options{})
	ran := 0
	r.RunAll(context.Background(), jobsOf(t, srv, "/ok", "/ok"), RunAllOptions{
		Stop:     stop,
		Finished: func(int, Job, *UnitResult, error) bool { ran++; return true },
	})
	if ran != 0 {
		t.Errorf("ran %d jobs after stop, want 0", ran)
	}
}

func TestRunAllUnreadableJob(t *testing.T) {
	r := NewRunner(Options{})
	var gotErr error
	r.RunAll(context.Background(), slices.Values([]Job{{Name: t.TempDir() + "/missing.hurl"}}), RunAllOptions{
		Finished: func(_ int, _ Job, _ *UnitResult, err error) bool { gotErr = err; return true },
	})
	if gotErr == nil {
		t.Error("no error for an unreadable job")
	}
}

// Job variables and secrets apply below the runner's options.
func TestRunAllJobVariables(t *testing.T) {
	srv, _ := slowServer(t, 0)
	r := NewRunner(Options{Variables: map[string]any{"path": "/ok"}})
	job := Job{
		Name:      t.TempDir() + "/a.hurl",
		Source:    []byte("GET " + srv.URL + "{{path}}\nHTTP 200\n[Asserts]\nbody == \"{{expect}}\"\n"),
		Variables: map[string]any{"path": "/fail", "expect": "/ok"},
		Secrets:   map[string]string{"token": "job-secret-5d2e"}, //nolint:gosec // G101: fake test value
	}
	var res *UnitResult
	r.RunAll(context.Background(), slices.Values([]Job{job}), RunAllOptions{
		Finished: func(_ int, _ Job, got *UnitResult, _ error) bool { res = got; return true },
	})
	if res == nil || !res.Success {
		t.Fatalf("result = %+v", res)
	}
	if r.Redact("job-secret-5d2e") == "job-secret-5d2e" {
		t.Error("job secret not registered")
	}
}

func TestRunAllFinishedFalseLetsRunningJobsComplete(t *testing.T) {
	srv, _ := slowServer(t, 50*time.Millisecond)
	two := []byte("GET " + srv.URL + "/ok\nHTTP 200\nGET " + srv.URL + "/ok\nHTTP 200\n")
	jobs := slices.Values([]Job{
		{Name: "slow.hurl", Source: two},
		{Name: "bad.hurl", Source: []byte("GET\n")},
		{Name: "never.hurl", Source: two},
	})
	r := NewRunner(Options{})
	got := map[string]*UnitResult{}
	r.RunAll(context.Background(), jobs, RunAllOptions{
		Parallel: 2,
		Finished: func(_ int, job Job, res *UnitResult, _ error) bool {
			got[job.Name] = res
			return res.ParseError == nil
		},
	})
	if got["bad.hurl"] == nil || got["bad.hurl"].ParseError == nil {
		t.Fatal("no parse error for bad.hurl")
	}
	if res := got["slow.hurl"]; res == nil || !res.Success || res.Interrupted || len(res.Entries) != 2 {
		t.Errorf("slow.hurl = %+v, want both entries run and a success", res)
	}
	if got["never.hurl"] != nil {
		t.Error("a job was scheduled after Finished returned false")
	}
}

// stopAfterFirstEntry returns options closing stop when the first entry
// of a job finishes, and the results they receive.
func stopAfterFirstEntry(stop chan struct{}) (RunAllOptions, *[]*UnitResult) {
	var results []*UnitResult
	return RunAllOptions{
		Stop: stop,
		Started: func(int, Job) (func(Event), io.Writer) {
			return func(ev Event) {
				if _, ok := ev.(EntryFinished); ok {
					select {
					case <-stop:
					default:
						close(stop)
					}
				}
			}, nil
		},
		Finished: func(_ int, _ Job, res *UnitResult, _ error) bool {
			results = append(results, res)
			return true
		},
	}, &results
}

func TestRunAllStopInterruptsRun(t *testing.T) {
	srv, _ := slowServer(t, 0)
	stop := make(chan struct{})
	hooks, results := stopAfterFirstEntry(stop)
	src := []byte("GET " + srv.URL + "/ok\nHTTP 200\nGET " + srv.URL + "/fail\nHTTP 200\n")
	NewRunner(Options{}).RunAll(context.Background(), slices.Values([]Job{{Name: "a.hurl", Source: src}}), hooks)
	if len(*results) != 1 {
		t.Fatalf("got %d results", len(*results))
	}
	res := (*results)[0]
	if !res.Interrupted || res.Success || len(res.Entries) != 1 {
		t.Errorf("interrupted=%v success=%v entries=%d, want an interrupted failure after 1 entry",
			res.Interrupted, res.Success, len(res.Entries))
	}
}

func TestRunAllStopDuringRetryInterval(t *testing.T) {
	srv, _ := slowServer(t, 0)
	stop := make(chan struct{})
	hooks, results := stopAfterFirstEntry(stop)
	src := []byte("GET " + srv.URL + "/fail\n[Options]\nretry: 3\nretry-interval: 10s\nHTTP 200\n")
	start := time.Now()
	NewRunner(Options{}).RunAll(context.Background(), slices.Values([]Job{{Name: "a.hurl", Source: src}}), hooks)
	if time.Since(start) > 5*time.Second {
		t.Error("stop did not end the retry interval")
	}
	res := (*results)[0]
	if res.Success || len(res.Entries) != 1 || res.Entries[0].Retried {
		t.Errorf("success=%v entries=%d, want the unretried error to stand", res.Success, len(res.Entries))
	}
}

func TestEntryStartedLast(t *testing.T) {
	srv, _ := slowServer(t, 0)
	src := []byte("GET " + srv.URL + "/ok\nGET " + srv.URL + "/ok\nGET " + srv.URL + "/ok\n")
	var lasts []int
	r := NewRunner(Options{ToEntry: 2})
	r.RunAll(context.Background(), slices.Values([]Job{{Name: "a.hurl", Source: src}}), RunAllOptions{
		Started: func(int, Job) (func(Event), io.Writer) {
			return func(ev Event) {
				if es, ok := ev.(EntryStarted); ok {
					lasts = append(lasts, es.Last)
				}
			}, nil
		},
	})
	if !slices.Equal(lasts, []int{2, 2}) {
		t.Errorf("EntryStarted.Last = %v, want [2 2]", lasts)
	}
}

func TestRunAllRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Query().Get("u")+" "+r.Header.Get("X-Token")) //nolint:gosec // G705: test server echoing its input
	}))
	t.Cleanup(srv.Close)
	src := []byte("GET " + srv.URL + "?u={{user}}\nX-Token: {{token}}\nHTTP 200\n" +
		"[Captures]\nsession: body redact\n[Asserts]\nbody == \"{{expect}}\"\n")
	job := Job{Name: "a.hurl", Source: src, Row: &Row{
		Index:     3,
		Variables: map[string]any{"user": "bob", "expect": "nobody"},
		Secrets:   map[string]string{"token": "row-token-1"},
	}}
	r := NewRunner(Options{Variables: map[string]any{"user": "alice", "expect": "bob row-token-1"}, Verbosity: Verbose})
	var logs strings.Builder
	var res *UnitResult
	r.RunAll(context.Background(), slices.Values([]Job{job}), RunAllOptions{
		Started: func(int, Job) (func(Event), io.Writer) {
			return func(ev Event) {
				if l, ok := ev.(Log); ok {
					logs.WriteString(l.Text + "\n")
				}
			}, nil
		},
		Finished: func(_ int, _ Job, got *UnitResult, _ error) bool { res = got; return true },
	})
	if res.Label() != "a.hurl#row-3" || res.Row != 3 {
		t.Errorf("label = %q", res.Label())
	}
	// The row's "expect" wins over the runner's, so the assert fails and
	// its message holds the actual body.
	if res.Success {
		t.Fatal("row variable did not override the runner's variable")
	}
	for _, secret := range []string{"row-token-1", "bob row-token-1"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("events leak %q", secret)
		}
	}
	actual := res.Entries[0].Errors[0].Actual()
	if got := res.Redact(actual); strings.Contains(got, "row-token-1") {
		t.Errorf("Redact(%q) = %q", actual, got)
	}
	if r.HasSecrets() || r.Redact("row-token-1") != "row-token-1" {
		t.Error("row secrets leaked into the run's secrets")
	}
	if (&UnitResult{File: "b.hurl"}).Label() != "b.hurl" {
		t.Error("label of a result without a row")
	}
}

// TestRunAllRowCredentialsStayInRow checks that Basic credentials built
// from a run secret and a row value are the row's secret, not the run's.
func TestRunAllRowCredentialsStayInRow(t *testing.T) {
	srv, _ := slowServer(t, 0)
	src := []byte("GET " + srv.URL + "/ok\n[BasicAuth]\n{{user}}: {{pw}}\nHTTP 200\n")
	var jobs []Job
	for i := range 3 {
		jobs = append(jobs, Job{Name: "a.hurl", Source: src, Row: &Row{Index: i + 1, Variables: map[string]any{"user": fmt.Sprint("user-", i)}}})
	}
	r := NewRunner(Options{Secrets: map[string]string{"pw": "run-password"}})
	var results []*UnitResult
	r.RunAll(context.Background(), slices.Values(jobs), RunAllOptions{
		Finished: func(_ int, _ Job, res *UnitResult, _ error) bool { results = append(results, res); return true },
	})
	encoded := base64.StdEncoding.EncodeToString([]byte("user-1:run-password"))
	if r.Redact(encoded) != encoded {
		t.Error("a row's credentials were added to the run's secrets")
	}
	if got := results[1].Redact("Basic " + encoded); got != "Basic ***" {
		t.Errorf("row result Redact = %q", got)
	}
}

// TestUnitResultRedactOverlap checks that a run secret containing a row
// secret is masked whole.
func TestUnitResultRedactOverlap(t *testing.T) {
	srv, _ := slowServer(t, 0)
	src := []byte("GET " + srv.URL + "/ok\nHTTP 200\n")
	r := NewRunner(Options{Secrets: map[string]string{"a": "xxxxYYYYzzzz"}})
	var res *UnitResult
	r.RunAll(context.Background(), slices.Values([]Job{{Name: "a.hurl", Source: src, Row: &Row{Index: 1, Secrets: map[string]string{"b": "YYYY"}}}}), RunAllOptions{
		Finished: func(_ int, _ Job, got *UnitResult, _ error) bool { res = got; return true },
	})
	if got := res.Redact("k=xxxxYYYYzzzz"); got != "k=***" {
		t.Errorf("Redact = %q", got)
	}
}

func TestRowVariablesJSONShapes(t *testing.T) {
	srv, _ := slowServer(t, 0)
	src := []byte("GET " + srv.URL + "/ok\nHTTP 200\n[Asserts]\nvariable \"o\" isCollection\n")
	r := NewRunner(Options{})
	var res *UnitResult
	row := &Row{Index: 1, Variables: map[string]any{"o": map[string]any{"k": []any{1, int64(2)}}}}
	r.RunAll(context.Background(), slices.Values([]Job{{Name: "a.hurl", Source: src, Row: row}}), RunAllOptions{
		Finished: func(_ int, _ Job, got *UnitResult, err error) bool {
			if err != nil {
				t.Fatal(err)
			}
			res = got
			return true
		},
	})
	if !res.Success {
		t.Errorf("object row variable: %v", res.Errors())
	}
}
