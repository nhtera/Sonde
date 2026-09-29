// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package runplan turns a run's invocation (the flags of `sonde run`, as
// given) and the environment into what the engine runs: the runner's
// Options and the jobs, with the user config file and the HURL_*/SONDE_*
// environment variables applied in the documented precedence (config
// file < environment < flag). The CLI and the desktop app both build
// their runs with it, so a run means the same thing in both.
package runplan

import "fmt"

// Invocation is every flag of `sonde run` as given, and the positional
// FILE arguments. A flag counts as given when Set names it (its long
// name, e.g. "max-time"); a flag not given falls through to the
// environment and the config file, like an absent flag on the command
// line. List flags (Variables, Header, ...) apply as they are.
type Invocation struct {
	// Cmd is "test" for `sonde test` (test mode forced on), else "run".
	Cmd string
	// Files are the FILE arguments ("-" is standard input); Glob the
	// --glob patterns.
	Files []string
	Glob  []string
	// Set names the flags given explicitly.
	Set map[string]bool

	// Variables and secrets.
	Variables      []string
	VariablesFiles []string
	Secrets        []string
	SecretsFiles   []string

	// Transport.
	CACert          string
	Cert            string
	Key             string
	Compressed      bool
	ConnectTimeout  string
	ConnectTo       []string
	Insecure        bool
	IPv4            bool
	IPv6            bool
	HTTP10          bool
	HTTP11          bool
	HTTP2           bool
	HTTP3           bool
	LimitRate       string
	Location        bool
	LocationTrusted bool
	MaxFilesize     string
	MaxRedirs       string
	MaxTime         string
	Netrc           bool
	NetrcFile       string
	NetrcOptional   bool
	NoProxy         string
	PathAsIs        bool
	PinnedPubKey    string
	Proxy           string
	Resolve         []string
	UnixSocket      string
	User            string
	UserAgent       string
	Header          []string
	Digest          bool
	Negotiate       bool
	NTLM            bool
	SSLNoRevoke     bool
	AWSSigV4        string

	// Cookies.
	Cookie        string
	CookieJar     string
	NoCookieStore bool

	// Run control.
	ContinueOnError bool
	Delay           string
	FromEntry       int
	ToEntry         int
	NoAssert        bool
	Repeat          string
	Retry           string
	RetryInterval   string
	FileRoot        string
	Jobs            int
	Parallel        bool
	ProgressBar     bool
	Test            bool
	Env             string
	Config          string
	Data            string
	DataSecrets     []string
	OpenAPI         OpenAPI

	// Output.
	Include     bool
	JSON        bool
	NoOutput    bool
	NoPretty    bool
	Pretty      bool
	Output      string
	ErrorFormat string
	Curl        string
	ReportHTML  string
	ReportJSON  string
	ReportJUnit string
	ReportTAP   string

	// Logging.
	Verbose     bool
	VeryVerbose bool
	Verbosity   string
}

// OpenAPI are the --openapi* flags.
type OpenAPI struct {
	Spec        string
	Server      string
	Strict      bool
	AllowRemote bool
}

// Changed reports whether flag was given explicitly.
func (inv *Invocation) Changed(flag string) bool { return inv.Set[flag] }

// UnsupportedError reports an option sonde does not implement: a runtime
// error, not a usage error.
type UnsupportedError struct{ Name string }

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("option %q is not supported by sonde yet", e.Name)
}
