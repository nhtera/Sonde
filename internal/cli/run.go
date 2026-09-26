// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
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
			return runMain(cmd, o, args)
		},
	}
	addRunFlags(cmd, o)
	return cmd
}

// runMain is the shared body of the root command's default dispatch and
// the `run` subcommand.
func runMain(cmd *cobra.Command, o *runOptions, args []string) error {
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

	files, err := resolveInputFiles(args, rc.glob)
	if err != nil {
		return err
	}

	sink := newOutputSink(rc.output, stdout)
	defer sink.Close() //nolint:errcheck // best-effort close

	logger := newEventLogger(stderr, rc.color, rc.errorFormat == "long")
	rc.engine.OnEvent = logger.handle

	runner := engine.NewRunner(rc.engine)

	worst := ExitOK
	total, succeeded, requests := 0, 0, 0
	start := time.Now()
	repeat := rc.repeat
	cookies := &cookieJarAccumulator{}
	var lastFile string

	for pass := 0; repeat < 0 || pass < repeat; pass++ {
		if cmd.Context().Err() != nil {
			break
		}
		for _, f := range files {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			src, name, readErr := f.read()
			total++
			lastFile = name
			if readErr != nil {
				reportReadError(stderr, name, readErr)
				worst = worstCode(worst, ExitParse)
				continue
			}
			res, err := runner.RunSource(cmd.Context(), name, src)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "error: %s\n\n", runner.Redact(err.Error()))
				worst = worstCode(worst, ExitRuntime)
				continue
			}
			if res.ParseError != nil {
				shown := trimBOM(src)
				if rc.color {
					writePrefixedError(stderr, res.ParseError.RenderColor(name, shown), true)
				} else {
					writePrefixedError(stderr, res.ParseError.Render(name, shown), false)
				}
			} else if rc.errorFormat == "long" {
				writeLongFormatErrors(stderr, runner, res, rc.color)
			}
			if res.Success {
				succeeded++
			}
			for _, e := range res.Entries {
				requests += len(e.Calls)
			}
			cookies.add(res.Cookies)
			if err := writeFileOutput(rc, sink, runner, res); err != nil {
				var oe *outputError
				if errors.As(err, &oe) {
					_, _ = fmt.Fprintf(stderr, "error: %s\n", oe.rendered)
					worst = worstCode(worst, ExitRuntime)
					continue
				}
				return NewExitError(ExitUndefined, fmt.Errorf("Issue writing to %s: %v", outputName(rc.output), err)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
			}
			if rc.test {
				printTestLine(stderr, res)
			}
			worst = worstCode(worst, classifyResult(res))
		}
	}
	duration := time.Since(start)

	if rc.test {
		_, _ = fmt.Fprint(stderr, testSummary(total, succeeded, requests, duration))
	}

	if rc.cookieJar != "" {
		if err := writeCookieJar(rc.cookieJar, lastFile, cookies.cookies(), runner); err != nil {
			return NewExitError(ExitUndefined, fmt.Errorf("Issue writing to %s: %v", rc.cookieJar, err)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	}

	if worst != ExitOK {
		return silentExit(worst)
	}
	return nil
}

// cookieJarAccumulator merges the end-of-run cookie stores of every file a
// run processed into one ordered list: a cookie's first appearance fixes
// its position, later files' values for the same domain/path/name replace
// it in place, matching a Netscape cookie jar's own semantics.
type cookieJarAccumulator struct {
	order []string
	byKey map[string]engine.Cookie
}

func (a *cookieJarAccumulator) add(cs []engine.Cookie) {
	if a.byKey == nil {
		a.byKey = map[string]engine.Cookie{}
	}
	for _, c := range cs {
		k := c.Domain + "\x00" + c.Path + "\x00" + c.Name
		if _, ok := a.byKey[k]; !ok {
			a.order = append(a.order, k)
		}
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

// printTestLine prints a one-line per-file status in --test mode.
func printTestLine(stderr io.Writer, res *engine.UnitResult) {
	status := "Success"
	if !res.Success {
		status = "Failure"
	}
	n := 0
	for _, e := range res.Entries {
		n += len(e.Calls)
	}
	_, _ = fmt.Fprintf(stderr, "%s %s (%d request(s) in %d ms)\n", status, res.File, n, res.Duration.Milliseconds())
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

// inputFile is one file (or stdin) `sonde run` will read. A stdin source
// is read at most once: repeated passes (--repeat) reuse the bytes read on
// the first attempt, since a pipe cannot be rewound.
type inputFile struct {
	name    string
	stdin   bool
	didRead bool
	src     []byte
	readErr error
}

func (f *inputFile) read() ([]byte, string, error) {
	if !f.stdin {
		src, err := readLimited(f.name)
		return src, f.name, err
	}
	if !f.didRead {
		f.src, f.readErr = readLimitedFrom(os.Stdin)
		f.didRead = true
	}
	return f.src, "-", f.readErr
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
// in the deterministic order filepath.WalkDir visits them).
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
