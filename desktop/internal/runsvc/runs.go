// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package runsvc runs request files for the page: a whole file, one entry
// again (Send), a test run of many files and a data-driven run. Every run
// is planned by internal/runplan from an invocation, the app's own
// environment and the CLI's user config file, the way `sonde run` plans
// it; the page's buffer is the source. Engine events cross to the page
// only as view DTOs, in sequenced batches on "run:<runId>", then Done.
package runsvc

import (
	"context"
	"errors"
	"io"
	"iter"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/view"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Hooks connect runs to the app's other services; each may be nil.
type Hooks struct {
	// Extend adds the app's settings and session overrides to an
	// invocation; Overrides returns a digest of them (sessions are keyed
	// on it).
	Extend    func(inv *runplan.Invocation)
	Overrides func() string
	// KeptCookies returns the jar kept for file between full runs (nil:
	// keep cookies is off); KeepCookies stores the jar after one.
	KeptCookies func(file string) []engine.Cookie
	KeepCookies func(file string, cookies []engine.Cookie)
	// Record stores a finished run in the history.
	Record func(s *Summary, results []*engine.UnitResult)
}

// Runs is the run service's core.
type Runs struct {
	emit    emit.Emitter
	project func() *sandbox.Root
	env     config.Env
	version string
	bodies  view.BodyStore
	handles *handles.Table
	Hooks   Hooks

	mu       sync.Mutex
	busy     map[string]string // file -> runId
	cancels  map[string]context.CancelFunc
	sessions map[string]*session // file (#row) -> last full run
}

// New returns the run service. env is the app's process environment;
// version names the default User-Agent.
func New(e emit.Emitter, project func() *sandbox.Root, env config.Env, version string, bodies view.BodyStore, h *handles.Table) *Runs {
	return &Runs{
		emit: e, project: project, env: env, version: version, bodies: bodies, handles: h,
		busy: map[string]string{}, cancels: map[string]context.CancelFunc{}, sessions: map[string]*session{},
	}
}

// Cancel stops run runID: requests in flight are aborted.
func (r *Runs) Cancel(runID string) {
	r.mu.Lock()
	cancel := r.cancels[runID]
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Run runs a file (entries 1…To).
func (r *Runs) Run(ctx context.Context, req RunRequest) (*Summary, error) {
	return r.start(ctx, req.RunID, "run", []string{req.File}, func(ctx context.Context, rn *run) error {
		if err := rn.plan(ctx, "run", req.Env, "", map[string]string{req.File: req.Source}); err != nil {
			return err
		}
		rn.opts.ToEntry = req.To
		src := []byte(req.Source)
		rn.execute(ctx, func(res *engine.UnitResult) {
			if !res.Interrupted && !canceled(res) {
				r.storeSession(req.File, src, req.Env, res)
			}
		})
		return nil
	})
}

// Send runs entry n alone, reusing the captures and cookies of the file's
// last full run. With no such run it runs entries 1…n; a run of another
// version of entries 1…n-1, environment or overrides is refused.
func (r *Runs) Send(ctx context.Context, req SendRequest) (*Summary, error) {
	src := []byte(req.Source)
	if _, ok := prefix(src, req.Entry); !ok {
		return nil, apperr.New(apperr.Invalid, "there is no request "+strconv.Itoa(req.Entry))
	}
	return r.start(ctx, req.RunID, "send", []string{req.File}, func(ctx context.Context, rn *run) error {
		if err := rn.plan(ctx, "run", req.Env, "", map[string]string{req.File: req.Source}); err != nil {
			return err
		}
		sess := r.session(req.File, 0)
		overrides := r.overrides()
		if sess == nil {
			// No run to reuse: run 1…n, which then is one.
			rn.opts.ToEntry = req.Entry
			rn.execute(ctx, func(res *engine.UnitResult) {
				if !res.Interrupted && !canceled(res) {
					r.storeSession(req.File, src, req.Env, res)
				}
			})
			return nil
		}
		if !sess.matches(src, req.Entry, req.Env, overrides, 0) {
			return apperr.New(apperr.Stale, "Results are from another version or environment · Run 1–"+strconv.Itoa(req.Entry))
		}
		rn.opts.FromEntry, rn.opts.ToEntry = req.Entry, req.Entry
		rn.summary.BaseRunAt = &sess.at
		plain, secret := sess.layers()
		rn.captures = func(opts *engine.Options, job *engine.Job) { runplan.ApplyCaptures(opts, job, plain, secret) }
		rn.seed = sess.cookies
		rn.execute(ctx, func(res *engine.UnitResult) {
			if !res.Interrupted && !canceled(res) {
				r.mu.Lock()
				sess.merge(res)
				r.mu.Unlock()
			}
		})
		return nil
	})
}

// RunTest runs files in test mode.
func (r *Runs) RunTest(ctx context.Context, req TestRequest) (*Summary, error) {
	if len(req.Files) == 0 {
		return nil, apperr.New(apperr.Invalid, "no files to run")
	}
	return r.start(ctx, req.RunID, "test", req.Files, func(ctx context.Context, rn *run) error {
		sources := map[string]string{}
		for _, f := range req.Files {
			if s, ok := req.Sources[f]; ok {
				sources[f] = s
			}
		}
		if err := rn.plan(ctx, "test", req.Env, "", sources); err != nil {
			return err
		}
		rn.execute(ctx, nil)
		return nil
	})
}

// RunData runs a file once per row of a data file.
func (r *Runs) RunData(ctx context.Context, req DataRequest) (*Summary, error) {
	data, err := r.handles.Take(req.DataHandle, handles.OpenFile)
	if err != nil {
		return nil, apperr.Wrap(apperr.Expired, err)
	}
	return r.start(ctx, req.RunID, "data", []string{req.File}, func(ctx context.Context, rn *run) error {
		if err := rn.plan(ctx, "run", req.Env, data, map[string]string{req.File: req.Source}); err != nil {
			return err
		}
		rn.rows = req.Rows
		rn.execute(ctx, nil)
		return nil
	})
}

// start reserves files, runs body and always ends the run with Done.
func (r *Runs) start(ctx context.Context, runID, kind string, files []string, body func(context.Context, *run) error) (*Summary, error) {
	if runID == "" {
		return nil, apperr.New(apperr.Invalid, "a run needs an id")
	}
	root := r.project()
	if root == nil {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := r.reserve(runID, files, cancel); err != nil {
		return nil, err
	}
	defer r.release(runID, files)

	rn := &run{runs: r, root: root, bridge: newBridge(r.emit, runID), summary: &Summary{RunID: runID, Kind: kind, Units: []Unit{}}}
	start := time.Now()
	err := body(ctx, rn)
	rn.summary.Duration = time.Since(start).Milliseconds()
	if err != nil {
		rn.summary.Outcome, rn.summary.Error = Errored, err.Error()
	} else {
		rn.finish(ctx)
		if r.Hooks.Record != nil {
			r.Hooks.Record(rn.summary, rn.results)
		}
	}
	rn.bridge.done(rn.summary)
	if err != nil {
		var e *apperr.Error
		if errors.As(err, &e) {
			return rn.summary, err
		}
		return rn.summary, apperr.Wrap(apperr.Invalid, err)
	}
	return rn.summary, nil
}

func (r *Runs) reserve(runID string, files []string, cancel context.CancelFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.cancels[runID]; ok {
		return apperr.New(apperr.Invalid, "run "+runID+" already exists")
	}
	for _, f := range files {
		if _, ok := r.busy[f]; ok {
			return apperr.New(apperr.Busy, f+" is already running")
		}
	}
	for _, f := range files {
		r.busy[f] = runID
	}
	r.cancels[runID] = cancel
	return nil
}

func (r *Runs) release(runID string, files []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range files {
		if r.busy[f] == runID {
			delete(r.busy, f)
		}
	}
	delete(r.cancels, runID)
}

func (r *Runs) overrides() string {
	if r.Hooks.Overrides == nil {
		return ""
	}
	return r.Hooks.Overrides()
}

func sessionKey(file string, row int) string {
	if row == 0 {
		return file
	}
	return file + "#" + strconv.Itoa(row)
}

func (r *Runs) session(file string, row int) *session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[sessionKey(file, row)]
}

func (r *Runs) storeSession(file string, src []byte, env string, res *engine.UnitResult) {
	s := newSession(src, env, r.overrides(), res)
	r.mu.Lock()
	r.sessions[sessionKey(file, res.Row)] = s
	r.mu.Unlock()
}

// run is one run being executed.
type run struct {
	runs    *Runs
	root    *sandbox.Root
	bridge  *bridge
	summary *Summary

	planned   *runplan.Plan
	opts      engine.Options
	files     map[string]string // absolute path -> project path
	sources   map[string][]byte // absolute path -> buffer text
	captures  func(*engine.Options, *engine.Job)
	seed      []engine.Cookie
	rows      []int
	results   []*engine.UnitResult
	startErrs map[*engine.UnitResult]error
	redact    func(string) string // the runner's: every secret of the run
}

// plan builds the run like the CLI: cmd, env and data are the command,
// --env and --data; sources maps project paths to their buffer text.
func (rn *run) plan(ctx context.Context, cmd, env, data string, sources map[string]string) error {
	inv := runplan.Invocation{Cmd: cmd, Env: env, Data: data, FileRoot: rn.root.Dir(), Set: map[string]bool{"file-root": true}}
	if env != "" {
		inv.Set["env"] = true
	}
	if data != "" {
		inv.Set["data"] = true
	}
	if h := rn.runs.Hooks.Extend; h != nil {
		h(&inv)
	}
	rn.files, rn.sources = map[string]string{}, map[string][]byte{}
	files := rn.reserved()
	var inputs []runplan.Input
	for _, f := range files {
		abs, err := rn.abs(f)
		if err != nil {
			return err
		}
		rn.files[abs] = f
		if s, ok := sources[f]; ok {
			rn.sources[abs] = []byte(s)
		}
		inputs = append(inputs, runplan.Input{Name: abs})
	}
	p, err := runplan.New(&inv, rn.runs.env, rn.runs.version)
	if err != nil {
		return err
	}
	if err := p.Resolve(ctx, inputs); err != nil {
		return err
	}
	rn.planned, rn.opts = p, p.Options
	rn.opts.Verbosity = max(rn.opts.Verbosity, engine.Verbose)
	rn.opts.BufferedLogs = true // a redact capture's value may otherwise be logged
	rn.opts.Stdout = io.Discard
	rn.summary.Warnings = p.Warnings
	return nil
}

// reserved is the run's files, sorted.
func (rn *run) reserved() []string {
	r := rn.runs
	r.mu.Lock()
	defer r.mu.Unlock()
	var files []string
	for f, id := range r.busy {
		if id == rn.summary.RunID {
			files = append(files, f)
		}
	}
	slices.Sort(files)
	return files
}

// abs resolves a project path to the absolute path runplan runs.
func (rn *run) abs(file string) (string, error) {
	if file == "" || filepath.IsAbs(file) || filepath.VolumeName(file) != "" {
		return "", apperr.New(apperr.Denied, "not a path in the project: "+file)
	}
	rel := filepath.FromSlash(file)
	if _, err := rn.root.Stat(rel); err != nil {
		if errors.Is(err, sandbox.ErrDenied) {
			return "", apperr.New(apperr.Denied, "not a path in the project: "+file)
		}
		return "", apperr.New(apperr.NotFound, file+" does not exist")
	}
	return filepath.Join(rn.root.Dir(), rel), nil
}

// execute runs the planned jobs; stored is called with each result of a
// full, single-file run (sessions).
func (rn *run) execute(ctx context.Context, stored func(*engine.UnitResult)) {
	opts := rn.opts
	var dataErr error
	jobs := rn.jobs(rn.planned.Jobs(nil, &dataErr), &opts)
	runner := engine.NewRunner(opts)
	rn.redact = runner.Redact
	enginex.EnableHostEvents(runner)
	if seed := rn.seedCookies(); seed != nil {
		enginex.SeedCookies(runner, seed)
	}
	converters := map[int]*view.Converter{}
	runner.RunAll(ctx, jobs, engine.RunAllOptions{
		Parallel: rn.planned.Workers,
		Started: func(seq int, job engine.Job) (func(engine.Event), io.Writer) {
			c := view.NewConverter(rn.files[job.Name], runner.Redact, rn.runs.bodies, func(dto any) {
				_, message := dto.(view.Message)
				rn.bridge.add(seq, dto, message)
			})
			converters[seq] = c
			return c.Handle, nil
		},
		Finished: func(seq int, job engine.Job, res *engine.UnitResult, err error) bool {
			if c := converters[seq]; c != nil {
				c.Flush()
			}
			if res == nil { // the job could not start (an unreadable file…)
				res = &engine.UnitResult{File: job.Name, Row: rowOf(job)}
				if rn.startErrs == nil {
					rn.startErrs = map[*engine.UnitResult]error{}
				}
				rn.startErrs[res] = err
			}
			rn.results = append(rn.results, res)
			if stored != nil {
				stored(res)
			}
			if kept := rn.runs.Hooks.KeepCookies; kept != nil && rn.summary.Kind == "run" && !res.Interrupted {
				kept(rn.files[job.Name], res.Cookies)
			}
			return true
		},
	})
	if dataErr != nil {
		rn.summary.Warnings = append(rn.summary.Warnings, dataErr.Error())
	}
}

// jobs applies the buffers, the Send captures and the row filter.
func (rn *run) jobs(all iter.Seq[engine.Job], opts *engine.Options) iter.Seq[engine.Job] {
	if rn.captures != nil {
		var probe engine.Job
		rn.captures(opts, &probe) // Options layers; the job's are applied per job
	}
	return func(yield func(engine.Job) bool) {
		for job := range all {
			if src, ok := rn.sources[job.Name]; ok {
				job.Source = src
			}
			if len(rn.rows) > 0 && (job.Row == nil || !slices.Contains(rn.rows, job.Row.Index)) {
				continue
			}
			if rn.captures != nil {
				o := *opts
				rn.captures(&o, &job)
			}
			if !yield(job) {
				return
			}
		}
	}
}

// seedCookies is the jar a run starts with: the Send session's, or the
// kept jar of a full run when keep cookies is on.
func (rn *run) seedCookies() []engine.Cookie {
	if rn.seed != nil {
		return rn.seed
	}
	if k := rn.runs.Hooks.KeptCookies; k != nil && rn.summary.Kind == "run" && len(rn.files) == 1 {
		for _, f := range rn.files {
			return k(f)
		}
	}
	return nil
}

// finish summarizes the results.
func (rn *run) finish(ctx context.Context) {
	s := rn.summary
	outcome := Passed
	for _, res := range rn.results {
		u := Unit{
			File: rn.files[res.File], Row: res.Row, Success: res.Success, Interrupted: res.Interrupted,
			Canceled: canceled(res) || res.Interrupted && ctx.Err() != nil,
			Requests: len(res.Entries), Duration: res.Duration.Milliseconds(),
		}
		if res.ParseError != nil {
			e := view.ConvertError(res.ParseError, res.Redact)
			u.ParseError = &e
		}
		if err := rn.startErrs[res]; err != nil {
			u.Error = rn.redact(err.Error())
		}
		s.Units = append(s.Units, u)
		s.Files++
		s.Requests += u.Requests
		switch {
		case u.Canceled:
			outcome = Canceled
		case (u.ParseError != nil || u.Error != "") && outcome != Canceled:
			outcome = Errored
		case !u.Success && outcome == Passed:
			outcome = Failed
		}
		if u.Success {
			s.Succeeded++
		}
	}
	s.Outcome = outcome
}

func rowOf(job engine.Job) int {
	if job.Row == nil {
		return 0
	}
	return job.Row.Index
}

// canceled reports whether a result ended with a canceled request.
func canceled(res *engine.UnitResult) bool {
	for _, err := range res.Errors() {
		if enginex.Transport(err) == "canceled" {
			return true
		}
	}
	return false
}
