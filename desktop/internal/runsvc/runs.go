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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/credential"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/view"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/value"
)

// Hooks connect runs to the app's other services; each may be nil.
type Hooks struct {
	// Extend adds the app's settings and session overrides to an
	// invocation; Overrides returns a digest of them (sessions are keyed
	// on it).
	Extend    func(inv *runplan.Invocation)
	Overrides func() string
	// KeptJar returns the path of the jar kept for file between full runs
	// ("" when keep cookies is off or there is none): the run reads it as
	// -b. KeepCookies stores the jar after a full run.
	KeptJar     func(file string) string
	KeepCookies func(file string, cookies []engine.Cookie)
	// Record stores a finished run in the history.
	Record func(s *Summary, results []*engine.UnitResult)
	// Protected reports a project file the page never reads (secrets, dot
	// files): never a data file either.
	Protected func(file string) bool
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
	tests    []testRun           // the last test runs, for their reports
}

// testRun is a test run kept for its reports: its results in the order
// of its files, and its redactor.
type testRun struct {
	id      string
	results []*engine.UnitResult
	redact  func(string) string
}

// keptTests is how many test runs keep their results for reports: the
// last (they hold every body), until another project opens.
const keptTests = 1

// New returns the run service. env is the app's process environment;
// version names the default User-Agent.
func New(e emit.Emitter, project func() *sandbox.Root, env config.Env, version string, bodies view.BodyStore, h *handles.Table) *Runs {
	return &Runs{
		emit: e, project: project, env: env, version: version, bodies: bodies, handles: h,
		busy: map[string]string{}, cancels: map[string]context.CancelFunc{}, sessions: map[string]*session{},
	}
}

// Invocation is the `sonde` invocation of a run (Copy as prints it): the
// command, env and data file, the file root (the project folder, "."),
// the overrides, and for a full run of one file with keep cookies on its
// kept jar as -b. Files are project paths.
func (r *Runs) Invocation(cmd, env, data, kind string, files []string) runplan.Invocation {
	inv := runplan.Invocation{Cmd: cmd, Env: env, Data: data, FileRoot: ".", Files: files, Set: map[string]bool{"file-root": true}}
	if env != "" {
		inv.Set["env"] = true
	}
	if data != "" {
		inv.Set["data"] = true
	}
	if h := r.Hooks.Extend; h != nil {
		h(&inv)
	}
	if k := r.Hooks.KeptJar; k != nil && kind == "run" && len(files) == 1 {
		if jar := k(files[0]); jar != "" {
			inv.Cookie = jar
			inv.Set["cookie"] = true
		}
	}
	return inv
}

// TestOptions sets a test run's options on inv, as the command's flags:
// --jobs (0 keeps the default) and --continue-on-error.
func TestOptions(inv *runplan.Invocation, jobs int, continueOnError bool) {
	if jobs > 0 {
		inv.Jobs, inv.Set["jobs"] = jobs, true
	}
	if continueOnError {
		inv.ContinueOnError, inv.Set["continue-on-error"] = true, true
	}
}

// Planned is a planned run of one file, not run: the options with the
// job's project layers merged (for rendering curl commands, which read
// the options only) and its captures from the file's last run.
func (r *Runs) Planned(ctx context.Context, file, source, env string, withCaptures bool) (engine.Options, error) {
	root := r.project()
	if root == nil {
		return engine.Options{}, apperr.New(apperr.NotFound, "no project is open")
	}
	rn := &run{runs: r, root: root, summary: &Summary{Kind: "prepare"}}
	if err := rn.plan(ctx, "run", env, "", []string{file}, map[string]string{file: source}); err != nil {
		return engine.Options{}, apperr.Wrap(apperr.Invalid, err)
	}
	opts := rn.opts
	var dataErr error
	for job := range rn.planned.Jobs(nil, &dataErr) {
		opts.Variables = merge(job.Variables, opts.Variables)
		opts.Secrets = merge(job.Secrets, opts.Secrets)
		break
	}
	if s := r.session(file, 0); withCaptures && s != nil {
		r.mu.Lock()
		plain, secret := s.layers()
		r.mu.Unlock()
		var job engine.Job
		runplan.ApplyCaptures(&opts, &job, plain, secret)
	}
	return opts, nil
}

