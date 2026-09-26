// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"io"
	"iter"
	"os"
	"sync"

	"github.com/nhtera/sonde/internal/syntax"
)

// Job is a file to run: its Source, or the file Name read when Source is
// nil.
type Job struct {
	Name   string
	Source []byte
	// Variables and Secrets apply to this job only (for example from a
	// project file); the runner's Options take precedence over them.
	Variables map[string]any
	Secrets   map[string]string
}

// Hooks connect the jobs of RunAll to their output. Calls to the hooks and
// to the event handlers they return are never concurrent.
type Hooks struct {
	// Started is called when a job starts, seq being its 0-based position
	// in the sequence of jobs; it returns where the job sends its events
	// and its standard output (nil: nowhere).
	Started func(seq int, job Job) (onEvent func(Event), stdout io.Writer)
	// Finished receives the result of a job, in completion order, or the
	// error that prevented it from running (an unreadable file, a setup
	// failure). It returns false to schedule no more jobs.
	Finished func(seq int, job Job, res *UnitResult, err error) bool
}

// RunAll runs jobs, at most n at a time (n < 1 runs one at a time). Jobs
// are isolated: a failing job does not affect the others.
//
// Closing stop schedules no more jobs and ends running jobs at their next
// entry boundary (their results are Interrupted); canceling ctx also
// aborts requests in flight. A Finished hook returning false only stops
// the scheduling: running jobs complete.
func (r *Runner) RunAll(ctx context.Context, stop <-chan struct{}, jobs iter.Seq[Job], n int, h Hooks) {
	n = max(n, 1)
	var (
		mu     sync.Mutex // serializes the hooks and event handlers
		wg     sync.WaitGroup
		slots  = make(chan struct{}, n)
		halted = make(chan struct{}) // closed when Finished asks to stop
		once   sync.Once
	)
	// ended is closed by stop or by a hook asking to stop.
	ended := make(chan struct{})
	go func() {
		select {
		case <-stop:
		case <-halted:
		case <-ctx.Done():
		}
		close(ended)
	}()
	done := func() bool {
		select {
		case <-stop:
			return true
		case <-halted:
			return true
		case <-ctx.Done():
			return true
		default:
			return false
		}
	}

	seq := 0
	for job := range jobs {
		select {
		case slots <- struct{}{}:
		case <-ended:
		}
		if done() {
			break
		}
		mu.Lock()
		var onEvent func(Event)
		var stdout io.Writer
		if h.Started != nil {
			onEvent, stdout = h.Started(seq, job)
		}
		mu.Unlock()
		uio := unitIO{stop: stop, vars: job.Variables, secrets: job.Secrets}
		if stdout != nil {
			uio.stdout = lockedWriter{&mu, stdout}
		}
		if onEvent != nil {
			uio.onEvent = func(ev Event) {
				mu.Lock()
				defer mu.Unlock()
				onEvent(ev)
			}
		}
		wg.Add(1)
		go func(seq int, job Job) {
			defer wg.Done()
			defer func() { <-slots }()
			res, err := r.runJob(ctx, job, uio)
			mu.Lock()
			defer mu.Unlock()
			if h.Finished != nil && !h.Finished(seq, job, res, err) {
				once.Do(func() { close(halted) })
			}
		}(seq, job)
		seq++
	}
	wg.Wait()
	once.Do(func() { close(halted) }) // releases the watcher goroutine
	<-ended
}

func (r *Runner) runJob(ctx context.Context, job Job, uio unitIO) (*UnitResult, error) {
	src := job.Source
	if src == nil {
		var err error
		if src, err = readSource(job.Name); err != nil {
			return nil, err
		}
	}
	return r.runSource(ctx, job.Name, src, uio)
}

// readSource reads a file, stopping past the size the parser accepts (a
// larger file is then reported by the parser), so that a device or a huge
// file is not read whole.
func readSource(name string) ([]byte, error) {
	f, err := os.Open(name) //nolint:gosec // G304: the caller names the file
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	return io.ReadAll(io.LimitReader(f, syntax.MaxFileSize+1))
}

// lockedWriter writes under the RunAll lock.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
