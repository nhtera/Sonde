// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
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
}

// hasReport reports whether any --report-* flag was given: results are
// only retained for the report writers when at least one is (memory flat
// otherwise, per the plan).
func (rc *runContext) hasReport() bool {
	return rc.reportHTML != "" || rc.reportJSON != "" || rc.reportJUnit != "" || rc.reportTAP != ""
}

// buildRunContext resolves every flag, environment variable and config
// file setting into a runContext, in the documented precedence order:
// config file < env vars (HURL_*/SONDE_*) < command line flag.
func buildRunContext(cmd *cobra.Command, o *runOptions, env config.Env, stdout io.Writer) (*runContext, error) {
	fileCfg, err := loadCLIConfigFile(env)
	if err != nil {
		return nil, NewExitError(ExitUsage, err)
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
	rc.include = resolveBool(cmd, "include", "INCLUDE", o.include, env)
	rc.jsonOutput = resolveBool(cmd, "json", "JSON", o.jsonOutput, env)
	noOutput := resolveBool(cmd, "no-output", "NO_OUTPUT", o.noOutput, env)
	rc.errorFormat, err = resolveErrorFormat(cmd, o, env)
	if err != nil {
		return nil, err
	}
	rc.cookieJar = o.cookieJar
	rc.curlFile = o.curl
	rc.reportHTML = o.reportHTML
	rc.reportJSON = o.reportJSON
	rc.reportJUnit = o.reportJUnit
	rc.reportTAP = o.reportTAP
	rc.env = o.env
	rc.configFile = o.config
	rc.progressBar = o.progressBar
	rc.pretty = resolvePretty(cmd, o, env, isTerminalWriter(stdout))
	rc.output = o.output

	rc.test = resolveBool(cmd, "test", "TEST", o.test, env)
	if rc.test {
		noOutput = true
	}
	rc.noOutput = noOutput
	rc.glob = o.glob

	// --test implies --parallel, matching the upstream CLI; --jobs (or
	// its env var) picks the worker count, defaulting to the number of
	// CPUs. Sequential (1) otherwise, regardless of --jobs. rc.parallel is
	// also the reference CLI's own switch between its sequential and
	// parallel runners (run_par vs. run_seq in main.rs) — independent of
	// --jobs, which only picks the parallel runner's worker count: even
	// `--test --jobs 1` uses the parallel runner with one worker, so this
	// is what CLI code should check for parallel-runner-only behavior
	// (buffered per-job logs, the progress bar), not rc.jobs > 1.
	parallel := rc.test || resolveBool(cmd, "parallel", "PARALLEL", o.parallel, env)
	rc.parallel = parallel
	rc.jobs = resolveJobs(cmd, o, env, parallel)

	variables, err := config.BuildVariables(env, o.variablesFiles, o.variables)
	if err != nil {
		return nil, NewExitError(ExitUsage, err)
	}
	secrets, err := config.BuildSecrets(env, o.secretsFiles, o.secrets)
	if err != nil {
		return nil, NewExitError(ExitUsage, err)
	}
	if err := config.CheckNoClash(variables, secrets); err != nil {
		return nil, NewExitError(ExitUsage, err)
	}
	engineVars := make(map[string]any, len(variables))
	for name, v := range variables {
		engineVars[name] = v
	}

	http, err := buildHTTPOptions(cmd, o, env, fileCfg)
	if err != nil {
		return nil, err
	}

	verbosity, err := resolveVerbosity(cmd, o, env, fileCfg)
	if err != nil {
		return nil, err
	}

	delay, err := resolveDuration(cmd, "delay", "DELAY", o.delay, env, config.Millisecond, 0)
	if err != nil {
		return nil, err
	}
	retry, err := resolveCount(cmd, "retry", "RETRY", o.retry, env, 0, "NUM")
	if err != nil {
		return nil, err
	}
	retryInterval, err := resolveDuration(cmd, "retry-interval", "RETRY_INTERVAL", o.retryInterval, env, config.Millisecond, 0)
	if err != nil {
		return nil, err
	}
	noAssert := resolveBool(cmd, "no-assert", "NO_ASSERT", o.noAssert, env)
	continueOnError := resolveBool(cmd, "continue-on-error", "CONTINUE_ON_ERROR", o.continueOnError, env)
	noCookieStore := resolveBool(cmd, "no-cookie-store", "NO_COOKIE_STORE", o.noCookieStore, env)

	rc.repeat, err = resolveCount(cmd, "repeat", "REPEAT", o.repeat, env, 1, "NUM")
	if err != nil {
		return nil, err
	}

	for _, unsupported := range []struct {
		name    string
		enabled bool
	}{
		{"aws-sigv4", o.awsSigv4 != ""},
		{"digest", o.digest},
		{"http1.0", o.http10},
		{"negotiate", o.negotiate},
		{"ntlm", o.ntlm},
		{"ssl-no-revoke", o.sslNoRevoke},
	} {
		if unsupported.enabled {
			return nil, unsupportedOptionErr(unsupported.name)
		}
	}

	rc.engine = engine.Options{
		Variables:       engineVars,
		Secrets:         secrets,
		FileRoot:        o.fileRoot,
		HTTP:            http,
		CookieFile:      o.cookie,
		NoCookieStore:   noCookieStore,
		Retry:           retry,
		RetryInterval:   retryInterval,
		Delay:           delay,
		FromEntry:       o.fromEntry,
		ToEntry:         o.toEntry,
		NoAssert:        noAssert,
		ContinueOnError: continueOnError,
		Verbosity:       verbosity,
		Stdout:          stdout,
		Version:         currentBuildInfo().Version,
		// A default User-Agent (used by the conformance shim to present
		// the reference tool's default); -A and user-agent still win.
		DefaultUserAgent: env["SONDE_DEFAULT_USER_AGENT"],
	}
	return rc, nil
}

// loadCLIConfigFile reads and parses the config file named by env, if any;
// a missing file is not an error.
func loadCLIConfigFile(env config.Env) (config.FileOptions, error) {
	path, ok := env.FilePath()
	if !ok {
		return config.FileOptions{}, nil
	}
	return config.LoadConfigFile(path)
}

func resolveErrorFormat(cmd *cobra.Command, o *runOptions, env config.Env) (string, error) {
	if changed(cmd, "error-format") {
		if o.errorFormat != "short" && o.errorFormat != "long" {
			return "", NewExitError(ExitUsage, fmt.Errorf("invalid value '%s' for error-format [possible values: long, short]", o.errorFormat))
		}
		return o.errorFormat, nil
	}
	if v, ok, err := env.ErrorFormat(); ok {
		if err != nil {
			return "", NewExitError(ExitUsage, err)
		}
		return v, nil
	}
	return "short", nil
}

// resolveJobs returns how many files RunAll may run at once: 1 when the
// run is not parallel, else --jobs (or HURL_JOBS/SONDE_JOBS) when it is a
// positive number, else the number of CPUs — matching the upstream CLI's
// own "--jobs default = available CPUs" rule. --jobs 1 (explicit) forces
// sequential even when parallel/test mode was otherwise requested.
func resolveJobs(cmd *cobra.Command, o *runOptions, env config.Env, parallel bool) int {
	if !parallel {
		return 1
	}
	if changed(cmd, "jobs") && o.jobs > 0 {
		return o.jobs
	}
	if n, ok, err := env.Int("JOBS"); ok && err == nil && n > 0 {
		return int(n)
	}
	return runtime.NumCPU()
}

func resolvePretty(cmd *cobra.Command, o *runOptions, env config.Env, stdoutTTY bool) bool {
	if changed(cmd, "pretty") && o.pretty {
		return true
	}
	if changed(cmd, "no-pretty") && o.noPretty {
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

func resolveVerbosity(cmd *cobra.Command, o *runOptions, env config.Env, fileCfg config.FileOptions) (engine.Verbosity, error) {
	if changed(cmd, "verbose") && o.verbose {
		return engine.Verbose, nil
	}
	if changed(cmd, "very-verbose") && o.veryVerbose {
		return engine.VeryVerbose, nil
	}
	if changed(cmd, "verbosity") {
		return parseVerbosityLevel(o.verbosity)
	}
	if v, ok, err := env.Verbosity(); ok {
		if err != nil {
			return 0, NewExitError(ExitUsage, err)
		}
		return parseVerbosityLevel(v)
	}
	if fileCfg.Verbose {
		return engine.Verbose, nil
	}
	return engine.Quiet, nil
}

func parseVerbosityLevel(s string) (engine.Verbosity, error) {
	switch s {
	case "brief":
		return engine.Brief, nil
	case "verbose":
		return engine.Verbose, nil
	case "debug":
		return engine.VeryVerbose, nil
	}
	return 0, NewExitError(ExitUsage, fmt.Errorf("invalid value '%s' for verbosity [possible values: brief, verbose, debug]", s))
}

// buildHTTPOptions resolves every transport flag.
func buildHTTPOptions(cmd *cobra.Command, o *runOptions, env config.Env, fileCfg config.FileOptions) (engine.HTTPOptions, error) {
	h := engine.HTTPOptions{}
	h.CACert = o.cacert
	h.ClientCert = o.cert
	h.ClientKey = o.key
	h.Compressed = resolveBool(cmd, "compressed", "COMPRESSED", o.compressed, env)
	h.Insecure = resolveBool(cmd, "insecure", "INSECURE", o.insecure, env)
	h.PathAsIs = o.pathAsIs
	h.PinnedPublicKey = o.pinnedPubKey
	h.Proxy = o.proxy
	h.NoProxy = o.noProxy
	h.UnixSocket = o.unixSocket
	h.User = resolveString(cmd, "user", "USER", o.user, env, "")
	h.UserAgent = resolveString(cmd, "user-agent", "USER_AGENT", o.userAgent, env, "")
	if h.UserAgent == "" && fileCfg.UserAgent != nil {
		h.UserAgent = *fileCfg.UserAgent
	}
	h.ConnectTo = o.connectTo
	h.Resolve = o.resolve
	h.Netrc = o.netrc
	h.NetrcFile = o.netrcFile
	h.NetrcOptional = o.netrcOptional

	if err := validateHeaders(fileCfg.Headers); err != nil {
		return h, err
	}
	h.Headers = append(h.Headers, fileCfg.Headers...)
	if envHeaders, ok, err := env.Headers(); ok {
		if err != nil {
			return h, NewExitError(ExitUsage, err)
		}
		h.Headers = append(h.Headers, envHeaders...)
	}
	if err := validateHeaders(o.header); err != nil {
		return h, err
	}
	h.Headers = append(h.Headers, o.header...)

	location, locationTrusted, err := resolveFollowLocation(cmd, o, env)
	if err != nil {
		return h, err
	}
	h.FollowLocation = location
	h.LocationTrusted = locationTrusted

	h.ConnectTimeout, err = resolveDuration(cmd, "connect-timeout", "CONNECT_TIMEOUT", o.connectTimeout, env, config.Second, 0)
	if err != nil {
		return h, err
	}
	h.Timeout, err = resolveDuration(cmd, "max-time", "MAX_TIME", o.maxTime, env, config.Second, 0)
	if err != nil {
		return h, err
	}
	maxRedirects, err := resolveCount(cmd, "max-redirs", "MAX_REDIRS", o.maxRedirs, env, 0, "NUM")
	if err != nil {
		return h, err
	}
	if fileCfg.MaxRedirs != nil && !changed(cmd, "max-redirs") {
		if _, _, ok := env.Lookup("MAX_REDIRS"); !ok {
			maxRedirects = *fileCfg.MaxRedirs
		}
	}
	h.MaxRedirects = maxRedirects

	maxFilesize, err := resolveInt64(cmd, "max-filesize", "MAX_FILESIZE", o.maxFilesize, env, 0, "BYTES")
	if err != nil {
		return h, err
	}
	h.MaxFilesize = maxFilesize
	limitRate, err := resolveInt64(cmd, "limit-rate", "LIMIT_RATE", o.limitRate, env, 0, "SPEED")
	if err != nil {
		return h, err
	}
	h.LimitRate = limitRate

	h.IPResolve = resolveIPResolve(cmd, o, env)
	h.HTTPVersion, err = resolveHTTPVersion(cmd, o, env)
	if err != nil {
		return h, err
	}
	return h, nil
}

func resolveFollowLocation(cmd *cobra.Command, o *runOptions, env config.Env) (location, locationTrusted bool, err error) {
	envLocation, err := env.FollowLocation(false)
	if err != nil {
		return false, false, NewExitError(ExitUsage, err)
	}
	location = envLocation
	if changed(cmd, "location") && o.location {
		location = true
	}
	locationTrusted = resolveBool(cmd, "location-trusted", "LOCATION_TRUSTED", o.locationTrusted, env)
	if locationTrusted {
		location = true
	}
	return location, locationTrusted, nil
}

func resolveIPResolve(cmd *cobra.Command, o *runOptions, env config.Env) engine.IPResolve {
	switch {
	case changed(cmd, "ipv6") && o.ipv6:
		return engine.IPv6
	case changed(cmd, "ipv4") && o.ipv4:
		return engine.IPv4
	}
	if v, ok := env.IPResolve(); ok {
		if v == "6" {
			return engine.IPv6
		}
		return engine.IPv4
	}
	return engine.IPAny
}

func resolveHTTPVersion(cmd *cobra.Command, o *runOptions, env config.Env) (engine.HTTPVersion, error) {
	switch {
	case changed(cmd, "http3") && o.http3:
		return engine.HTTP3, nil
	case changed(cmd, "http2") && o.http2:
		return engine.HTTP2, nil
	case changed(cmd, "http1.1") && o.http11:
		return engine.HTTP11, nil
	case changed(cmd, "http1.0") && o.http10:
		return 0, unsupportedOptionErr("http1.0")
	}
	if v, ok := env.HTTPVersion(); ok {
		switch v {
		case "3":
			return engine.HTTP3, nil
		case "2":
			return engine.HTTP2, nil
		case "1.1":
			return engine.HTTP11, nil
		case "1.0":
			return 0, unsupportedOptionErr("http1.0")
		}
	}
	return engine.HTTPDefault, nil
}

// validateHeaders checks that every "--header"/config-file header has a
// ':' separator, matching the upstream CLI's own message.
func validateHeaders(headers []string) error {
	for _, h := range headers {
		if !strings.Contains(h, ":") {
			return NewExitError(ExitUsage, fmt.Errorf("Invalid header <%s>, missing `:`", h)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	}
	return nil
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
