// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/cookiejar"
	"github.com/nhtera/sonde/internal/report"
	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/testsummary"
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

	rc, err := buildRunContext(cmd, o.invocation(cmd, args, forceTest), env, stdout)
	if err != nil {
		return err
	}

	files, err := resolveInputFiles(args, rc.glob)
	if err != nil {
		return err
	}
	inputs := make([]runplan.Input, len(files))
	for i, f := range files {
		inputs[i] = runplan.Input{Name: f.name, Stdin: f.stdin}
	}
	if err := rc.plan.Resolve(cmd.Context(), inputs); err != nil {
		return NewExitError(ExitUsage, err)
	}
	rc.engine.Validator = rc.plan.Options.Validator
	rc.jobs = rc.plan.Workers

	// Stdin can only be consumed once, however many repeat passes there
	// are, so it is read here rather than per job.
	var stdinSrc []byte
	for _, f := range files {
		if !f.stdin {
			continue
		}
		if stdinSrc, err = runplan.ReadLimited(os.Stdin); err != nil {
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
	for _, w := range rc.plan.Warnings {
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
	cookies := &cookiejar.Accumulator{}
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
		units *= rc.data.Rows()
		if rc.data.Rows() == 0 {
			newEventLogger(stderr, rc.color, false).writePrefixedMessage(ansiYellowBold, "warning", rc.data.Path()+": no data rows")
		}
	}
	pb := newProgressBar(barMode, rc.color, progressMaxWidth(), newJobTotal(units, rc.repeat))

	opt := engine.RunAllOptions{
		Parallel: rc.jobs,
		Stop:     stop,
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
				if rc.color {
					writePrefixedError(stderr, res.ParseError.RenderColor(), true)
				} else {
					writePrefixedError(stderr, res.ParseError.Render(), false)
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
			cookies.Add(res.Cookies, res.Redact)

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
				_, _ = fmt.Fprint(stderr, testsummary.Line(res, rc.color))
			}
			worst = worstCode(worst, classifyResult(res))
			return true
		},
	}

	if buffered && rc.engine.Verbosity >= engine.Verbose {
		newEventLogger(stderr, rc.color, false).writeStar(fmt.Sprintf("Parallel run using %d workers", rc.jobs), false)
	}
	var dataErr error
	runner.RunAll(ctx, rc.plan.Jobs(stdinSrc, &dataErr), opt)
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
		if err := writeCookieJar(rc.cookieJar, maxSeqFile, cookies.Cookies(), runner.Redact); err != nil {
			return NewExitError(ExitUndefined, fmt.Errorf("Issue writing to %s: %v", rc.cookieJar, err)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	}

	if rc.test {
		_, _ = fmt.Fprint(stderr, testsummary.Summary(total, succeeded, requests, duration))
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

// worstCode keeps the most severe of two exit codes, in the documented
// severity order: parse error > runtime error > assert failure > success.
func worstCode(a, b int) int {
	outcome := func(c int) testsummary.Outcome {
		switch c {
		case ExitParse:
			return testsummary.Parse
		case ExitRuntime:
			return testsummary.Runtime
		case ExitAssert:
			return testsummary.Assert
		}
		return testsummary.OK
	}
	if testsummary.Worst(outcome(a), outcome(b)) != outcome(a) {
		return b
	}
	return a
}

// classifyResult maps a finished run to the exit code it contributes:
// ExitOK on success, ExitAssert when every decisive error is an assert
// failure, ExitRuntime when at least one is not.
func classifyResult(res *engine.UnitResult) int {
	switch testsummary.Classify(res) {
	case testsummary.Parse:
		return ExitParse
	case testsummary.Runtime:
		return ExitRuntime
	case testsummary.Assert:
		return ExitAssert
	}
	return ExitOK
}

func outputName(output string) string {
	if output == "" {
		return "-"
	}
	return output
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

// writeCookieJar writes cookies to path (a command line path, not
// confined), creating its directory, each line redacted with redact.
func writeCookieJar(path, forFile string, cookies []engine.Cookie, redact func(string) string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	root, err := sandbox.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return cookiejar.Write(root, filepath.Base(path), forFile, cookies, redact)
}
