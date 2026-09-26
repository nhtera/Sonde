// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/redact"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

// Runner runs files with shared options. Secrets known to a runner
// (options and `redact` captures of every file it ran) are redacted from
// the log events it sends and by Redact. A Runner runs one file at a time.
type Runner struct {
	opt     Options
	secrets *redact.Registry
}

// NewRunner returns a runner.
func NewRunner(opt Options) *Runner {
	r := &Runner{opt: opt, secrets: redact.New()}
	for name, v := range opt.Secrets {
		r.secrets.Add(name, v)
	}
	return r
}

// Redact masks every secret known to the runner in s.
func (r *Runner) Redact(s string) string { return r.secrets.Redact(s) }

// HasSecrets reports whether the runner knows any secret.
func (r *Runner) HasSecrets() bool { return r.secrets.Len() > 0 }

// Close releases the runner's resources.
func (r *Runner) Close() error { return nil }

// RunFile reads and runs a file; the error reports a file that cannot be
// read or a setup failure (e.g. an unreadable cookie file).
func (r *Runner) RunFile(ctx context.Context, path string) (*UnitResult, error) {
	src, err := readSource(path)
	if err != nil {
		return nil, err
	}
	return r.RunSource(ctx, path, src)
}

// RunSource runs the content of a file named name (used for messages and
// to locate the default file root).
func (r *Runner) RunSource(ctx context.Context, name string, src []byte) (*UnitResult, error) {
	return r.runSource(ctx, name, src, unitIO{onEvent: r.opt.OnEvent, stdout: r.opt.Stdout, validator: r.opt.Validator})
}

// unitIO is where a unit sends its events and output, and when it stops.
type unitIO struct {
	onEvent func(Event)
	stdout  io.Writer
	// stop, when closed, ends the unit at its next entry boundary.
	stop <-chan struct{}
	// vars and secrets of the job, below the runner's options.
	vars    map[string]any
	secrets map[string]string
	// row, when set, is the data row, above the runner's options.
	row *Row
	// validator checks responses against a contract (nil: none).
	validator ResponseValidator
}

func (r *Runner) runSource(ctx context.Context, name string, src []byte, uio unitIO) (*UnitResult, error) {
	res := &UnitResult{File: name, Source: src, Timestamp: time.Now(), runSecrets: r.secrets}
	if uio.row != nil {
		res.Row = uio.row.Index
		res.rowSecrets = redact.New()
		for name, v := range uio.row.Secrets {
			res.rowSecrets.Add(name, v)
		}
	}
	f, err := syntax.Parse(name, src, syntax.DialectFor(name))
	if err != nil {
		var perr *syntax.Error
		if !errors.As(err, &perr) {
			return nil, err
		}
		res.ParseError = perr
		return res, nil
	}
	rootDir := r.opt.FileRoot
	if rootDir == "" {
		rootDir = filepath.Dir(name)
	}
	root, err := sandbox.Open(rootDir)
	if err != nil {
		return nil, err
	}
	defer root.Close() //nolint:errcheck // read-only use
	u := &unit{runner: r, file: f, name: name, src: src, root: root, rootDir: rootDir, io: uio, rowSecrets: res.rowSecrets}
	client, err := httpx.NewClient(httpx.ClientConfig{
		Sandbox:       root,
		CookieFile:    r.opt.CookieFile,
		NoCookieStore: r.opt.NoCookieStore,
		Version:       r.opt.Version,
		UserAgent:     r.opt.DefaultUserAgent,
		Debug: func(line string) {
			if u.verbosity >= VeryVerbose {
				u.log(LogDebug, line)
			}
		},
		Warn: func(msg string) { u.log(LogWarning, msg) },
	})
	if err != nil {
		return nil, err
	}
	defer client.Close() //nolint:errcheck // idle connections only
	u.client = client
	vars, err := r.variables(uio)
	if err != nil {
		return nil, err
	}
	u.env = &template.Env{Vars: vars, Now: r.opt.Now, UUID: r.opt.UUID, ReadFile: u.readFile}
	start := time.Now()
	u.run(ctx, res)
	res.Duration = time.Since(start)
	for _, c := range client.Cookies() {
		res.Cookies = append(res.Cookies, Cookie(c))
	}
	return res, nil
}

// variables builds the initial variables: the job's, the runner's
// options, then the data row's.
func (r *Runner) variables(uio unitIO) (template.Vars, error) {
	vars := template.Vars{}
	var rowVars map[string]any
	if uio.row != nil {
		rowVars = uio.row.Variables
	}
	for _, src := range []map[string]any{uio.vars, r.opt.Variables, rowVars} {
		for name, v := range src {
			val, err := toValue(v)
			if err != nil {
				return nil, fmt.Errorf("variable %s: %w", name, err)
			}
			vars.Set(name, val)
		}
	}
	for i, src := range []map[string]string{uio.secrets, r.opt.Secrets} {
		for name, v := range src {
			r.secrets.Add(name, v)
			if _, ok := r.opt.Variables[name]; i == 0 && ok {
				continue // the runner's variable wins over a job secret
			}
			vars.SetSecret(name, v)
		}
	}
	if uio.row != nil {
		for name, v := range uio.row.Secrets {
			vars.SetSecret(name, v)
		}
	}
	return vars, nil
}

