// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import "github.com/spf13/cobra"

// runOptions holds every upstream-compatible flag of `sonde [options]
// FILE...` (and its `sonde run` alias) as cobra bound them. Values are
// resolved against environment variables and the config file in
// build_options.go; a flag not explicitly given on the command line
// (cmd.Flags().Changed) falls through to those lower-precedence sources.
type runOptions struct {
	// Variables and secrets.
	variables      []string
	variablesFiles []string
	secrets        []string
	secretsFiles   []string

	// Transport.
	cacert          string
	cert            string
	key             string
	compressed      bool
	connectTimeout  string
	connectTo       []string
	insecure        bool
	ipv4            bool
	ipv6            bool
	http10          bool
	http11          bool
	http2           bool
	http3           bool
	limitRate       string
	location        bool
	locationTrusted bool
	maxFilesize     string
	maxRedirs       string
	maxTime         string
	netrc           bool
	netrcFile       string
	netrcOptional   bool
	noProxy         string
	pathAsIs        bool
	pinnedPubKey    string
	proxy           string
	resolve         []string
	unixSocket      string
	user            string
	userAgent       string
	header          []string
	digest          bool
	negotiate       bool
	ntlm            bool
	sslNoRevoke     bool
	awsSigv4        string

	// Cookies.
	cookie        string
	cookieJar     string
	noCookieStore bool

	// Run control.
	continueOnError bool
	delay           string
	fromEntry       int
	toEntry         int
	noAssert        bool
	repeat          string
	retry           string
	retryInterval   string
	fileRoot        string
	glob            []string
	jobs            int
	parallel        bool
	progressBar     bool
	test            bool

	// Output.
	include     bool
	jsonOutput  bool
	noOutput    bool
	noPretty    bool
	pretty      bool
	output      string
	errorFormat string
	curl        string
	reportHTML  string
	reportJSON  string
	reportJUnit string
	reportTAP   string

	// Logging.
	verbose     bool
	veryVerbose bool
	verbosity   string

	version bool
}

