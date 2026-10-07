// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/datarow"
	"github.com/nhtera/sonde/internal/runplan"
)

// runContext is everything a run of one or more files needs: the engine
// options every file shares, and the CLI-only settings that shape output.
type runContext struct {
	engine engine.Options

	// colorOut and colorErr are whether output to stdout (bodies, -i
	// headers) and to stderr (logs, errors, progress, summaries) is
	// coloured. Each defaults to whether its own stream is a terminal.
	colorOut    bool
	colorErr    bool
	stdoutTTY   bool
	include     bool
	jsonOutput  bool
	noOutput    bool
	pretty      bool
	output      string
	errorFormat string // "short" or "long"
	cookieJar   string
	curlFile    string
	reportHTML  string
	reportJSON  string
	reportJUnit string
	reportTAP   string
	test        bool
	progressBar bool
	glob        []string
	repeat      int // 1: once (default), -1: infinite
	// parallel is whether this run uses the parallel runner architecture
	// at all (--test or --parallel), independent of the worker count: see
	// its assignment in buildRunContext for why this, not jobs > 1, is
	// what selects buffered per-job logs and the progress bar.
	parallel bool
	// jobs is the number of files RunAll may run at once: 1 (sequential)
	// unless --parallel or --test/`sonde test`, in which case it is
	// --jobs, or the number of CPUs when --jobs was not given.
	jobs int
	// env and configFile are --env/--config, resolved against sonde.yaml
	// per input file in run.go (env also falls back to SONDE_ENV, then
	// each file's nearest project's defaults.env).
	env        string
	configFile string
	// data is the --data file, checked in full (nil: none).
	data *datarow.Run
	// plan builds the run's jobs (runplan).
	plan *runplan.Plan
}

// hasReport reports whether any --report-* flag was given: results are
// only retained for the report writers when at least one is (memory flat
// otherwise, per the plan).
func (rc *runContext) hasReport() bool {
	return rc.reportHTML != "" || rc.reportJSON != "" || rc.reportJUnit != "" || rc.reportTAP != ""
}

// buildRunContext resolves every flag, environment variable and config
// file setting into a runContext, in the documented precedence order:
// config file < env vars (HURL_*/SONDE_*) < command line flag. The run
// itself (engine options, run control) is built by runplan; the output
// settings are the CLI's.
func buildRunContext(cmd *cobra.Command, inv *runplan.Invocation, env config.Env, stdout, stderr io.Writer) (*runContext, error) {
	// A bad --error-format is reported first, as before the config file
	// joined the run plan; the plan then reads the config file before
	// any other setting.
	if changed(cmd, "error-format") {
		if _, err := resolveErrorFormat(cmd, inv, env, ""); err != nil {
			return nil, err
		}
	}
	plan, err := runplan.New(inv, env, currentBuildInfo().Version)
	if err != nil {
		return nil, planError(err)
	}
	file := plan.Config

	rc := &runContext{}
	rc.stdoutTTY = isTerminal(stdout)
	rc.colorOut = resolveColor(cmd, env, rc.stdoutTTY, file.Color)
	rc.colorErr = resolveColor(cmd, env, isTerminal(stderr), file.Color)
	rc.include = runplan.ResolveBool(inv, "include", "INCLUDE", inv.Include, env)
	rc.jsonOutput = runplan.ResolveBool(inv, "json", "JSON", inv.JSON, env)
	// HURL_NO_OUTPUT=false leaves the config file's --no-output on, as
	// upstream does.
	noOutput := runplan.ResolveBool(inv, "no-output", "NO_OUTPUT", inv.NoOutput, env) ||
		(!changed(cmd, "no-output") && file.NoOutput)
	rc.errorFormat, err = resolveErrorFormat(cmd, inv, env, file.ErrorFormat)
	if err != nil {
		return nil, err
	}
	rc.cookieJar = inv.CookieJar
	rc.curlFile = inv.Curl
	rc.reportHTML = inv.ReportHTML
	rc.reportJSON = inv.ReportJSON
	rc.reportJUnit = inv.ReportJUnit
	rc.reportTAP = inv.ReportTAP
	rc.env = inv.Env
	rc.configFile = inv.Config
	rc.progressBar = inv.ProgressBar
	rc.pretty = resolvePretty(cmd, inv, env, rc.stdoutTTY, file.Pretty)
	rc.output = inv.Output
	rc.glob = inv.Glob
	rc.plan = plan
	rc.test = plan.Test
	if rc.test {
		noOutput = true
	}
	rc.noOutput = noOutput
	rc.parallel = plan.Parallel
	rc.jobs = plan.Workers
	rc.repeat = plan.Repeat
	rc.data = plan.Data
	rc.engine = plan.Options
	rc.engine.Stdout = stdout
	// `output: -` entries get the same rendering as the default output.
	if rc.pretty {
		colorOut := rc.colorOut
		rc.engine.StdoutBody = func(resp *exchange.Response, body []byte) []byte {
			return prettyBody(body, resp, colorOut)
		}
	}
	return rc, nil
}

// planError gives an error of runplan its exit code: an unsupported option
// is a runtime error, any other a usage error.
func planError(err error) error {
	var ue *runplan.UnsupportedError
	if errors.As(err, &ue) {
		return NewExitError(ExitRuntime, err)
	}
	return NewExitError(ExitUsage, err)
}

// resolveErrorFormat resolves --error-format: the flag, then
// HURL_ERROR_FORMAT, then the config file's (file, "" when unset), then
// "short".
func resolveErrorFormat(cmd *cobra.Command, inv *runplan.Invocation, env config.Env, file string) (string, error) {
	if changed(cmd, "error-format") {
		if inv.ErrorFormat != "short" && inv.ErrorFormat != "long" {
			return "", NewExitError(ExitUsage, fmt.Errorf("invalid value '%s' for error-format [possible values: long, short]", inv.ErrorFormat))
		}
		return inv.ErrorFormat, nil
	}
	if v, ok, err := env.ErrorFormat(); ok {
		if err != nil {
			return "", NewExitError(ExitUsage, err)
		}
		return v, nil
	}
	if file != "" {
		return file, nil
	}
	return "short", nil
}

// resolvePretty resolves --pretty/--no-pretty: the flags, then the
// environment, then the config file's (file, nil when unset), then
// whether stdout is a terminal.
func resolvePretty(cmd *cobra.Command, inv *runplan.Invocation, env config.Env, stdoutTTY bool, file *bool) bool {
	if changed(cmd, "pretty") && inv.Pretty {
		return true
	}
	if changed(cmd, "no-pretty") && inv.NoPretty {
		return false
	}
	if v, ok := env.Bool("PRETTY"); ok {
		return v
	}
	if v, ok := env.Bool("NO_PRETTY"); ok {
		return !v
	}
	if file != nil {
		return *file
	}
	return stdoutTTY
}

// resolveColor decides whether one stream is coloured: whether it is a
// terminal (tty), overridden by the config file's --color/--no-color
// (file, nil when unset), then by NO_COLOR and HURL_COLOR/SONDE_COLOR,
// then by --color, then by --no-color.
func resolveColor(cmd *cobra.Command, env config.Env, tty bool, file *bool) bool {
	base := tty
	if file != nil {
		base = *file
	}
	color := env.Color(base)
	if changed(cmd, "color") {
		color = true
	}
	// --no-color is checked last so it always wins when both are given,
	// matching the upstream CLI's own silent priority (no hard usage
	// error for the combination).
	if changed(cmd, "no-color") {
		color = false
	}
	return color
}