func toValue(v any) (value.Value, error) {
	switch v := v.(type) {
	case nil:
		return value.Null{}, nil
	case value.Value:
		return v, nil
	case string:
		return value.String(v), nil
	case bool:
		return value.Bool(v), nil
	case int:
		return value.Int(v), nil
	case int64:
		return value.Int(v), nil
	case float64:
		return value.Float(v), nil
	case []any:
		list := make(value.List, len(v))
		for i, e := range v {
			ev, err := toValue(e)
			if err != nil {
				return nil, err
			}
			list[i] = ev
		}
		return list, nil
	case map[string]any:
		obj := make(value.Object, 0, len(v))
		for k, e := range v {
			ev, err := toValue(e)
			if err != nil {
				return nil, err
			}
			obj = append(obj, value.Member{Key: k, Value: ev})
		}
		slices.SortFunc(obj, func(a, b value.Member) int { return strings.Compare(a.Key, b.Key) })
		return obj, nil
	}
	return nil, fmt.Errorf("unsupported type %T", v)
}

// unit is the state of one file run.
type unit struct {
	runner *Runner
	file   *syntax.File
	name   string
	src    []byte
	root   *sandbox.Root
	// rootDir is the file root as given (possibly relative), for messages.
	rootDir string
	client  *httpx.Client
	env     *template.Env
	// verbosity of the running entry.
	verbosity Verbosity
	// last is the index of the last entry to run.
	last int
	io   unitIO
	// rowSecrets are the data row's secrets (nil without a row).
	rowSecrets *redact.Registry
	// forExport is true only for a unit RenderCurl builds (curl_export.go):
	// it never sends anything, so buildRequest/body/multipartParam skip
	// checkURL and never read a file body's actual content, only its
	// name — see the comments at each check.
	forExport bool
}

// addSecret registers a secret found while running: with the row's
// secrets when the unit runs a data row, so that rows never grow the
// run's secrets, else with the run's.
func (u *unit) addSecret(name, v string) {
	if u.rowSecrets != nil {
		u.rowSecrets.Add(name, v)
		return
	}
	u.runner.secrets.Add(name, v)
}

// redact masks the run's secrets and the row's.
func (u *unit) redact(s string) string {
	return u.runner.secrets.RedactWith(s, u.rowSecrets)
}

func (u *unit) emit(ev Event) {
	if u.io.onEvent != nil {
		u.io.onEvent(ev)
	}
}

// stopped reports whether the unit must not start another entry.
func (u *unit) stopped(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	select {
	case <-u.io.stop:
		return true
	default:
		return false
	}
}

func (u *unit) log(level LogLevel, text string) {
	u.emit(Log{Level: level, Text: u.redact(text)})
}

// debug logs verbose detail when the running entry is verbose.
func (u *unit) debug(text string) {
	if u.verbosity >= Verbose {
		u.log(LogDebug, text)
	}
}

func (u *unit) debugImportant(text string) {
	if u.verbosity >= Verbose {
		u.log(LogDebugImportant, text)
	}
}

// readFile reads request-file paths through the sandbox.
func (u *unit) readFile(name string) ([]byte, error) {
	b, err := u.root.ReadFile(name)
	if errors.Is(err, sandbox.ErrDenied) {
		return nil, fmt.Errorf("%w: %s", template.ErrFileAccessDenied, name)
	}
	return b, err
}

// protectCredentials registers "user:password" as a secret when it
// contains one, so that its Base64 form in an Authorization header is
// redacted too.
func (u *unit) protectCredentials(userPass string) {
	if userPass != "" && u.redact(userPass) != userPass {
		u.addSecret("credentials", userPass)
	}
}

// logError logs a runtime error of an entry starting at entryLine,
// rendered for a terminal (CRLF line endings become LF), plain and with
// colors.
func (u *unit) logError(level LogLevel, err *runerr.Error, entryLine int) {
	lf := strings.NewReplacer("\r\n", "\n")
	u.emit(Log{
		Level: level,
		Text:  u.redact(lf.Replace(err.Render(u.name, string(u.src), entryLine))),
		Color: u.redact(lf.Replace(err.RenderColor(u.name, string(u.src), entryLine))),
	})
}

