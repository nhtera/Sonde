// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/datarow"
	"github.com/nhtera/sonde/internal/runplan"
)

// runContext is everything a run of one or more files needs: the engine
// options every file shares, and the CLI-only settings that shape output.
type runContext struct {
	engine engine.Options

	color       bool
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
func buildRunContext(cmd *cobra.Command, inv *runplan.Invocation, env config.Env, stdout io.Writer) (*runContext, error) {
	// The config file is checked first, so a broken one is reported
	// before any other setting.
	if path, ok := env.FilePath(); ok {
		if _, err := config.LoadConfigFile(path); err != nil {
			return nil, NewExitError(ExitUsage, err)
		}
	}

	rc := &runContext{}
	rc.color = env.Color(isTerminalWriter(os.Stdout))
	if changed(cmd, "color") {
		rc.color = true
	}
	// --no-color is checked last so it always wins when both are given,
	// matching the upstream CLI's own silent priority (no hard usage
	// error for the combination).
	if changed(cmd, "no-color") {
		rc.color = false
	}
	rc.include = runplan.ResolveBool(inv, "include", "INCLUDE", inv.Include, env)
	rc.jsonOutput = runplan.ResolveBool(inv, "json", "JSON", inv.JSON, env)
	noOutput := runplan.ResolveBool(inv, "no-output", "NO_OUTPUT", inv.NoOutput, env)
	var err error
	rc.errorFormat, err = resolveErrorFormat(cmd, inv, env)
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
	rc.pretty = resolvePretty(cmd, inv, env, isTerminalWriter(stdout))
	rc.output = inv.Output
	rc.glob = inv.Glob

	plan, err := runplan.New(inv, env, currentBuildInfo().Version)
	if err != nil {
		return nil, planError(err)
	}
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

func resolveErrorFormat(cmd *cobra.Command, inv *runplan.Invocation, env config.Env) (string, error) {
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
	return "short", nil
}

func resolvePretty(cmd *cobra.Command, inv *runplan.Invocation, env config.Env, stdoutTTY bool) bool {
	if changed(cmd, "pretty") && inv.Pretty {
		return true
	}
	if changed(cmd, "no-pretty") && inv.NoPretty {
		return false
	}
	if v, ok := env.Bool("PRETTY"); ok && v {
		return true
	}
	if v, ok := env.Bool("NO_PRETTY"); ok && v {
		return false
	}
	return stdoutTTY
}

// isTerminalWriter reports whether w is a character device (a terminal),
// the stdlib-only approximation of isatty used to default --color/--pretty.
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
