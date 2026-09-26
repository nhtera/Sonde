// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/report"
	"github.com/nhtera/sonde/internal/syntax"
)

// newRunCmd returns the explicit `sonde run [options] FILE...` alias; its
// flags and behavior are identical to the root command's default form.
func newRunCmd() *cobra.Command {
	o := &runOptions{}
	cmd := &cobra.Command{
		Use:                   "run [options] FILE...",
		Short:                 "Run request files (same as `sonde FILE...`)",
		Args:                  cobra.ArbitraryArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMain(cmd, o, args, false)
		},
	}
	addRunFlags(cmd, o)
	return cmd
}

// newTestCmd returns the `sonde test [options] FILE|DIR...` alias: `sonde
// run` with --test forced on, whether or not the flag itself was given.
func newTestCmd() *cobra.Command {
	o := &runOptions{}
	cmd := &cobra.Command{
		Use:                   "test [options] FILE|DIR...",
		Short:                 "Run request files in test mode (same as `sonde --test FILE...`)",
		Args:                  cobra.ArbitraryArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMain(cmd, o, args, true)
		},
	}
	addRunFlags(cmd, o)
	return cmd
}

// runMain is the shared body of the root command's default dispatch, the
// `run` subcommand and the `test` subcommand (forceTest).
func runMain(cmd *cobra.Command, o *runOptions, args []string, forceTest bool) error {
	if o.version {
		return writeVersion(cmd.OutOrStdout(), currentBuildInfo())
	}

	env := config.FromOSEnviron()
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	rc, err := buildRunContext(cmd, o, env, stdout)
	if err != nil {
		return err
	}
	if forceTest {
		rc.test = true
		rc.parallel = true
		rc.noOutput = true
		if rc.jobs < 2 {
			rc.jobs = resolveJobs(cmd, o, env, true)
		}
	}

	files, err := resolveInputFiles(args, rc.glob)
	if err != nil {
		return err
	}

	extras, defaultsJobs, projectWarnings, err := resolveJobExtras(files, rc, env)
	if err != nil {
		return err
	}
	if rc.data != nil {
		for name, e := range extras {
			if err := rc.data.checkSecrets(e.secrets); err != nil {
				return NewExitError(ExitUsage, fmt.Errorf("%s: %w (sonde.yaml)", name, err))
			}
		}
	}
	if err := resolveContracts(cmd, o, rc, extras, files); err != nil {
		return err
	}
	if rc.jobs > 1 && defaultsJobs > 0 && !changed(cmd, "jobs") {
		if _, _, ok := env.Lookup("JOBS"); !ok {
			rc.jobs = defaultsJobs
		}
	}

	// Stdin can only be consumed once, however many repeat passes there
	// are, so it is read here rather than per job.
	var stdinSrc []byte
	for _, f := range files {
		if !f.stdin {
			continue
		}
		if stdinSrc, err = readLimitedFrom(os.Stdin); err != nil {
			reportReadError(stderr, "-", err)
			return silentExit(ExitParse)
		}
		break
	}

	sink := newOutputSink(rc.output, stdout)
	defer sink.Close() //nolint:errcheck // best-effort close

	// buffered is also whether this run holds each job's log events until
	// it ends, then re-redacts the whole buffer (flushBuffer) instead of
	// writing them live: BufferedLogs tells the engine that CLI-side
	// safety net exists, so it can allow a `redact` capture in verbose
	// mode instead of rejecting it outright (its own per-event redaction
	// only knows the secrets registered so far). Setting it without
	// actually buffering here would be a real leak, not just a stricter
	// check, so the two must stay tied to the same condition. It is
	// rc.parallel (--test or --parallel), not rc.jobs > 1: the reference
	// CLI's parallel runner, and everything that comes with it (buffered
	// per-job logs, the progress bar), applies whenever that runner is
	// used at all, even at --jobs 1.
	buffered := rc.parallel
	rc.engine.BufferedLogs = buffered
	runner := engine.NewRunner(rc.engine)

	// sonde.yaml discovery warnings (a candidate skipped for failing the
	// ownership/permission check) are printed once the run's own secret
	// registry exists, redacted like any other stderr text even though
	// they only ever name a path, never a secret value.
	for _, w := range projectWarnings {
		newEventLogger(stderr, rc.color, false).writePrefixedMessage(ansiYellowBold, "warning", runner.Redact(w))
	}

	ctx := cmd.Context()
	// haltSiblings lets the Finished hook below stop every in-flight
	// sibling job immediately, on top of whatever external stop (Ctrl-C)
	// already applies: a parse error or unreadable file must abort the
	// whole run right away (upstream parity), not merely stop scheduling
	// new jobs the way a Finished hook returning false alone does.
	stop, haltSiblings := mergeStop(stopFromContext(ctx))

	worst := ExitOK
	aborted := false
	total, succeeded, requests := 0, 0, 0
	start := time.Now()
	cookies := &cookieJarAccumulator{}
	// maxSeqFile is the input file of whichever job has the highest seq
	// finished so far, for the cookie jar's "Cookies for file <FILE>"
	// header: a plain running max, not a per-seq history (that would grow
	// without bound across an infinite --repeat).
	maxSeq, maxSeqFile := -1, ""
	var curlBySeq map[int][]string
	if rc.curlFile != "" {
		curlBySeq = map[int][]string{}
	}
	// Report results are kept only when a --report-* flag was given, so
	// memory stays flat for a plain run over any number of files.
	var resultsBySeq map[int]*engine.UnitResult
	if rc.hasReport() {
		resultsBySeq = map[int]*engine.UnitResult{}
	}
	jobBufs := map[int]*jobBuffer{}

	barMode := newProgressMode(rc.test, rc.progressBar, isTerminalWriter(stderr))
	units := len(files)
	if rc.data != nil {
		units *= rc.data.rows
		if rc.data.rows == 0 {
			newEventLogger(stderr, rc.color, false).writePrefixedMessage(ansiYellowBold, "warning", rc.data.path+": no data rows")
		}
	}
	pb := newProgressBar(barMode, rc.color, progressMaxWidth(), newJobTotal(units, rc.repeat))

	hooks := engine.Hooks{
		Started: func(seq int, job engine.Job) (func(engine.Event), io.Writer) {
			var handle func(engine.Event)
			var out io.Writer
			if !buffered {
				logger := newEventLogger(stderr, rc.color, rc.errorFormat == "long").withProgress(pb)
				handle, out = logger.handle, stdout
			} else {
				jb := &jobBuffer{}
				jb.logger = newEventLogger(&jb.stderr, rc.color, rc.errorFormat == "long")
				jobBufs[seq] = jb
				handle, out = jb.logger.handle, &jb.stdout
			}
			// The progress bar is always drawn straight to the live
			// stderr, whether or not this job's own output is buffered
			// for later (parallel mode): it is the run's own status
			// display, not part of any one file's output.
			return func(ev engine.Event) {
				if es, ok := ev.(engine.EntryStarted); ok {
					pb.onEntryStarted(stderr, seq, job.Label(), es.Index, es.Last, es.Retry)
				}
				handle(ev)
			}, out
		},
		Finished: func(seq int, job engine.Job, res *engine.UnitResult, jobErr error) bool {
			if aborted {
				// A parse or read error already ended the run: the jobs
				// still finishing (other rows of the same file, siblings
				// stopped early) print nothing, as if the run had exited.
				return false
			}
			total++
			// Unconditional, whichever branch below this job ends up
			// taking: the reference CLI's own Completed handler always
			// clears the bar first, so every completion line prints on a
			// clean line and the run's "completed" counter advances.
			pb.onFinished(stderr, seq)
			if seq > maxSeq {
				maxSeq, maxSeqFile = seq, job.Name
			}
			jb := jobBufs[seq]
			delete(jobBufs, seq)
			errW := io.Writer(stderr)
			if jb != nil {
				errW = &jb.stderr
			}

			if jobErr != nil {
				flushBuffer(stderr, jb, runner.Redact)
				if _, isPathErr := jobErr.(*fs.PathError); isPathErr { //nolint:errorlint // deliberately not errors.As: a *fs.PathError wrapped inside a setup error (e.g. a bad --cookie file) must NOT match here
					// The file itself could not be read: matches the
					// upstream CLI's own behavior, abort the whole run
					// immediately (no reports, no summary — see below),
					// ending every in-flight sibling at its next entry
					// boundary too, not just stopping new scheduling.
					reportReadError(stderr, job.Name, jobErr)
					worst = worstCode(worst, ExitParse)
					aborted = true
					haltSiblings()
					return false
				}
				// A setup failure other than reading the file (e.g. an
				// unreadable --cookie file): this file failed, but the
				// run continues to the next one.
				_, _ = fmt.Fprintf(stderr, "error: %s\n\n", runner.Redact(jobErr.Error()))
				worst = worstCode(worst, ExitRuntime)
				return true
			}
			redact := res.Redact
			if res.ParseError != nil {
				flushBuffer(stderr, jb, redact)
				shown := trimBOM(res.Source)
				if rc.color {
					writePrefixedError(stderr, res.ParseError.RenderColor(job.Name, shown), true)
				} else {
					writePrefixedError(stderr, res.ParseError.Render(job.Name, shown), false)
				}
				worst = worstCode(worst, ExitParse)
				aborted = true
				// Upstream parity: a parse error aborts the whole run
				// immediately, with no reports and no summary (see the
				// worst == ExitParse check after RunAll), so there is
				// nothing to add to resultsBySeq here any more.
				haltSiblings()
				return false
			}
			if rc.errorFormat == "long" {
				writeLongFormatErrors(errW, res, rc.color)
			}
			if resultsBySeq != nil {
				resultsBySeq[seq] = res
			}
			flushBuffer(stderr, jb, redact)

			if res.Success {
				succeeded++
			}
			for _, e := range res.Entries {
				requests += len(e.Calls)
				if curlBySeq != nil && e.Curl != "" {
					curlBySeq[seq] = append(curlBySeq[seq], res.Redact(e.Curl))
				}
			}
			cookies.add(res.Cookies, res.Redact)

			outSink := sink
			if jb != nil {
				outSink = newBufferSink(&jb.fileOutput)
			}
			if werr := writeFileOutput(rc, outSink, runner, res); werr != nil {
				var oe *outputError
				if errors.As(werr, &oe) {
					flushJobOutput(stdout, sink, jb)
					_, _ = fmt.Fprintf(stderr, "error: %s\n", oe.rendered)
					worst = worstCode(worst, ExitRuntime)
				} else {
					_, _ = fmt.Fprintf(stderr, "error: Issue writing to %s: %v\n\n", outputName(rc.output), werr) //nolint:staticcheck,revive // kept for CLI message-format compatibility
					worst = worstCode(worst, ExitUndefined)
					return false
				}
			} else {
				flushJobOutput(stdout, sink, jb)
			}

			if rc.test {
				printTestLine(stderr, rc.color, res)
			}
			worst = worstCode(worst, classifyResult(res))
			return true
		},
	}

	if buffered && rc.engine.Verbosity >= engine.Verbose {
		newEventLogger(stderr, rc.color, false).writeStar(fmt.Sprintf("Parallel run using %d workers", rc.jobs), false)
	}
	var dataErr error
	runner.RunAll(ctx, stop, buildJobs(files, stdinSrc, rc.repeat, extras, rc.data, &dataErr), rc.jobs, hooks)
	if dataErr != nil {
		return NewExitError(ExitUsage, dataErr)
	}
	// Upstream parity: a parse error or unreadable input file aborts the
	// run right here — no curl export, no reports, no cookie jar, no
	// --test summary. The Finished hook already printed the error (and
	// any job that had already completed printed its own line, same as
	// upstream); everything below this is run-level, and none of it runs.
	if worst == ExitParse {
		return silentExit(ExitParse)
	}
	// Duration excludes report writing, matching the reference CLI (it
	// takes its own elapsed-time snapshot before export_results runs).
	duration := time.Since(start)
	verbose := rc.engine.Verbosity >= engine.Verbose

	// Curl export, then the report files, then the cookie jar, in the
	// reference CLI's own export_results order; this whole block runs
	// before the --test summary line, also matching upstream (the summary
	// is computed and printed only after reports are written).
	if curlBySeq != nil {
		if err := writeCurlFile(rc.curlFile, curlBySeq, runner); err != nil {
			return NewExitError(ExitUndefined, fmt.Errorf("Issue writing to %s: %v", rc.curlFile, err)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	}
	if resultsBySeq != nil {
		if err := writeReports(stderr, rc.color, verbose, rc, orderedResults(resultsBySeq), runner.Redact); err != nil {
			// The reference implementation's own report error (e.g. an
			// existing report file that is not valid TAP/JUnit/JSON to
			// merge into) is printed as-is, not wrapped in "Issue writing
			// to ...": it already names the file and the problem.
			return NewExitError(ExitUndefined, err)
		}
	}
	if rc.cookieJar != "" {
		logWriting(stderr, rc.color, verbose, "cookies", rc.cookieJar)
		if err := writeCookieJar(rc.cookieJar, maxSeqFile, cookies.cookies(), runner); err != nil {
			return NewExitError(ExitUndefined, fmt.Errorf("Issue writing to %s: %v", rc.cookieJar, err)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	}

	if rc.test {
		_, _ = fmt.Fprint(stderr, testSummary(total, succeeded, requests, duration))
	}

	if worst != ExitOK {
		return silentExit(worst)
	}
	return nil
}

// mergeStop returns a channel that closes as soon as either external does
// (an outer Ctrl-C) or haltNow is called, plus haltNow itself (safe to
// call more than once, and from any goroutine). A run's Finished hook uses
// haltNow so a parse error or unreadable input file can abort every
// in-flight sibling job immediately — the same effect an external stop
// has — on top of whatever Finished returning false already does on its
// own (stop scheduling further jobs, but let running ones finish).
func mergeStop(external <-chan struct{}) (merged <-chan struct{}, haltNow func()) {
	out := make(chan struct{})
	internal := make(chan struct{})
	var once sync.Once
	haltNow = func() { once.Do(func() { close(internal) }) }
	go func() {
		select {
		case <-external:
		case <-internal:
		}
		close(out)
	}()
	return out, haltNow
}

// jobBuffer holds one running job's stderr (log events) and its two
// separate stdout streams while it runs in parallel mode, so all three can
// be flushed once the job finishes, in the upstream parallel runner's own
// order (stderr, then stdout) — matching sequential mode's live output
// byte for byte, just delayed to completion. Buffering the stderr also
// lets it be redacted once more at flush time, catching a secret a
// `redact` capture only registered partway through.
type jobBuffer struct {
	stderr bytes.Buffer
	// stdout receives this job's own `output: -`/entry-level writes
	// (unitIO's stdout, returned to the engine from Started): always
	// flushed to the process's real stdout, never to an -o FILE target,
	// matching sequential mode where Started returns the real stdout
	// directly.
	stdout bytes.Buffer
	// fileOutput receives writeFileOutput's own result — the CLI's
	// default last-response-body output, or --json — flushed to whatever
	// sink the CLI is writing to (-o FILE, or stdout when none was
	// given). Kept separate from stdout so the two never mix: only this
	// one is ever redirected by -o.
	fileOutput bytes.Buffer
	logger     *eventLogger
}

// flushBuffer writes jb's buffered stderr to w, redacted; jb may be nil
// (sequential mode, already written directly).
func flushBuffer(w io.Writer, jb *jobBuffer, redact func(string) string) {
	if jb == nil || jb.stderr.Len() == 0 {
		return
	}
	_, _ = w.Write([]byte(redact(jb.stderr.String())))
	jb.stderr.Reset()
}

// flushJobOutput writes jb's two stdout streams to their respective
// destinations: entry-level `output: -` writes always to stdout (the
// process's real standard output), and the CLI's own file output to sink
// (-o FILE, or stdout when none was given). jb may be nil (sequential
// mode, already written directly to each destination as it happened).
func flushJobOutput(stdout io.Writer, sink *outputSink, jb *jobBuffer) {
	if jb == nil {
		return
	}
	if jb.stdout.Len() > 0 {
		_, _ = stdout.Write(jb.stdout.Bytes())
	}
	if jb.fileOutput.Len() > 0 {
		if w, err := sink.writer(); err == nil {
			_, _ = w.Write(jb.fileOutput.Bytes())
		}
	}
}

// writeCurlFile writes --curl FILE: every executed request's curl
// command, one per line, in original file order (curlBySeq's keys are
// completion order; sorting by seq restores it), redacted with the run's
// final secret union. Matches the upstream CLI's own report/curl.rs:
// truncate-create once, not incremental like -o.
func writeCurlFile(path string, curlBySeq map[int][]string, runner *engine.Runner) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	seqs := make([]int, 0, len(curlBySeq))
	for seq := range curlBySeq {
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)
	var b bytes.Buffer
	for _, seq := range seqs {
		for _, cmd := range curlBySeq[seq] {
			b.WriteString(runner.Redact(cmd))
			b.WriteByte('\n')
		}
	}
	return os.WriteFile(path, b.Bytes(), 0o600) //nolint:gosec // G304: the command line names the file
}

// jobExtras holds one input file's sonde.yaml-resolved variables and
// secrets. They are set on that file's engine.Job below the runner's
// Options: an Options value of the same name wins (docs/sonde-yaml.md
// precedence), so this is a pure fallback layer.
type jobExtras struct {
	vars    map[string]any
	secrets map[string]string
	// project is the file's sonde.yaml.
	project *config.Project
	// validator checks the file's responses against its contract (nil:
	// the run's, from --openapi).
	validator engine.ResponseValidator
}

// resolveContracts loads the OpenAPI specs of the run, once each: the
// run's (--openapi) and those of the files' sonde.yaml projects.
func resolveContracts(cmd *cobra.Command, o *runOptions, rc *runContext, extras map[string]jobExtras, files []*inputFile) error {
	c := newContracts(cmd, o.openAPI)
	v, err := c.forRun()
	if err != nil {
		return err
	}
	rc.engine.Validator = v
	for _, f := range files {
		e, ok := extras[f.name]
		if !ok {
			continue
		}
		if e.validator, err = c.forFile(f.name, e.project); err != nil {
			return err
		}
		extras[f.name] = e
	}
	return nil
}

// resolveJobExtras discovers each real (non-stdin) file's sonde.yaml,
// selects its environment (--env, then SONDE_ENV, then that project's own
// defaults.env) and resolves its variables/secrets, returning them keyed
// by file name. defaultsJobs is the first project found's defaults.jobs
// (0 if none set anywhere), used by runMain as a --jobs fallback. warnings
// are every discovery warning the search collected (a candidate sonde.yaml
// skipped for failing the ownership/permission check — see
// config.ProjectCache.TakeWarnings); the caller is responsible for
// printing them, redacted like any other stderr text.
//
// rc.configFile, when set, names the sonde.yaml used for every file,
// skipping discovery entirely. A file with no sonde.yaml above it (and no
// --config) is simply left out of the result: it runs with no sonde.yaml
// variables or secrets, not an error.
func resolveJobExtras(files []*inputFile, rc *runContext, env config.Env) (extras map[string]jobExtras, defaultsJobs int, warnings []string, err error) {
	extras = make(map[string]jobExtras)
	loaded := map[string]*config.Project{}
	var cache *config.ProjectCache
	if rc.configFile == "" {
		cache = config.NewProjectCache()
	}
	haveDefaultsJobs := false

	for _, f := range files {
		if f.stdin {
			continue
		}

		path := rc.configFile
		if path == "" {
			found, ok, ferr := cache.FindProject(filepath.Dir(f.name))
			if ferr != nil {
				return nil, 0, nil, NewExitError(ExitUsage, ferr)
			}
			if !ok {
				// No sonde.yaml applies to this file at all. An explicit
				// --env has nothing to select an environment from, which
				// is an error; SONDE_ENV alone (rc.env stays "" here,
				// since SelectEnv's flag argument is exactly rc.env) is
				// silently ignored instead, matching a plain run with no
				// sonde.yaml anywhere.
				if rc.env != "" {
					return nil, 0, nil, NewExitError(ExitUsage, fmt.Errorf("%s: no sonde.yaml found for environment %q", f.name, rc.env))
				}
				continue
			}
			path = found
		}

		proj, ok := loaded[path]
		if !ok {
			var lerr error
			proj, lerr = config.LoadProject(path)
			if lerr != nil {
				return nil, 0, nil, NewExitError(ExitUsage, lerr)
			}
			loaded[path] = proj
		}
		if !haveDefaultsJobs && proj.Defaults.Jobs > 0 {
			defaultsJobs, haveDefaultsJobs = proj.Defaults.Jobs, true
		}

		envName := config.SelectEnv(rc.env, env["SONDE_ENV"], proj.Defaults.Env)
		vars, secrets, rerr := proj.Resolve(envName)
		if rerr != nil {
			return nil, 0, nil, NewExitError(ExitUsage, rerr)
		}
		e := jobExtras{secrets: secrets, project: proj}
		if len(vars) > 0 {
			e.vars = make(map[string]any, len(vars))
			for name, v := range vars {
				e.vars[name] = v
			}
		}
		extras[f.name] = e
	}
	if cache != nil {
		warnings = cache.TakeWarnings()
	}
	return extras, defaultsJobs, warnings, nil
}

// orderedResults returns m's values ordered by seq: RunAll's Finished hook
// fires in completion order, not necessarily the input files' original
// order, but reports must reflect that original order (same reasoning as
// writeCurlFile's seq sort above).
func orderedResults(m map[int]*engine.UnitResult) []*engine.UnitResult {
	seqs := make([]int, 0, len(m))
	for seq := range m {
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)
	out := make([]*engine.UnitResult, len(seqs))
	for i, seq := range seqs {
		out[i] = m[seq]
	}
	return out
}

// writeReports appends results to every --report-* file rc names, in the
// same order the reference CLI's own export_results does (JUnit, TAP,
// HTML, JSON — curl and the cookie jar are logged and written separately,
// around this call, matching that same function). Each writer
// (internal/report) reads and merges any content already there, so
// reports accumulate across separate invocations, not just within one
// run. A parse-error result is included: upstream still writes reports
// for files that ran before, or as, a parse error. verbose gates the
// "Writing ... report to ..." line each one prints first, matching
// BaseLogger::debug, which is silent below --verbose.
func writeReports(stderr io.Writer, color, verbose bool, rc *runContext, results []*engine.UnitResult, redact func(string) string) error {
	if rc.reportJUnit != "" {
		logWriting(stderr, color, verbose, "JUnit report", rc.reportJUnit)
		if err := report.WriteJUnit(rc.reportJUnit, results, redact); err != nil {
			return err
		}
	}
	if rc.reportTAP != "" {
		logWriting(stderr, color, verbose, "TAP report", rc.reportTAP)
		if err := report.WriteTAP(rc.reportTAP, results, redact); err != nil {
			return err
		}
	}
	if rc.reportHTML != "" {
		logWriting(stderr, color, verbose, "HTML report", rc.reportHTML)
		if err := report.WriteHTML(rc.reportHTML, results, redact); err != nil {
			return err
		}
	}
	if rc.reportJSON != "" {
		logWriting(stderr, color, verbose, "JSON report", rc.reportJSON)
		if err := report.WriteJSON(rc.reportJSON, results, redact); err != nil {
			return err
		}
	}
	return nil
}

// logWriting writes "* Writing <what> to <path>" when verbose, matching
// the reference CLI's BaseLogger::debug calls around each report/cookie
// jar export: silent below --verbose, one line at --verbose and above.
func logWriting(stderr io.Writer, color, verbose bool, what, path string) {
	if !verbose {
		return
	}
	newEventLogger(stderr, color, false).writeStar(fmt.Sprintf("Writing %s to %s", what, path), false)
}

// buildJobs returns the lazy sequence of jobs RunAll runs: the resolved
// input files, repeated repeat times (-1: forever). A real file's Source
// is left nil so RunAll reads it (lazily, once per attempt); stdin's
// Source is the bytes read once in runMain, reused for every repeat. A
// non-stdin file's sonde.yaml variables/secrets, if any, come from extras.
//
// With a data file, each file runs once per row; a data file that fails
// to read midway ends the sequence and sets *dataErr.
func buildJobs(files []*inputFile, stdinSrc []byte, repeat int, extras map[string]jobExtras, data *dataRun, dataErr *error) iter.Seq[engine.Job] {
	return func(yield func(engine.Job) bool) {
		for pass := 0; repeat < 0 || pass < repeat; pass++ {
			yielded := false
			for _, f := range files {
				job := engine.Job{Name: f.name}
				if f.stdin {
					job = engine.Job{Name: "-", Source: stdinSrc}
				} else if e, ok := extras[f.name]; ok {
					job.Variables = e.vars
					job.Secrets = e.secrets
					job.Validator = e.validator
				}
				if data == nil {
					if !yield(job) {
						return
					}
					yielded = true
					continue
				}
				if job.Source == nil {
					// Read once for all the rows; on failure the jobs
					// read it again and report the error.
					job.Source = readSourceOnce(f.name)
				}
				stopped, err := data.each(func(row *engine.Row) bool {
					job.Row = row
					yielded = true
					return yield(job)
				})
				if err != nil {
					*dataErr = err
					return
				}
				if stopped {
					return
				}
			}
			if !yielded {
				return // no rows: another pass would yield nothing either
			}
		}
	}
}

// cookieJarAccumulator merges the end-of-run cookie stores of every file a
// run processed into one ordered list: a cookie's first appearance fixes
// its position, later files' values for the same domain/path/name replace
// it in place, matching a Netscape cookie jar's own semantics.
type cookieJarAccumulator struct {
	order []string
	byKey map[string]engine.Cookie
}

// add merges cs, their values redacted with redact.
func (a *cookieJarAccumulator) add(cs []engine.Cookie, redact func(string) string) {
	if a.byKey == nil {
		a.byKey = map[string]engine.Cookie{}
	}
	for _, c := range cs {
		k := c.Domain + "\x00" + c.Path + "\x00" + c.Name
		if _, ok := a.byKey[k]; !ok {
			a.order = append(a.order, k)
		}
		c.Value = redact(c.Value)
		a.byKey[k] = c
	}
}

func (a *cookieJarAccumulator) cookies() []engine.Cookie {
	out := make([]engine.Cookie, 0, len(a.order))
	for _, k := range a.order {
		out = append(out, a.byKey[k])
	}
	return out
}

// worstCode keeps the most severe of two exit codes, in the documented
// severity order: parse error > runtime error > assert failure > success.
func worstCode(a, b int) int {
	rank := func(c int) int {
		switch c {
		case ExitParse:
			return 3
		case ExitRuntime:
			return 2
		case ExitAssert:
			return 1
		}
		return 0
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// classifyResult maps a finished run to the exit code it contributes:
// ExitOK on success, ExitAssert when every decisive error is an assert
// failure, ExitRuntime when at least one is not.
func classifyResult(res *engine.UnitResult) int {
	if res.ParseError != nil {
		return ExitParse
	}
	if res.Success {
		return ExitOK
	}
	for _, e := range res.Errors() {
		if !e.Assert {
			return ExitRuntime
		}
	}
	return ExitAssert
}

// trimBOM drops a leading UTF-8 BOM, matching the position skipped by the
// parser (see internal/cli/input.go).
func trimBOM(src []byte) []byte {
	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		return src[3:]
	}
	return src
}

func outputName(output string) string {
	if output == "" {
		return "-"
	}
	return output
}

// printTestLine prints a one-line per-file status in --test mode, colored
// like the reference CLI's own ParProgress::print_completed when color is
// on: a bold green "Success" or bold red "Failure", the filename bold.
func printTestLine(stderr io.Writer, color bool, res *engine.UnitResult) {
	status, style := "Success", ansiGreenBold
	if !res.Success {
		status, style = "Failure", ansiRedBold
	}
	n := 0
	for _, e := range res.Entries {
		n += len(e.Calls)
	}
	if !color {
		_, _ = fmt.Fprintf(stderr, "%s %s (%d request(s) in %d ms)\n", status, res.Label(), n, res.Duration.Milliseconds())
		return
	}
	_, _ = fmt.Fprintf(stderr, "%s%s%s %s%s%s (%d request(s) in %d ms)\n",
		style, status, ansiReset, ansiBold, res.Label(), ansiReset, n, res.Duration.Milliseconds())
}

// testSummary is --test's final block: the documented wording and number
// formatting, reproduced exactly.
func testSummary(totalFiles, succeededFiles, totalRequests int, duration time.Duration) string {
	failed := totalFiles - succeededFiles
	var successPct, failedPct float64
	if totalFiles > 0 {
		successPct = 100 * float64(succeededFiles) / float64(totalFiles)
		failedPct = 100 * float64(failed) / float64(totalFiles)
	}
	ms := duration.Milliseconds()
	var rate float64
	if ms > 0 {
		rate = 1000 * float64(totalRequests) / float64(ms)
	}
	return fmt.Sprintf(
		"--------------------------------------------------------------------------------\n"+
			"Executed files:    %d\n"+
			"Executed requests: %d (%.1f/s)\n"+
			"Succeeded files:   %d (%.1f%%)\n"+
			"Failed files:      %d (%.1f%%)\n"+
			"Duration:          %d ms (%s)\n\n",
		totalFiles, totalRequests, rate, succeededFiles, successPct, failed, failedPct, ms, formatDurationHMS(duration))
}

func formatDurationHMS(d time.Duration) string {
	total := d.Milliseconds()
	hours := total / 3600000
	minutes := (total % 3600000) / 60000
	seconds := (total % 60000) / 1000
	millis := total % 1000
	return fmt.Sprintf("%dh:%dm:%ds:%dms", hours, minutes, seconds, millis)
}

// inputFile is one file (or stdin) `sonde run` will run.
type inputFile struct {
	name  string
	stdin bool
}

// resolveInputFiles expands args and glob patterns into the files to run,
// in the upstream order: positional FILE args first (a directory expands
// to every ".hurl"/".sonde" file under it), then each --glob pattern's
// matches, in the order the flags were given. Every source is checked to
// exist before anything runs; no FILE and no --glob at all means read a
// single input from stdin.
func resolveInputFiles(args []string, globs []string) ([]*inputFile, error) {
	var files []*inputFile
	for _, a := range args {
		if a == "-" {
			files = append(files, &inputFile{stdin: true})
			continue
		}
		expanded, err := expandInputArg(a)
		if err != nil {
			return nil, err
		}
		files = append(files, expanded...)
	}
	for _, pattern := range globs {
		matches, err := globFiles(pattern)
		if err != nil {
			return nil, NewExitError(ExitUsage, fmt.Errorf("invalid --glob pattern %q: %w", pattern, err))
		}
		if len(matches) == 0 {
			return nil, cannotAccessErr(pattern)
		}
		for _, m := range matches {
			files = append(files, &inputFile{name: m})
		}
	}
	if len(files) == 0 {
		files = append(files, &inputFile{stdin: true})
	}
	return files, nil
}

// expandInputArg resolves one positional FILE argument: it must exist, and
// a directory expands to every ".hurl"/".sonde" file under it (recursive,
// in the deterministic order filepath.WalkDir visits them — lexical order
// within each directory).
func expandInputArg(a string) ([]*inputFile, error) {
	info, err := os.Stat(a)
	if err != nil {
		return nil, cannotAccessErr(a)
	}
	if !info.IsDir() {
		return []*inputFile{{name: a}}, nil
	}
	var files []*inputFile
	err = filepath.WalkDir(a, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(d.Name()); ext == ".hurl" || ext == ".sonde" {
			files = append(files, &inputFile{name: path})
		}
		return nil
	})
	if err != nil {
		return nil, NewExitError(ExitUsage, err)
	}
	return files, nil
}

// cannotAccessErr reports a positional FILE or --glob pattern that named
// nothing on disk, matching the upstream CLI's own message.
func cannotAccessErr(path string) error {
	return NewExitError(ExitUsage, fmt.Errorf("Cannot access '%s': No such file or directory", path)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
}

// readSourceOnce reads a request file shared by many jobs; nil when it
// cannot be read or is too large (each job then reports it).
func readSourceOnce(name string) []byte {
	f, err := os.Open(name) //nolint:gosec // G304: an input file named on the command line
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only
	src, err := readLimitedFrom(f)
	if err != nil {
		return nil
	}
	return src
}

// readLimitedFrom reads all of r but stops past syntax.MaxFileSize.
func readLimitedFrom(r io.Reader) ([]byte, error) {
	src, err := io.ReadAll(io.LimitReader(r, syntax.MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(src) > syntax.MaxFileSize {
		return nil, fmt.Errorf("file is larger than %d MiB", syntax.MaxFileSize>>20)
	}
	return src, nil
}