// run executes the entries of the file.
func (u *unit) run(ctx context.Context, res *UnitResult) {
	opt := u.runner.opt
	entries := u.file.Entries
	if len(entries) == 0 {
		res.Success = true
		return
	}
	current, last := 1, len(entries)
	if opt.FromEntry > 0 {
		current = opt.FromEntry
	}
	if opt.ToEntry > 0 && opt.ToEntry < last {
		last = opt.ToEntry
	}
	u.last = last
	u.verbosity = opt.Verbosity
	u.logRunInfo(last, len(entries))
	repeatCount := 0
	for current <= last {
		if u.stopped(ctx) {
			res.Interrupted = true
			break
		}
		entry := entries[current-1]
		u.verbosity = u.entryVerbosity(entry)
		u.debugImportant("------------------------------------------------------------------------------")
		u.debugImportant(fmt.Sprintf("Executing entry %d", current))

		eo, err := u.entryOptions(entry)
		if err == nil {
			u.protectCredentials(eo.http.User)
		}
		if err != nil {
			re := asRunErr(err, entry.Request.Span)
			er := &EntryResult{Index: current, Line: entry.Request.Method.Span.Start.Line, Errors: []*runerr.Error{re}}
			u.logError(LogError, re, er.Line)
			res.Entries = append(res.Entries, er)
			if opt.ContinueOnError {
				current++
				continue
			}
			break
		}
		if eo.skip {
			u.debug("")
			u.debugImportant(fmt.Sprintf("Entry %d has been skipped", current))
			current++
			continue
		}
		if eo.repeat != nil && *eo.repeat == 0 {
			u.debug("")
			u.debugImportant(fmt.Sprintf("Entry %d is skipped (repeat 0 times)", current))
			current++
			continue
		}
		if eo.delay > 0 {
			u.debug("")
			u.debugImportant(fmt.Sprintf("Delay entry %d (pause %d ms)", current, eo.delay.Milliseconds()))
			if !u.sleep(ctx, eo.delay) {
				res.Interrupted = true
				break
			}
		}
		results := u.runWithRetry(ctx, entry, current, eo)
		res.Entries = append(res.Entries, results...)
		hasError := len(results) > 0 && len(results[len(results)-1].Errors) > 0
		if hasError && !opt.ContinueOnError {
			break
		}
		repeatCount++
		switch {
		case eo.repeat == nil:
			repeatCount = 0
			current++
		case *eo.repeat == -1:
			u.debugImportant(fmt.Sprintf("Repeat entry %d (x%d)", current, repeatCount))
		case repeatCount >= *eo.repeat:
			repeatCount = 0
			current++
		default:
			u.debugImportant(fmt.Sprintf("Repeat entry %d (x%d/%d)", current, repeatCount, *eo.repeat))
		}
	}
	res.Success = success(res.Entries) && !res.Interrupted
	if res.Success && len(res.Entries) == 0 {
		u.log(LogWarning, "No entry have been executed for file "+u.name)
	}
}

// success reports whether every attempt that was not retried has no
// error.
func success(entries []*EntryResult) bool {
	for _, e := range entries {
		if !e.Retried && len(e.Errors) > 0 {
			return false
		}
	}
	return true
}

// runWithRetry runs an entry, retrying on any error as configured.
func (u *unit) runWithRetry(ctx context.Context, entry *syntax.Entry, index int, eo *entryOptions) []*EntryResult {
	var results []*EntryResult
	for retry := 0; ; retry++ {
		u.emit(EntryStarted{Index: index, Retry: retry, Last: u.last})
		res := u.runEntry(ctx, entry, index, eo)
		hasError := len(res.Errors) > 0
		maxReached := eo.retry >= 0 && retry >= eo.retry
		if maxReached && eo.retry > 0 {
			u.debugImportant("Retry max count reached, no more retry")
			u.debug("")
		}
		again := eo.retry != 0 && !maxReached && hasError && !u.stopped(ctx)
		res.Retried = again
		switch {
		case !hasError && eo.output != nil:
			u.writeOutput(res, eo)
		case hasError && again:
			for _, err := range res.Errors {
				u.logError(LogDebugError, err, res.Line)
			}
		case hasError:
			for _, err := range res.Errors {
				u.logError(LogError, err, res.Line)
			}
		}
		u.emit(EntryFinished{Result: res})
		results = append(results, res)
		if !again {
			return results
		}
		limit := "ꝏ"
		if eo.retry >= 0 {
			limit = fmt.Sprint(eo.retry)
		}
		u.debug("")
		u.debugImportant(fmt.Sprintf("Retry on entry %d (count: %d/%s, interval: %d ms)",
			index, retry+1, limit, eo.retryInterval.Milliseconds()))
		if !u.sleep(ctx, eo.retryInterval) {
			res.Retried = false // the retry never happened: its error stands
			return results
		}
		u.debugImportant("------------------------------------------------------------------------------")
		u.debugImportant(fmt.Sprintf("Executing entry %d", index))
	}
}

// sleep waits d, or until ctx is done or the unit is stopped; it reports
// whether d elapsed.
func (u *unit) sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	case <-u.io.stop:
		return false
	}
}

// logRunInfo logs the non-secret variables and the entry range.
func (u *unit) logRunInfo(last, total int) {
	if u.verbosity < Verbose {
		return
	}
	var names []string
	for name, v := range u.env.Vars {
		if !v.Secret {
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		slices.Sort(names)
		u.debugImportant("Variables:")
		for _, n := range names {
			u.debug(fmt.Sprintf("    %s: %s", n, value.Display(u.env.Vars[n].Value)))
		}
	}
	if u.runner.opt.ToEntry > 0 {
		u.debug(fmt.Sprintf("Executing %d/%d entries", last, total))
	}
}
