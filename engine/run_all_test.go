// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bytes"
	"context"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"slices"
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
	r.RunAll(context.Background(), nil, jobsOf(t, srv, "/ok", "/fail", "/ok", "/ok"), 2, Hooks{
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
	r.RunAll(context.Background(), nil, jobsOf(t, srv, "/ok", "/ok", "/ok"), 1, Hooks{
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
	r.RunAll(context.Background(), stop, jobsOf(t, srv, "/ok", "/ok"), 1, Hooks{
		Finished: func(int, Job, *UnitResult, error) bool { ran++; return true },
	})
	if ran != 0 {
		t.Errorf("ran %d jobs after stop, want 0", ran)
	}
}

func TestRunAllUnreadableJob(t *testing.T) {
	r := NewRunner(Options{})
	var gotErr error
	r.RunAll(context.Background(), nil, slices.Values([]Job{{Name: t.TempDir() + "/missing.hurl"}}), 1, Hooks{
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
	r.RunAll(context.Background(), nil, slices.Values([]Job{job}), 1, Hooks{
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
	r.RunAll(context.Background(), nil, jobs, 2, Hooks{
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

// stopAfterFirstEntry returns hooks closing stop when the first entry of a
// job finishes, and the results they receive.
func stopAfterFirstEntry(stop chan struct{}) (Hooks, *[]*UnitResult) {
	var results []*UnitResult
	return Hooks{
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
	NewRunner(Options{}).RunAll(context.Background(), stop, slices.Values([]Job{{Name: "a.hurl", Source: src}}), 1, hooks)
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
	NewRunner(Options{}).RunAll(context.Background(), stop, slices.Values([]Job{{Name: "a.hurl", Source: src}}), 1, hooks)
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
	r.RunAll(context.Background(), nil, slices.Values([]Job{{Name: "a.hurl", Source: src}}), 1, Hooks{
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