// addRunFlags registers every upstream-compatible flag on cmd's local flag
// set, so it can be attached both to the root command (the `sonde
// FILE...` entry point) and to the explicit `sonde run` alias.
func addRunFlags(cmd *cobra.Command, o *runOptions) {
	f := cmd.Flags()

	f.StringArrayVar(&o.variables, "variable", nil, "defines a variable")
	f.StringArrayVar(&o.variablesFiles, "variables-file", nil, "defines variables from a properties file")
	f.StringArrayVar(&o.secrets, "secret", nil, "defines a variable whose value is treated as a secret")
	f.StringArrayVar(&o.secretsFiles, "secrets-file", nil, "defines secrets from a file")

	f.StringVar(&o.cacert, "cacert", "", "CA certificate bundle used to verify the server (PEM)")
	f.StringVarP(&o.cert, "cert", "E", "", "client certificate file, optionally with :PASSWORD")
	f.StringVar(&o.key, "key", "", "private key file matching --cert")
	f.BoolVar(&o.compressed, "compressed", false, "requests a compressed response and decodes it")
	f.StringVar(&o.connectTimeout, "connect-timeout", "", "maximum time allowed to establish the connection")
	f.StringArrayVar(&o.connectTo, "connect-to", nil, "redirects connections for HOST1:PORT1 to HOST2:PORT2")
	f.BoolVarP(&o.insecure, "insecure", "k", false, "skips TLS certificate verification")
	f.BoolVarP(&o.ipv4, "ipv4", "4", false, "resolves hostnames to IPv4 addresses only")
	f.BoolVarP(&o.ipv6, "ipv6", "6", false, "resolves hostnames to IPv6 addresses only")
	f.BoolVarP(&o.http10, "http1.0", "0", false, "forces HTTP/1.0 (not supported by sonde)")
	f.BoolVar(&o.http11, "http1.1", false, "forces HTTP/1.1")
	f.BoolVar(&o.http2, "http2", false, "forces HTTP/2")
	f.BoolVar(&o.http3, "http3", false, "forces HTTP/3")
	f.StringVar(&o.limitRate, "limit-rate", "", "caps the transfer rate in bytes per second")
	f.BoolVarP(&o.location, "location", "L", false, "follows HTTP redirects")
	f.BoolVar(&o.locationTrusted, "location-trusted", false, "follows redirects and forwards credentials to every host")
	f.StringVar(&o.maxFilesize, "max-filesize", "", "caps the size of a downloaded file")
	f.StringVar(&o.maxRedirs, "max-redirs", "", "maximum number of redirects to follow, -1 for unlimited")
	f.StringVarP(&o.maxTime, "max-time", "m", "", "maximum time allowed for the whole transfer")
	f.BoolVarP(&o.netrc, "netrc", "n", false, "reads credentials from ~/.netrc, failing if absent")
	f.StringVar(&o.netrcFile, "netrc-file", "", "reads credentials from the given netrc-format file")
	f.BoolVar(&o.netrcOptional, "netrc-optional", false, "reads credentials from ~/.netrc if present, else the URL")
	f.StringVar(&o.noProxy, "no-proxy", "", "lists hosts that bypass the proxy")
	f.BoolVar(&o.pathAsIs, "path-as-is", false, "sends the URL path without normalizing /../ or /./")
	f.StringVar(&o.pinnedPubKey, "pinnedpubkey", "", "verifies the server's public key against pinned hashes")
	f.StringVarP(&o.proxy, "proxy", "x", "", "routes the request through the given proxy")
	f.StringArrayVar(&o.resolve, "resolve", nil, "provides a custom address for a HOST:PORT pair")
	f.StringVar(&o.unixSocket, "unix-socket", "", "connects through a Unix domain socket instead of the network")
	f.StringVarP(&o.user, "user", "u", "", "adds Basic authentication with USER:PASSWORD")
	f.StringVarP(&o.userAgent, "user-agent", "A", "", "sets the User-Agent header sent to the server")
	f.StringArrayVarP(&o.header, "header", "H", nil, "adds a custom header to every request")
	f.BoolVar(&o.digest, "digest", false, "uses HTTP Digest authentication (not supported by sonde)")
	f.BoolVar(&o.negotiate, "negotiate", false, "uses SPNEGO authentication (not supported by sonde)")
	f.BoolVar(&o.ntlm, "ntlm", false, "uses NTLM authentication (not supported by sonde)")
	f.BoolVar(&o.sslNoRevoke, "ssl-no-revoke", false, "disables certificate revocation checks (not supported by sonde)")
	f.StringVar(&o.awsSigv4, "aws-sigv4", "", "signs the request with AWS Signature Version 4 (not supported by sonde)")

	f.StringVarP(&o.cookie, "cookie", "b", "", "reads cookies from a Netscape-format FILE")
	f.StringVarP(&o.cookieJar, "cookie-jar", "c", "", "writes cookies to FILE after the run")
	f.BoolVar(&o.noCookieStore, "no-cookie-store", false, "disables the cookie store between requests")

	f.BoolVar(&o.continueOnError, "continue-on-error", false, "keeps running remaining files after a failure")
	f.StringVar(&o.delay, "delay", "", "sleep before each request")
	f.IntVar(&o.fromEntry, "from-entry", 0, "starts execution at the given entry number")
	f.IntVar(&o.toEntry, "to-entry", 0, "stops execution at the given entry number")
	f.BoolVar(&o.noAssert, "no-assert", false, "ignores asserts defined in the file")
	f.StringVar(&o.repeat, "repeat", "", "repeats the input file sequence N times, -1 for infinite")
	f.StringVar(&o.retry, "retry", "", "maximum retries on entry error, -1 for unlimited")
	f.StringVar(&o.retryInterval, "retry-interval", "", "delay between retries")
	f.StringVar(&o.fileRoot, "file-root", "", "sets the root directory used to resolve file paths")
	f.StringArrayVar(&o.glob, "glob", nil, "adds input files matching the given glob pattern")
	f.IntVar(&o.jobs, "jobs", 0, "maximum number of parallel jobs, 1 disables parallelism")
	f.BoolVar(&o.parallel, "parallel", false, "runs files in parallel (default in test mode)")
	f.BoolVar(&o.progressBar, "progress-bar", false, "shows a progress bar in test mode")
	f.BoolVar(&o.test, "test", false, "activates test mode (parallel execution, test-style output)")

	f.BoolVarP(&o.include, "include", "i", false, "includes the response headers in the output")
	f.BoolVar(&o.jsonOutput, "json", false, "outputs each file's result as JSON")
	f.BoolVar(&o.noOutput, "no-output", false, "suppresses the default last-response-body output")
	f.BoolVar(&o.noPretty, "no-pretty", false, "disables pretty-printing of response output")
	f.BoolVar(&o.pretty, "pretty", false, "pretty-prints JSON response output")
	f.StringVarP(&o.output, "output", "o", "", "writes to FILE instead of stdout")
	f.StringVar(&o.errorFormat, "error-format", "", "controls how error messages are rendered (short or long)")
	f.StringVar(&o.curl, "curl", "", "exports each request as a list of curl commands (not supported by sonde yet)")
	f.StringVar(&o.reportHTML, "report-html", "", "writes an HTML report to DIR (not supported by sonde yet)")
	f.StringVar(&o.reportJSON, "report-json", "", "writes a JSON report to DIR (not supported by sonde yet)")
	f.StringVar(&o.reportJUnit, "report-junit", "", "writes a JUnit XML report to FILE (not supported by sonde yet)")
	f.StringVar(&o.reportTAP, "report-tap", "", "writes a TAP report to FILE (not supported by sonde yet)")

	f.BoolVarP(&o.verbose, "verbose", "v", false, "turns on verbose output (alias of --verbosity verbose)")
	f.BoolVar(&o.veryVerbose, "very-verbose", false, "turns on very verbose output, including HTTP and libcurl-style logs")
	f.StringVar(&o.verbosity, "verbosity", "", "sets the verbosity level for debug logging")

	f.BoolVarP(&o.version, "version", "V", false, "prints version, commit, build date and Go version")
}
