// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/datarow"
)

// Plan is a run built from an Invocation: the runner's Options and the
// run control settings (New), then each input file's sonde.yaml values
// and contract (Resolve), from which Jobs builds the jobs.
//
// Layers, lowest first: a job's sonde.yaml values (Job.Variables and
// Job.Secrets), then Options.Variables (--variable, the environment and
// variables files), then a data row. Options.Secrets are applied last.
type Plan struct {
	// Options are the runner's options. Stdout and BufferedLogs are left
	// to the caller; Validator is set by Resolve.
	Options engine.Options
	// Test is test mode (--test, or `sonde test`); Parallel whether the
	// parallel runner is used (test mode or --parallel); Workers the
	// number of files run at a time (--jobs); Repeat the passes over the
	// files (-1: forever).
	Test     bool
	Parallel bool
	Workers  int
	Repeat   int
	// Data is the --data file, checked in full (nil: none).
	Data *datarow.Run
	// Provenance lists the settings that came from the environment or
	// the config file rather than a flag.
	Provenance []Source
	// Warnings are sonde.yaml discovery warnings (Resolve); they only
	// name paths.
	Warnings []string

	inv    *Invocation
	env    config.Env
	files  []Input
	extras map[string]jobExtras
}

// Source is where a setting of a run came from, when not a flag.
type Source struct {
	// Setting is the flag ("--max-time"), or "variable NAME" / "secret
	// NAME".
	Setting string
	// Origin is the environment variable ("HURL_MAX_TIME") or the config
	// file's path.
	Origin string
}

// Input is a file to run: a path, or standard input.
type Input struct {
	Name  string
	Stdin bool
}

