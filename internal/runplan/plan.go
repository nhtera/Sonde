// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
	// Config is the user config file's options (zero when there is none),
	// for the CLI's output settings; the run's own are applied above.
	Config config.FileOptions
	// Provenance lists the settings that came from the environment or
	// the config file rather than a flag.
	Provenance []Source
	// Warnings are a config file left at the old location (New) and
	// sonde.yaml discovery warnings (Resolve); they only name paths.
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
	p := &Plan{inv: inv, env: env, Config: fileCfg}
	if old := env.LegacyFilePath(); fileCfg.Path == "" && old != "" {
		if _, err := os.Stat(old); err == nil {
			p.Warnings = append(p.Warnings, fmt.Sprintf("config file %s is not read: move it to %s", old, filepath.Join(env["HOME"], ".config", "hurl", "config")))
		}
	}
	p.Test = inv.Cmd == "test" || ResolveBoolOr(inv, "test", "TEST", inv.Test, env, fileCfg.Test)
	// --test implies --parallel, matching the upstream CLI; --jobs (or
	// its env var) picks the worker count, defaulting to the number of
	// CPUs. Sequential (1) otherwise, regardless of --jobs. Parallel is
	// also the switch between the sequential and parallel runners,
	// independent of --jobs, which only picks the parallel runner's worker
	// count: even `--test --jobs 1` uses the parallel runner with one
	// worker, so this
	// is what callers check for parallel-runner-only behavior (buffered
	// per-job logs, the progress bar), not Workers > 1.
	p.Parallel = p.Test || ResolveBool(inv, "parallel", "PARALLEL", inv.Parallel, env)
	p.Workers = resolveJobs(inv, env, p.Parallel, fileCfg.Jobs)

	variables, err := config.BuildVariables(fileCfg, env, inv.VariablesFiles, inv.Variables)
	if err != nil {
		return nil, err
	}
	secrets, err := config.BuildSecrets(fileCfg, env, inv.SecretsFiles, inv.Secrets)
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
	delay, err := resolveDuration(inv, "delay", "DELAY", inv.Delay, env, config.Millisecond, deref(fileCfg.Delay))
	if err != nil {
		return nil, err
	}
	retry, err := resolveCount(inv, "retry", "RETRY", inv.Retry, env, deref(fileCfg.Retry), "NUM")
	if err != nil {
		return nil, err
	}
	retryInterval, err := resolveDuration(inv, "retry-interval", "RETRY_INTERVAL", inv.RetryInterval, env, config.Millisecond, deref(fileCfg.RetryInterval))
	if err != nil {
		return nil, err
	}
	noAssert := ResolveBoolOr(inv, "no-assert", "NO_ASSERT", inv.NoAssert, env, fileCfg.NoAssert)
	continueOnError := ResolveBoolOr(inv, "continue-on-error", "CONTINUE_ON_ERROR", inv.ContinueOnError, env, fileCfg.ContinueOnError)
	failWithBody := ResolveBoolOr(inv, "fail-with-body", "FAIL_WITH_BODY", inv.FailWithBody, env, fileCfg.FailWithBody)
	noCoercion := ResolveBoolOr(inv, "no-jsonpath-coercion", "NO_JSONPATH_COERCION", inv.NoJSONPathCoercion, env, fileCfg.NoJSONPathCoercion)
	noCookieStore := ResolveBoolOr(inv, "no-cookie-store", "NO_COOKIE_STORE", inv.NoCookieStore, env, fileCfg.NoCookieStore)

	p.Repeat, err = resolveCount(inv, "repeat", "REPEAT", inv.Repeat, env, 1, "NUM")
	if err != nil {
		return nil, err
	}

	// --ssl-no-revoke is accepted as is: Go's certificate verifier checks
	// no revocation on any platform, so there is nothing to turn off.

	p.Options = engine.Options{
		Variables:          engineVars,
		Secrets:            secrets,
		FileRoot:           inv.FileRoot,
		HTTP:               http,
		CookieFile:         inv.Cookie,
		NoCookieStore:      noCookieStore,
		Retry:              retry,
		RetryInterval:      retryInterval,
		Delay:              delay,
		FromEntry:          inv.FromEntry,
		ToEntry:            inv.ToEntry,
		NoAssert:           noAssert,
		ContinueOnError:    continueOnError,
		FailWithBody:       failWithBody,
		NoJSONPathCoercion: noCoercion,
		Verbosity:          verbosity,
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
// run is not parallel, else --jobs (or HURL_JOBS/SONDE_JOBS, or the config
// file's) when it is a positive number, else the number of CPUs — matching
// the upstream CLI's own "--jobs default = available CPUs" rule. --jobs 1
// (explicit) forces sequential even when parallel/test mode was otherwise
// requested.
func resolveJobs(inv *Invocation, env config.Env, parallel bool, fileJobs *int) int {
	if !parallel {
		return 1
	}
	if inv.Changed("jobs") && inv.Jobs > 0 {
		return inv.Jobs
	}
	if n, ok, err := env.Int("JOBS"); ok && err == nil && n > 0 {
		return int(n)
	}
	if fileJobs != nil {
		return *fileJobs
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
	if fileCfg.Verbosity != "" {
		return parseVerbosityLevel(fileCfg.Verbosity)
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
	h.Compressed = ResolveBoolOr(inv, "compressed", "COMPRESSED", inv.Compressed, env, fileCfg.Compressed)
	h.Insecure = ResolveBoolOr(inv, "insecure", "INSECURE", inv.Insecure, env, fileCfg.Insecure)
	h.PathAsIs = inv.PathAsIs
	h.PinnedPublicKey = inv.PinnedPubKey
	h.Proxy = fileString(inv, "proxy", inv.Proxy, fileCfg.Proxy)
	h.NoProxy = fileString(inv, "no-proxy", inv.NoProxy, fileCfg.NoProxy)
	h.UnixSocket = inv.UnixSocket
	h.User = resolveString(inv, "user", "USER", inv.User, env, deref(fileCfg.User))
	h.UserAgent = resolveString(inv, "user-agent", "USER_AGENT", inv.UserAgent, env, deref(fileCfg.UserAgent))
	h.ConnectTo = inv.ConnectTo
	h.Resolve = inv.Resolve
	h.Netrc = inv.Netrc
	h.NetrcFile = inv.NetrcFile
	h.NetrcOptional = inv.NetrcOptional
	h.AWSSigV4 = inv.AWSSigV4
	h.Digest = inv.Digest
	h.NTLM = inv.NTLM
	h.Negotiate = inv.Negotiate

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
	var err error
	if h.NoHeaders, err = resolveNoHeaders(inv, env, fileCfg); err != nil {
		return h, err
	}
	if h.ProxyHeaders, err = resolveProxyHeaders(inv, env, fileCfg); err != nil {
		return h, err
	}

	location, locationTrusted, err := resolveFollowLocation(inv, env, fileCfg)
	if err != nil {
		return h, err
	}
	h.FollowLocation = location
	h.LocationTrusted = locationTrusted

	h.ConnectTimeout, err = resolveDuration(inv, "connect-timeout", "CONNECT_TIMEOUT", inv.ConnectTimeout, env, config.Second, deref(fileCfg.ConnectTimeout))
	if err != nil {
		return h, err
	}
	h.Timeout, err = resolveDuration(inv, "max-time", "MAX_TIME", inv.MaxTime, env, config.Second, deref(fileCfg.MaxTime))
	if err != nil {
		return h, err
	}
	h.MaxRedirects, err = resolveCount(inv, "max-redirs", "MAX_REDIRS", inv.MaxRedirs, env, deref(fileCfg.MaxRedirs), "NUM")
	if err != nil {
		return h, err
	}
	h.MaxFilesize, err = resolveInt64(inv, "max-filesize", "MAX_FILESIZE", inv.MaxFilesize, env, deref(fileCfg.MaxFilesize), "BYTES")
	if err != nil {
		return h, err
	}
	h.LimitRate, err = resolveInt64(inv, "limit-rate", "LIMIT_RATE", inv.LimitRate, env, deref(fileCfg.LimitRate), "SPEED")
	if err != nil {
		return h, err
	}

	h.IPResolve = resolveIPResolve(inv, env, fileCfg.IPv6)
	h.HTTPVersion, err = resolveHTTPVersion(inv, env, fileCfg.HTTPVersion)
	if err != nil {
		return h, err
	}
	return h, nil
}

// resolveNoHeaders adds up the header names to remove from the config
// file, the environment and --no-header; each is trimmed and must not be
// empty.
func resolveNoHeaders(inv *Invocation, env config.Env, fileCfg config.FileOptions) ([]string, error) {
	names := slices.Clone(fileCfg.NoHeaders)
	envNames, _, err := env.NoHeaders()
	if err != nil {
		return nil, err
	}
	names = append(names, envNames...)
	for _, name := range inv.NoHeader {
		if name = strings.TrimSpace(name); name == "" {
			return nil, errors.New("Missing header name") //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
		names = append(names, name)
	}
	return names, nil
}

// resolveProxyHeaders adds up the proxy headers from the config file, the
// environment and --proxy-header.
func resolveProxyHeaders(inv *Invocation, env config.Env, fileCfg config.FileOptions) ([]string, error) {
	headers := slices.Clone(fileCfg.ProxyHeaders)
	envHeaders, _, err := env.ProxyHeaders()
	if err != nil {
		return nil, err
	}
	headers = append(headers, envHeaders...)
	headers = append(headers, inv.ProxyHeader...)
	for _, h := range headers {
		if !strings.Contains(h, ":") {
			return nil, fmt.Errorf("Invalid proxy header <%s>, missing `:`", h) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	}
	return headers, nil
}

func resolveFollowLocation(inv *Invocation, env config.Env, fileCfg config.FileOptions) (location, locationTrusted bool, err error) {
	envLocation, err := env.FollowLocation(fileCfg.Location)
	if err != nil {
		return false, false, err
	}
	location = envLocation
	if inv.Changed("location") && inv.Location {
		location = true
	}
	locationTrusted = ResolveBoolOr(inv, "location-trusted", "LOCATION_TRUSTED", inv.LocationTrusted, env, fileCfg.LocationTrusted)
	// The flag implies --location; from the environment or the config
	// file, location-trusted is already part of location above, so
	// HURL_LOCATION=false still turns following off.
	if inv.Changed("location-trusted") && inv.LocationTrusted {
		location = true
	}
	locationTrusted = locationTrusted && location
	return location, locationTrusted, nil
}

func resolveIPResolve(inv *Invocation, env config.Env, fileIPv6 bool) engine.IPResolve {
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
	if fileIPv6 {
		return engine.IPv6
	}
	return engine.IPAny
}

func resolveHTTPVersion(inv *Invocation, env config.Env, fileVersion string) (engine.HTTPVersion, error) {
	switch {
	case inv.Changed("http3") && inv.HTTP3:
		return engine.HTTP3, nil
	case inv.Changed("http2-prior-knowledge") && inv.HTTP2Prior:
		return engine.HTTP2PriorKnowledge, nil
	case inv.Changed("http2") && inv.HTTP2:
		return engine.HTTP2, nil
	case inv.Changed("http1.1") && inv.HTTP11:
		return engine.HTTP11, nil
	case inv.Changed("http1.0") && inv.HTTP10:
		return engine.HTTP10, nil
	}
	v, ok := env.HTTPVersion()
	if !ok {
		v = fileVersion
	}
	switch v {
	case "3":
		return engine.HTTP3, nil
	case "2-prior-knowledge":
		return engine.HTTP2PriorKnowledge, nil
	case "2":
		return engine.HTTP2, nil
	case "1.1":
		return engine.HTTP11, nil
	case "1.0":
		return engine.HTTP10, nil
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
	{"http1.0", "HTTP10"}, {"http1.1", "HTTP11"}, {"http2", "HTTP2"}, {"http2-prior-knowledge", "HTTP2_PRIOR_KNOWLEDGE"},
	{"http3", "HTTP3"}, {"fail-with-body", "FAIL_WITH_BODY"}, {"no-jsonpath-coercion", "NO_JSONPATH_COERCION"},
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
	for _, list := range []struct{ flag, env string }{{"header", "HEADER"}, {"no-header", "NO_HEADER"}, {"proxy-header", "PROXY_HEADER"}} {
		if _, key, ok := env.Lookup(list.env); ok {
			out = append(out, Source{Setting: "--" + list.flag, Origin: key})
		}
	}
	out = append(out, fileProvenance(inv, env, fileCfg)...)
	// A variable or secret the environment defines, unless a variables
	// file or a flag replaces it.
	flagVars, errV := config.BuildVariables(config.FileOptions{}, config.Env{}, inv.VariablesFiles, inv.Variables)
	flagSecrets, errS := config.BuildSecrets(config.FileOptions{}, config.Env{}, inv.SecretsFiles, inv.Secrets)
	if errors.Join(errV, errS) == nil {
		out = append(out, prefixed("variable", env.VariableEnvVars(), func(n string) bool { _, ok := flagVars[n]; return ok }, env, "VARIABLE_")...)
		out = append(out, prefixed("secret", env.SecretEnvVars(), func(n string) bool { _, ok := flagSecrets[n]; return ok }, env, "SECRET_")...)
	}
	return out
}

// fileOverrides are the config file options a flag or an environment
// variable other than the option's own (envSettings) replaces: the
// options that set the same setting under another name.
var fileOverrides = map[string]struct{ flags, envs []string }{
	"color":        {[]string{"color", "no-color"}, []string{"COLOR", "NO_COLOR"}},
	"no-color":     {[]string{"color", "no-color"}, []string{"COLOR", "NO_COLOR"}},
	"pretty":       {[]string{"pretty", "no-pretty"}, []string{"PRETTY", "NO_PRETTY"}},
	"no-pretty":    {[]string{"pretty", "no-pretty"}, []string{"PRETTY", "NO_PRETTY"}},
	"verbose":      {verbosityFlags, verbosityEnvs},
	"very-verbose": {verbosityFlags, verbosityEnvs},
	"verbosity":    {verbosityFlags, verbosityEnvs},
	"http1.0":      {httpVersionFlags, httpVersionEnvs},
	"http1.1":      {httpVersionFlags, httpVersionEnvs},
	"http2":        {httpVersionFlags, httpVersionEnvs},
	"http3":        {httpVersionFlags, httpVersionEnvs},
	"ipv6":         {[]string{"ipv4", "ipv6"}, []string{"IPV4", "IPV6"}},
	"error-format": {[]string{"error-format"}, []string{"ERROR_FORMAT"}},
	"no-output":    {[]string{"no-output"}, []string{"NO_OUTPUT"}},
}

var (
	verbosityFlags   = []string{"verbose", "very-verbose", "verbosity"}
	verbosityEnvs    = []string{"VERBOSE", "VERY_VERBOSE", "VERBOSITY"}
	httpVersionFlags = []string{"http1.0", "http1.1", "http2", "http2-prior-knowledge", "http3"}
	httpVersionEnvs  = []string{"HTTP10", "HTTP11", "HTTP2", "HTTP2_PRIOR_KNOWLEDGE", "HTTP3"}
)

// fileProvenance lists the settings the config file sets that no flag or
// environment variable replaces, in file order. Lists (--header, ...) add
// to the other sources, so they are always listed; each variable and
// secret is listed by name, unless another source replaces the variable.
// Values are never listed.
func fileProvenance(inv *Invocation, env config.Env, fileCfg config.FileOptions) []Source {
	var out []Source
	envVars := env.VariableEnvVars()
	flagVars, _ := config.BuildVariables(config.FileOptions{}, config.Env{}, inv.VariablesFiles, inv.Variables)
	for _, key := range fileCfg.Keys {
		switch key {
		case "variable":
			seen := map[string]bool{}
			for _, a := range fileCfg.Variables {
				_, inEnv := envVars[a.Name]
				_, inFlags := flagVars[a.Name]
				if !seen[a.Name] && !inEnv && !inFlags {
					out = append(out, Source{Setting: "variable " + a.Name, Origin: fileCfg.Path})
				}
				seen[a.Name] = true
			}
			continue
		case "secret":
			for _, name := range slices.Sorted(maps.Keys(fileCfg.Secrets)) {
				out = append(out, Source{Setting: "secret " + name, Origin: fileCfg.Path})
			}
			continue
		}
		if fileOptionReplaced(inv, env, key) {
			continue
		}
		out = append(out, Source{Setting: "--" + key, Origin: fileCfg.Path})
	}
	return out
}

// fileOptionReplaced reports whether a flag or an environment variable
// sets what config file option key sets.
func fileOptionReplaced(inv *Invocation, env config.Env, key string) bool {
	flags, envs := []string{key}, []string(nil)
	if o, ok := fileOverrides[key]; ok {
		flags, envs = o.flags, o.envs
	} else {
		for _, s := range envSettings {
			if s.flag == key {
				envs = []string{s.env}
			}
		}
	}
	switch key {
	case "header", "no-header", "proxy-header":
		return false
	case "color", "no-color":
		if _, ok := env["NO_COLOR"]; ok {
			return true
		}
	}
	for _, f := range flags {
		if inv.Changed(f) {
			return true
		}
	}
	return envReplaces(env, key, envs)
}

// envReplaces reports whether one of envs replaces config file option key
// the way the run resolves it: a flag only by a value that parses, a
// group (verbosity, HTTP version, IP family) by the value its accessor
// resolves, anything else by being set.
func envReplaces(env config.Env, key string, envs []string) bool {
	switch {
	case slices.Equal(envs, verbosityEnvs):
		_, ok, err := env.Verbosity()
		return ok && err == nil
	case slices.Equal(envs, httpVersionEnvs):
		_, ok := env.HTTPVersion()
		return ok
	case key == "ipv6":
		_, ok := env.IPResolve()
		return ok
	case key == "jobs":
		n, ok, err := env.Int("JOBS")
		return ok && err == nil && n > 0
	case key == "no-output":
		v, ok := env.Bool("NO_OUTPUT")
		return ok && v
	}
	_, isFlag := fileFlags[key]
	for _, e := range envs {
		if isFlag {
			if _, ok := env.Bool(e); ok {
				return true
			}
		} else if _, _, ok := env.Lookup(e); ok {
			return true
		}
	}
	return false
}

// fileFlags are the config file options that take no value: an
// environment variable replaces one only with a boolean it reads.
var fileFlags = map[string]bool{
	"color": true, "compressed": true, "continue-on-error": true, "insecure": true,
	"location": true, "location-trusted": true, "no-assert": true, "no-color": true,
	"no-cookie-store": true, "no-pretty": true, "pretty": true, "test": true,
	"fail-with-body": true, "no-jsonpath-coercion": true,
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