// merge layers top over base (new maps).
func merge[V any](base, top map[string]V) map[string]V {
	out := make(map[string]V, len(base)+len(top))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range top {
		out[k] = v
	}
	return out
}

// Prepare plans a run of file (its buffer source, in env) without running
// it: the runner and job, for tools that act on one entry (gRPC methods,
// an interactive WebSocket session).
func (r *Runs) Prepare(ctx context.Context, file, source, env string) (*engine.Runner, engine.Job, error) {
	root := r.project()
	if root == nil {
		return nil, engine.Job{}, apperr.New(apperr.NotFound, "no project is open")
	}
	rn := &run{runs: r, root: root, summary: &Summary{Kind: "prepare"}}
	if err := rn.plan(ctx, "run", env, "", []string{file}, map[string]string{file: source}); err != nil {
		return nil, engine.Job{}, apperr.Wrap(apperr.Invalid, err)
	}
	opts := rn.opts
	var dataErr error
	for job := range rn.jobs(rn.planned.Jobs(nil, &dataErr), &opts) {
		runner := engine.NewRunner(opts)
		enginex.EnableHostEvents(runner)
		return runner, job, nil
	}
	return nil, engine.Job{}, apperr.New(apperr.Invalid, "nothing to run")
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
		if err := rn.plan(ctx, "run", req.Env, "", []string{req.File}, map[string]string{req.File: req.Source}); err != nil {
			return err
		}
		rn.opts.ToEntry = req.To
		src := []byte(req.Source)
		values := rn.valuesDigest()
		rn.execute(ctx, func(res *engine.UnitResult) {
			if !res.Interrupted && !canceled(res) {
				r.storeSession(req.File, src, req.Env, values, res)
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
		if err := rn.plan(ctx, "run", req.Env, "", []string{req.File}, map[string]string{req.File: req.Source}); err != nil {
			return err
		}
		sess := r.session(req.File, 0)
		values := rn.valuesDigest()
		if sess == nil {
			// No run to reuse: run 1…n, which then is one.
			rn.opts.ToEntry = req.Entry
			rn.execute(ctx, func(res *engine.UnitResult) {
				if !res.Interrupted && !canceled(res) {
					r.storeSession(req.File, src, req.Env, values, res)
				}
			})
			return nil
		}
		r.mu.Lock()
		ok := sess.matches(src, req.Entry, req.Env, values, 0)
		r.mu.Unlock()
		if !ok {
			return apperr.New(apperr.Stale, "Results are from another version or environment · Run 1–"+strconv.Itoa(req.Entry))
		}
		rn.opts.FromEntry, rn.opts.ToEntry = req.Entry, req.Entry
		rn.summary.BaseRunAt = &sess.at
		r.mu.Lock()
		plain, secret := sess.layers()
		r.mu.Unlock()
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
		// No more jobs than files.
		rn.tune = func(inv *runplan.Invocation) { TestOptions(inv, min(req.Jobs, len(req.Files)), req.ContinueOnError) }
		if err := rn.plan(ctx, "test", req.Env, "", req.Files, sources); err != nil {
			return err
		}
		rn.execute(ctx, nil)
		return nil
	})
}

// RunData runs a file once per row of a data file.
func (r *Runs) RunData(ctx context.Context, req DataRequest) (*Summary, error) {
	return r.start(ctx, req.RunID, "data", []string{req.File}, func(ctx context.Context, rn *run) error {
		// A dialog's handle is taken once the file is ours: a busy file
		// keeps it.
		var data string
		var err error
		if req.DataFile != "" {
			if data, err = r.dataFile(rn, req.DataFile); err != nil {
				return err
			}
		} else if data, err = r.handles.Take(req.DataHandle, handles.OpenFile); err != nil {
			return apperr.Wrap(apperr.Expired, err)
		}
		rn.dataSecrets = req.Secrets
		if req.DataFile != "" {
			// A project data file's credential columns (password, token…)
			// are secrets, as if named with --data-secret: their values
			// never reach the page.
			for _, col := range dataColumns(rn.root, req.DataFile) {
				if credential.Likely(col, "") && !slices.Contains(rn.dataSecrets, col) {
					rn.dataSecrets = append(rn.dataSecrets, col)
				}
			}
		}
		if err := rn.plan(ctx, "run", req.Env, data, []string{req.File}, map[string]string{req.File: req.Source}); err != nil {
			return err
		}
		rn.rows = req.Rows
		rn.execute(ctx, nil)
		return nil
	})
}

// dataFile is the absolute path of a project data file: a .csv or .json
// the page could read, never a secrets or dot file.
func (r *Runs) dataFile(rn *run, file string) (string, error) {
	ext := strings.ToLower(filepath.Ext(file))
	if ext != ".csv" && ext != ".json" {
		return "", apperr.New(apperr.Invalid, "a data file is a .csv or .json file: "+file)
	}
	if r.Hooks.Protected != nil && r.Hooks.Protected(file) {
		return "", apperr.New(apperr.Denied, file+" holds secrets or settings the app does not show")
	}
	abs, err := rn.abs(file)
	if err != nil {
		return "", err
	}
	// A plain file: not a link the run would follow out of the project.
	if fi, err := rn.root.Lstat(filepath.FromSlash(file)); err != nil || !fi.Mode().IsRegular() {
		return "", apperr.New(apperr.Denied, file+" is not a regular file in the project")
	}
	return abs, nil
}

// dataColumns lists the columns of a project data file: a CSV's header,
// or the keys of a JSON array's first object.
func dataColumns(root *sandbox.Root, file string) []string {
	data, err := root.ReadFile(filepath.FromSlash(file))
	if err != nil {
		return nil
	}
	if strings.EqualFold(filepath.Ext(file), ".json") {
		var rows []map[string]json.RawMessage
		if json.Unmarshal(data, &rows) != nil || len(rows) == 0 {
			return nil
		}
		return slices.Sorted(maps.Keys(rows[0]))
	}
	header, err := csv.NewReader(bytes.NewReader(data)).Read()
	if err != nil {
		return nil
	}
	return header
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
	rn.summary.StartedAt = start.UTC()
	err := body(ctx, rn)
	rn.summary.Duration = time.Since(start).Milliseconds()
	if err != nil {
		rn.summary.Outcome, rn.summary.Error = Errored, err.Error()
		if e, ok := errors.AsType[*apperr.Error](err); ok {
			rn.summary.ErrorCode = e.Code
		}
	} else {
		rn.finish(ctx)
		if kind == "test" {
			rn.summary.Text = testText(rn.shown(), time.Duration(rn.summary.Duration)*time.Millisecond)
			r.keepTest(runID, rn)
		}
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

// Reset forgets every Send session (a project was opened: its files are
// not the previous project's).
func (r *Runs) Reset() {
	r.mu.Lock()
	r.sessions = map[string]*session{}
	r.tests = nil
	r.mu.Unlock()
}

// Capture is a capture of a file's last run, for the variables list. A
// redacted capture has no Value.
type Capture struct {
	Name   string
	Value  string
	Secret bool
}

// Captures returns the captures of file's last full run (and its Sends),
// last writer winning; nil when it has none.
func (r *Runs) Captures(file string) []Capture {
	s := r.session(file, 0)
	if s == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	byName := map[string]int{}
	var out []Capture
	for _, c := range s.captures {
		it := Capture{Name: c.name, Secret: c.secret}
		if !c.secret {
			it.Value = c.value.String()
		}
		if i, ok := byName[c.name]; ok {
			out[i] = it
			continue
		}
		byName[c.name] = len(out)
		out = append(out, it)
	}
	return out
}

func (r *Runs) storeSession(file string, src []byte, env, values string, res *engine.UnitResult) {
	s := newSession(src, env, values, res)
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

	planned  *runplan.Plan
	opts     engine.Options
	files    map[string]string // absolute path -> project path
	sources  map[string][]byte // absolute path -> buffer text
	captures func(*engine.Options, *engine.Job)
	seed     []engine.Cookie
	rows     []int
	// tune sets the run's own options on its invocation (a test run's).
	tune        func(*runplan.Invocation)
	dataSecrets []string
	results     []*engine.UnitResult
	seqs        map[*engine.UnitResult]int // each result's job, in input order
	startErrs   map[*engine.UnitResult]error
	redact      func(string) string // the runner's: every secret of the run
}

// plan builds the run like the CLI: cmd, env and data are the command,
// --env and --data; sources maps project paths to their buffer text.
func (rn *run) plan(ctx context.Context, cmd, env, data string, files []string, sources map[string]string) error {
	inv := rn.runs.Invocation(cmd, env, data, rn.summary.Kind, files)
	if rn.tune != nil {
		rn.tune(&inv)
	}
	inv.FileRoot = rn.root.Dir()
	if len(rn.dataSecrets) > 0 {
		inv.DataSecrets, inv.Set["data-secret"] = rn.dataSecrets, true
	}
	// The files run in the order given, as the command's arguments do.
	rn.files, rn.sources = map[string]string{}, map[string][]byte{}
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
	rn.summary.Env = env
	return nil
}

// valuesDigest is a digest of what the planned run resolves: the
// overrides and the first job's project values (secrets hashed with the
// rest; the digest never leaves the process).
func (rn *run) valuesDigest() string {
	h := sha256.New()
	_, _ = io.WriteString(h, rn.runs.overrides()+"\x00")
	var dataErr error
	for job := range rn.planned.Jobs(nil, &dataErr) {
		for _, m := range []map[string]string{stringsOf(job.Variables), job.Secrets, stringsOf(rn.opts.Variables), rn.opts.Secrets} {
			for _, k := range slices.Sorted(maps.Keys(m)) {
				_, _ = io.WriteString(h, k+"="+m[k]+"\x00")
			}
			_, _ = io.WriteString(h, "\x01")
		}
		break
	}
	return hex.EncodeToString(h.Sum(nil))
}

func stringsOf(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
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
			if job.Row != nil {
				c.SetRow(job.Row.Index, rowLabel(job.Row))
			}
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
			if rn.seqs == nil {
				rn.seqs = map[*engine.UnitResult]int{}
			}
			rn.seqs[res] = seq
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

// seedCookies is the jar a Send starts with: its session's. (A full run
// with keep cookies reads its kept jar as -b.)
func (rn *run) seedCookies() []engine.Cookie { return rn.seed }

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

// rowLabels are the data columns that name a row, best first.
var rowLabels = []string{"name", "title", "label", "username", "user", "email", "id"}

// rowLabel names a data row by its first naming column ("Grace Hopper"):
// never a secret column or a value that looks like a credential, and short.
func rowLabel(row *engine.Row) string {
	names := slices.Sorted(maps.Keys(row.Variables))
	for _, key := range rowLabels {
		for _, name := range names {
			if !strings.EqualFold(name, key) {
				continue
			}
			v := row.Variables[name]
			var text string
			switch v := v.(type) {
			case string:
				text = v
			case value.Value:
				text = value.Display(v)
			}
			if _, secret := row.Secrets[name]; secret || text == "" || credential.Likely(name, text) {
				continue
			}
			if r := []rune(text); len(r) > 40 {
				text = string(r[:39]) + "…"
			}
			return text
		}
	}
	return ""
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