// New builds a run's options from inv and env, in the documented
// precedence order: config file < environment variables (HURL_*/SONDE_*)
// < flag. version names the default User-Agent (sonde/<version>). An
// *UnsupportedError reports an option sonde does not implement; every
// other error is a usage error.
func New(inv *Invocation, env config.Env, version string) (*Plan, error) {
	fileCfg, err := loadCLIConfigFile(env)
	if err != nil {
		return nil, err
	}
	p := &Plan{inv: inv, env: env}
	p.Test = inv.Cmd == "test" || ResolveBool(inv, "test", "TEST", inv.Test, env)
	// --test implies --parallel, matching the upstream CLI; --jobs (or
	// its env var) picks the worker count, defaulting to the number of
	// CPUs. Sequential (1) otherwise, regardless of --jobs. Parallel is
	// also the reference CLI's own switch between its sequential and
	// parallel runners (run_par vs. run_seq in main.rs) — independent of
	// --jobs, which only picks the parallel runner's worker count: even
	// `--test --jobs 1` uses the parallel runner with one worker, so this
	// is what callers check for parallel-runner-only behavior (buffered
	// per-job logs, the progress bar), not Workers > 1.
	p.Parallel = p.Test || ResolveBool(inv, "parallel", "PARALLEL", inv.Parallel, env)
	p.Workers = resolveJobs(inv, env, p.Parallel)

	variables, err := config.BuildVariables(env, inv.VariablesFiles, inv.Variables)
	if err != nil {
		return nil, err
	}
	secrets, err := config.BuildSecrets(env, inv.SecretsFiles, inv.Secrets)
	if err != nil {
		return nil, err
	}
	if err := config.CheckNoClash(variables, secrets); err != nil {
		return nil, err
	}
	if p.Data, err = datarow.Open(inv.Data, inv.DataSecrets, variableNames(inv.Variables), secrets); err != nil {
		return nil, err
	}
	engineVars := make(map[string]any, len(variables))
	for name, v := range variables {
		engineVars[name] = v
	}

	http, err := buildHTTPOptions(inv, env, fileCfg)
	if err != nil {
		return nil, err
	}
	verbosity, err := resolveVerbosity(inv, env, fileCfg)
	if err != nil {
		return nil, err
	}
	delay, err := resolveDuration(inv, "delay", "DELAY", inv.Delay, env, config.Millisecond, 0)
	if err != nil {
		return nil, err
	}
	retry, err := resolveCount(inv, "retry", "RETRY", inv.Retry, env, 0, "NUM")
	if err != nil {
		return nil, err
	}
	retryInterval, err := resolveDuration(inv, "retry-interval", "RETRY_INTERVAL", inv.RetryInterval, env, config.Millisecond, 0)
	if err != nil {
		return nil, err
	}
	noAssert := ResolveBool(inv, "no-assert", "NO_ASSERT", inv.NoAssert, env)
	continueOnError := ResolveBool(inv, "continue-on-error", "CONTINUE_ON_ERROR", inv.ContinueOnError, env)
	noCookieStore := ResolveBool(inv, "no-cookie-store", "NO_COOKIE_STORE", inv.NoCookieStore, env)

	p.Repeat, err = resolveCount(inv, "repeat", "REPEAT", inv.Repeat, env, 1, "NUM")
	if err != nil {
		return nil, err
	}

	for _, unsupported := range []struct {
		name    string
		enabled bool
	}{
		{"aws-sigv4", inv.AWSSigV4 != ""},
		{"digest", inv.Digest},
		{"http1.0", inv.HTTP10},
		{"negotiate", inv.Negotiate},
		{"ntlm", inv.NTLM},
		{"ssl-no-revoke", inv.SSLNoRevoke},
	} {
		if unsupported.enabled {
			return nil, &UnsupportedError{Name: unsupported.name}
		}
	}

	p.Options = engine.Options{
		Variables:       engineVars,
		Secrets:         secrets,
		FileRoot:        inv.FileRoot,
		HTTP:            http,
		CookieFile:      inv.Cookie,
		NoCookieStore:   noCookieStore,
		Retry:           retry,
		RetryInterval:   retryInterval,
		Delay:           delay,
		FromEntry:       inv.FromEntry,
		ToEntry:         inv.ToEntry,
		NoAssert:        noAssert,
		ContinueOnError: continueOnError,
		Verbosity:       verbosity,
		// The default User-Agent; SONDE_DEFAULT_USER_AGENT replaces it
		// (the conformance shim presents the reference tool's default).
		// -A and user-agent still win.
		DefaultUserAgent: DefaultUserAgent(env, version),
	}
	p.Provenance = provenance(inv, env, fileCfg)
	return p, nil
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

// DefaultUserAgent is the User-Agent sent when neither the command line
// nor the entry sets one: SONDE_DEFAULT_USER_AGENT, else sonde/<version>.
func DefaultUserAgent(env config.Env, version string) string {
	if ua := env["SONDE_DEFAULT_USER_AGENT"]; ua != "" {
		return ua
	}
	return "sonde/" + version
}

// resolveJobs returns how many files RunAll may run at once: 1 when the
// run is not parallel, else --jobs (or HURL_JOBS/SONDE_JOBS) when it is a
// positive number, else the number of CPUs — matching the upstream CLI's
// own "--jobs default = available CPUs" rule. --jobs 1 (explicit) forces
// sequential even when parallel/test mode was otherwise requested.
func resolveJobs(inv *Invocation, env config.Env, parallel bool) int {
	if !parallel {
		return 1
	}
	if inv.Changed("jobs") && inv.Jobs > 0 {
		return inv.Jobs
	}
	if n, ok, err := env.Int("JOBS"); ok && err == nil && n > 0 {
		return int(n)
	}
	return runtime.NumCPU()
}

func resolveVerbosity(inv *Invocation, env config.Env, fileCfg config.FileOptions) (engine.Verbosity, error) {
	if inv.Changed("verbose") && inv.Verbose {
		return engine.Verbose, nil
	}
	if inv.Changed("very-verbose") && inv.VeryVerbose {
		return engine.VeryVerbose, nil
	}
	if inv.Changed("verbosity") {
		return parseVerbosityLevel(inv.Verbosity)
	}
	if v, ok, err := env.Verbosity(); ok {
		if err != nil {
			return 0, err
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
	return 0, fmt.Errorf("invalid value '%s' for verbosity [possible values: brief, verbose, debug]", s)
}

// buildHTTPOptions resolves every transport flag.
func buildHTTPOptions(inv *Invocation, env config.Env, fileCfg config.FileOptions) (engine.HTTPOptions, error) {
	h := engine.HTTPOptions{}
	h.CACert = inv.CACert
	h.ClientCert = inv.Cert
	h.ClientKey = inv.Key
	h.Compressed = ResolveBool(inv, "compressed", "COMPRESSED", inv.Compressed, env)
	h.Insecure = ResolveBool(inv, "insecure", "INSECURE", inv.Insecure, env)
	h.PathAsIs = inv.PathAsIs
	h.PinnedPublicKey = inv.PinnedPubKey
	h.Proxy = inv.Proxy
	h.NoProxy = inv.NoProxy
	h.UnixSocket = inv.UnixSocket
	h.User = resolveString(inv, "user", "USER", inv.User, env, "")
	h.UserAgent = resolveString(inv, "user-agent", "USER_AGENT", inv.UserAgent, env, "")
	if h.UserAgent == "" && fileCfg.UserAgent != nil {
		h.UserAgent = *fileCfg.UserAgent
	}
	h.ConnectTo = inv.ConnectTo
	h.Resolve = inv.Resolve
	h.Netrc = inv.Netrc
	h.NetrcFile = inv.NetrcFile
	h.NetrcOptional = inv.NetrcOptional

	if err := validateHeaders(fileCfg.Headers); err != nil {
		return h, err
	}
	h.Headers = append(h.Headers, fileCfg.Headers...)
	if envHeaders, ok, err := env.Headers(); ok {
		if err != nil {
			return h, err
		}
		h.Headers = append(h.Headers, envHeaders...)
	}
	if err := validateHeaders(inv.Header); err != nil {
		return h, err
	}
	h.Headers = append(h.Headers, inv.Header...)

	location, locationTrusted, err := resolveFollowLocation(inv, env)
	if err != nil {
		return h, err
	}
	h.FollowLocation = location
	h.LocationTrusted = locationTrusted

	h.ConnectTimeout, err = resolveDuration(inv, "connect-timeout", "CONNECT_TIMEOUT", inv.ConnectTimeout, env, config.Second, 0)
	if err != nil {
		return h, err
	}
	h.Timeout, err = resolveDuration(inv, "max-time", "MAX_TIME", inv.MaxTime, env, config.Second, 0)
	if err != nil {
		return h, err
	}
	maxRedirects, err := resolveCount(inv, "max-redirs", "MAX_REDIRS", inv.MaxRedirs, env, 0, "NUM")
	if err != nil {
		return h, err
	}
	if fileCfg.MaxRedirs != nil && !inv.Changed("max-redirs") {
		if _, _, ok := env.Lookup("MAX_REDIRS"); !ok {
			maxRedirects = *fileCfg.MaxRedirs
		}
	}
	h.MaxRedirects = maxRedirects

	maxFilesize, err := resolveInt64(inv, "max-filesize", "MAX_FILESIZE", inv.MaxFilesize, env, 0, "BYTES")
	if err != nil {
		return h, err
	}
	h.MaxFilesize = maxFilesize
	limitRate, err := resolveInt64(inv, "limit-rate", "LIMIT_RATE", inv.LimitRate, env, 0, "SPEED")
	if err != nil {
		return h, err
	}
	h.LimitRate = limitRate

	h.IPResolve = resolveIPResolve(inv, env)
	h.HTTPVersion, err = resolveHTTPVersion(inv, env)
	if err != nil {
		return h, err
	}
	return h, nil
}

func resolveFollowLocation(inv *Invocation, env config.Env) (location, locationTrusted bool, err error) {
	envLocation, err := env.FollowLocation(false)
	if err != nil {
		return false, false, err
	}
	location = envLocation
	if inv.Changed("location") && inv.Location {
		location = true
	}
	locationTrusted = ResolveBool(inv, "location-trusted", "LOCATION_TRUSTED", inv.LocationTrusted, env)
	if locationTrusted {
		location = true
	}
	return location, locationTrusted, nil
}

func resolveIPResolve(inv *Invocation, env config.Env) engine.IPResolve {
	switch {
	case inv.Changed("ipv6") && inv.IPv6:
		return engine.IPv6
	case inv.Changed("ipv4") && inv.IPv4:
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

func resolveHTTPVersion(inv *Invocation, env config.Env) (engine.HTTPVersion, error) {
	switch {
	case inv.Changed("http3") && inv.HTTP3:
		return engine.HTTP3, nil
	case inv.Changed("http2") && inv.HTTP2:
		return engine.HTTP2, nil
	case inv.Changed("http1.1") && inv.HTTP11:
		return engine.HTTP11, nil
	case inv.Changed("http1.0") && inv.HTTP10:
		return 0, &UnsupportedError{Name: "http1.0"}
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
			return 0, &UnsupportedError{Name: "http1.0"}
		}
	}
	return engine.HTTPDefault, nil
}

// validateHeaders checks that every "--header"/config-file header has a
// ':' separator, matching the upstream CLI's own message.
func validateHeaders(headers []string) error {
	for _, h := range headers {
		if !strings.Contains(h, ":") {
			return fmt.Errorf("Invalid header <%s>, missing `:`", h) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	}
	return nil
}

// variableNames returns the names of --variable assignments.
func variableNames(variables []string) []string {
	var names []string
	for _, s := range variables {
		if a, err := config.ParseAssignment(s, config.Inferred); err == nil {
			names = append(names, a.Name)
		}
	}
	return names
}

// envSettings are the flags an environment variable can set, and the
// variable's name without its HURL_/SONDE_ prefix.
var envSettings = []struct{ flag, env string }{
	{"compressed", "COMPRESSED"}, {"insecure", "INSECURE"}, {"user", "USER"}, {"user-agent", "USER_AGENT"},
	{"location", "LOCATION"}, {"location-trusted", "LOCATION_TRUSTED"},
	{"connect-timeout", "CONNECT_TIMEOUT"}, {"max-time", "MAX_TIME"}, {"max-redirs", "MAX_REDIRS"},
	{"max-filesize", "MAX_FILESIZE"}, {"limit-rate", "LIMIT_RATE"},
	{"ipv4", "IPV4"}, {"ipv6", "IPV6"},
	{"http1.0", "HTTP10"}, {"http1.1", "HTTP11"}, {"http2", "HTTP2"}, {"http3", "HTTP3"},
	{"verbosity", "VERBOSITY"}, {"delay", "DELAY"}, {"retry", "RETRY"}, {"retry-interval", "RETRY_INTERVAL"},
	{"no-assert", "NO_ASSERT"}, {"continue-on-error", "CONTINUE_ON_ERROR"}, {"no-cookie-store", "NO_COOKIE_STORE"},
	{"repeat", "REPEAT"}, {"parallel", "PARALLEL"}, {"test", "TEST"}, {"jobs", "JOBS"},
}

// provenance lists the settings of a run that the environment or the
// config file set: a flag given on the command line hides them.
func provenance(inv *Invocation, env config.Env, fileCfg config.FileOptions) []Source {
	var out []Source
	for _, s := range envSettings {
		if inv.Changed(s.flag) {
			continue
		}
		if _, key, ok := env.Lookup(s.env); ok {
			out = append(out, Source{Setting: "--" + s.flag, Origin: key})
		}
	}
	if _, key, ok := env.Lookup("HEADER"); ok {
		out = append(out, Source{Setting: "--header", Origin: key})
	}
	if path, ok := env.FilePath(); ok {
		_, _, envUA := env.Lookup("USER_AGENT")
		_, _, envRedirs := env.Lookup("MAX_REDIRS")
		_, _, envVerbosity := env.Lookup("VERBOSITY")
		switch {
		case fileCfg.UserAgent != nil && !inv.Changed("user-agent") && !envUA:
			out = append(out, Source{Setting: "--user-agent", Origin: path})
		}
		if fileCfg.MaxRedirs != nil && !inv.Changed("max-redirs") && !envRedirs {
			out = append(out, Source{Setting: "--max-redirs", Origin: path})
		}
		verboseFlag := inv.Changed("verbose") || inv.Changed("very-verbose") || inv.Changed("verbosity")
		if fileCfg.Verbose && !verboseFlag && !envVerbosity {
			out = append(out, Source{Setting: "--verbose", Origin: path})
		}
		if len(fileCfg.Headers) > 0 {
			out = append(out, Source{Setting: "--header", Origin: path})
		}
	}
	// A variable or secret the environment defines, unless a variables
	// file or a flag replaces it.
	flagVars, errV := config.BuildVariables(config.Env{}, inv.VariablesFiles, inv.Variables)
	flagSecrets, errS := config.BuildSecrets(config.Env{}, inv.SecretsFiles, inv.Secrets)
	if errors.Join(errV, errS) == nil {
		out = append(out, prefixed("variable", env.VariableEnvVars(), func(n string) bool { _, ok := flagVars[n]; return ok }, env, "VARIABLE_")...)
		out = append(out, prefixed("secret", env.SecretEnvVars(), func(n string) bool { _, ok := flagSecrets[n]; return ok }, env, "SECRET_")...)
	}
	return out
}

// prefixed lists the names of vars (from HURL_<infix>*/SONDE_<infix>*)
// that replaced does not report, sorted.
func prefixed(kind string, vars map[string]string, replaced func(string) bool, env config.Env, infix string) []Source {
	names := make([]string, 0, len(vars))
	for name := range vars {
		if !replaced(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]Source, 0, len(names))
	for _, name := range names {
		_, key, _ := env.Lookup(infix + name)
		out = append(out, Source{Setting: kind + " " + name, Origin: key})
	}
	return out
}
