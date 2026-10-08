// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nhtera/sonde/internal/runplan"
)

// runOptions holds every upstream-compatible flag of `sonde [options]
// FILE...` (and its `sonde run` alias) as cobra bound them, in the
// invocation runplan builds the run from. Values are resolved against
// environment variables and the config file there; a flag not explicitly
// given on the command line (cmd.Flags().Changed, recorded in inv.Set by
// invocation) falls through to those lower-precedence sources.
type runOptions struct {
	inv     runplan.Invocation
	version bool
}

// invocation completes o's invocation with what cobra knows once the
// command line is parsed: the flags given, the FILE arguments and the
// command.
func (o *runOptions) invocation(cmd *cobra.Command, args []string, forceTest bool) *runplan.Invocation {
	inv := &o.inv
	inv.Set = map[string]bool{}
	cmd.Flags().Visit(func(f *pflag.Flag) { inv.Set[f.Name] = true })
	inv.Files = args
	inv.Cmd = "run"
	if forceTest {
		inv.Cmd = "test"
	}
	return inv
}

// addRunFlags registers every upstream-compatible flag on cmd's local flag
// set, so it can be attached both to the root command (the `sonde
// FILE...` entry point) and to the explicit `sonde run` alias.
func addRunFlags(cmd *cobra.Command, o *runOptions) {
	f := cmd.Flags()

	f.StringArrayVar(&o.inv.Variables, "variable", nil, "defines a variable")
	f.StringArrayVar(&o.inv.VariablesFiles, "variables-file", nil, "defines variables from a properties file")
	f.StringArrayVar(&o.inv.Secrets, "secret", nil, "defines a variable whose value is treated as a secret")
	f.StringArrayVar(&o.inv.SecretsFiles, "secrets-file", nil, "defines secrets from a file")

	f.StringVar(&o.inv.CACert, "cacert", "", "CA certificate bundle used to verify the server (PEM)")
	f.StringVarP(&o.inv.Cert, "cert", "E", "", "client certificate file, optionally with :PASSWORD")
	f.StringVar(&o.inv.Key, "key", "", "private key file matching --cert")
	f.BoolVar(&o.inv.Compressed, "compressed", false, "requests a compressed response and decodes it")
	f.StringVar(&o.inv.ConnectTimeout, "connect-timeout", "", "maximum time allowed to establish the connection")
	f.StringArrayVar(&o.inv.ConnectTo, "connect-to", nil, "redirects connections for HOST1:PORT1 to HOST2:PORT2")
	f.BoolVarP(&o.inv.Insecure, "insecure", "k", false, "skips TLS certificate verification")
	f.BoolVarP(&o.inv.IPv4, "ipv4", "4", false, "resolves hostnames to IPv4 addresses only")
	f.BoolVarP(&o.inv.IPv6, "ipv6", "6", false, "resolves hostnames to IPv6 addresses only")
	f.BoolVarP(&o.inv.HTTP10, "http1.0", "0", false, "forces HTTP/1.0")
	f.BoolVar(&o.inv.HTTP11, "http1.1", false, "forces HTTP/1.1")
	f.BoolVar(&o.inv.HTTP2, "http2", false, "forces HTTP/2")
	f.BoolVar(&o.inv.HTTP2Prior, "http2-prior-knowledge", false, "uses HTTP/2 without an HTTP/1.1 upgrade")
	f.BoolVar(&o.inv.HTTP3, "http3", false, "uses HTTP/3 (QUIC) for https://, falling back to TCP when it cannot connect")
	f.StringVar(&o.inv.LimitRate, "limit-rate", "", "caps the transfer rate in bytes per second")
	f.BoolVarP(&o.inv.Location, "location", "L", false, "follows HTTP redirects")
	f.BoolVar(&o.inv.LocationTrusted, "location-trusted", false, "follows redirects and forwards credentials to every host")
	f.StringVar(&o.inv.MaxFilesize, "max-filesize", "", "caps the size of a downloaded file")
	f.StringVar(&o.inv.MaxRedirs, "max-redirs", "", "maximum number of redirects to follow, -1 for unlimited")
	f.StringVarP(&o.inv.MaxTime, "max-time", "m", "", "maximum time allowed for the whole transfer")
	f.BoolVarP(&o.inv.Netrc, "netrc", "n", false, "reads credentials from ~/.netrc, failing if absent")
	f.StringVar(&o.inv.NetrcFile, "netrc-file", "", "reads credentials from the given netrc-format file")
	f.BoolVar(&o.inv.NetrcOptional, "netrc-optional", false, "reads credentials from ~/.netrc if present, else the URL")
	f.StringArrayVar(&o.inv.NoHeader, "no-header", nil, "removes a header sent to the server, a default one included")
	f.StringVar(&o.inv.NoProxy, "no-proxy", "", "lists hosts that bypass the proxy")
	f.BoolVar(&o.inv.PathAsIs, "path-as-is", false, "sends the URL path without normalizing /../ or /./")
	f.StringVar(&o.inv.PinnedPubKey, "pinnedpubkey", "", "verifies the server's public key against pinned hashes")
	f.StringVarP(&o.inv.Proxy, "proxy", "x", "", "routes the request through the given proxy")
	f.StringArrayVar(&o.inv.ProxyHeader, "proxy-header", nil, "adds a header sent to the proxy only")
	f.StringArrayVar(&o.inv.Resolve, "resolve", nil, "provides a custom address for a HOST:PORT pair")
	f.StringVar(&o.inv.UnixSocket, "unix-socket", "", "connects through a Unix domain socket instead of the network")
	f.StringVarP(&o.inv.User, "user", "u", "", "adds Basic authentication with USER:PASSWORD")
	f.StringVarP(&o.inv.UserAgent, "user-agent", "A", "", "sets the User-Agent header sent to the server")
	f.StringArrayVarP(&o.inv.Header, "header", "H", nil, "adds a custom header to every request")
	f.BoolVar(&o.inv.Digest, "digest", false, "uses HTTP Digest authentication")
	f.BoolVar(&o.inv.Negotiate, "negotiate", false, "uses SPNEGO (Negotiate) authentication from the Kerberos credential cache")
	f.BoolVar(&o.inv.NTLM, "ntlm", false, "uses NTLM authentication")
	f.BoolVar(&o.inv.SSLNoRevoke, "ssl-no-revoke", false, "accepted for compatibility: sonde checks no certificate revocation")
	f.StringVar(&o.inv.AWSSigV4, "aws-sigv4", "", "signs the request with AWS Signature Version 4")

	f.StringVarP(&o.inv.Cookie, "cookie", "b", "", "reads cookies from a Netscape-format FILE")
	f.StringVarP(&o.inv.CookieJar, "cookie-jar", "c", "", "writes cookies to FILE after the run")
	f.BoolVar(&o.inv.NoCookieStore, "no-cookie-store", false, "disables the cookie store between requests")

	f.BoolVar(&o.inv.ContinueOnError, "continue-on-error", false, "keeps running remaining files after a failure")
	f.BoolVar(&o.inv.FailWithBody, "fail-with-body", false, "writes the response body of a failed entry before its errors")
	f.BoolVar(&o.inv.NoJSONPathCoercion, "no-jsonpath-coercion", false, "keeps jsonpath results a list of matches")
	f.StringVar(&o.inv.Delay, "delay", "", "sleep before each request")
	f.IntVar(&o.inv.FromEntry, "from-entry", 0, "starts execution at the given entry number")
	f.IntVar(&o.inv.ToEntry, "to-entry", 0, "stops execution at the given entry number")
	f.BoolVar(&o.inv.NoAssert, "no-assert", false, "ignores asserts defined in the file")
	f.StringVar(&o.inv.Repeat, "repeat", "", "repeats the input file sequence N times, -1 for infinite")
	f.StringVar(&o.inv.Retry, "retry", "", "maximum retries on entry error, -1 for unlimited")
	f.StringVar(&o.inv.RetryInterval, "retry-interval", "", "delay between retries")
	f.StringVar(&o.inv.FileRoot, "file-root", "", "sets the root directory used to resolve file paths")
	f.StringArrayVar(&o.inv.Glob, "glob", nil, "adds input files matching the given glob pattern")
	f.IntVar(&o.inv.Jobs, "jobs", 0, "maximum number of parallel jobs, 1 disables parallelism")
	f.BoolVar(&o.inv.Parallel, "parallel", false, "runs files in parallel (default in test mode)")
	f.BoolVar(&o.inv.ProgressBar, "progress-bar", false, "shows a progress bar in test mode")
	f.BoolVar(&o.inv.Test, "test", false, "activates test mode (parallel execution, test-style output)")
	f.StringVar(&o.inv.Env, "env", "", "selects a sonde.yaml environment by name")
	f.StringVar(&o.inv.Config, "config", "", "uses this sonde.yaml for every input file, skipping discovery")
	f.StringVar(&o.inv.Data, "data", "", "runs each file once per row of a CSV or JSON data file")
	f.StringSliceVar(&o.inv.DataSecrets, "data-secret", nil, "marks data file columns as secrets (comma-separated, repeatable)")
	addOpenAPIFlags(f, &o.inv.OpenAPI)

	f.BoolVarP(&o.inv.Include, "include", "i", false, "includes the response headers in the output")
	f.BoolVar(&o.inv.JSON, "json", false, "outputs each file's result as JSON")
	f.BoolVar(&o.inv.NoOutput, "no-output", false, "suppresses the default last-response-body output")
	f.BoolVar(&o.inv.NoPretty, "no-pretty", false, "disables pretty-printing of response output")
	f.BoolVar(&o.inv.Pretty, "pretty", false, "pretty-prints JSON response output")
	f.StringVarP(&o.inv.Output, "output", "o", "", "writes to FILE instead of stdout")
	f.StringVar(&o.inv.ErrorFormat, "error-format", "", "controls how error messages are rendered (short or long)")
	f.StringVar(&o.inv.Curl, "curl", "", "exports each request as a list of curl commands")
	f.StringVar(&o.inv.ReportHTML, "report-html", "", "writes an HTML report to DIR")
	f.StringVar(&o.inv.ReportJSON, "report-json", "", "writes a JSON report to DIR")
	f.StringVar(&o.inv.ReportJUnit, "report-junit", "", "writes a JUnit XML report to FILE")
	f.StringVar(&o.inv.ReportTAP, "report-tap", "", "writes a TAP report to FILE")

	f.BoolVarP(&o.inv.Verbose, "verbose", "v", false, "turns on verbose output (alias of --verbosity verbose)")
	f.BoolVar(&o.inv.VeryVerbose, "very-verbose", false, "turns on very verbose output, including HTTP and libcurl-style logs")
	f.StringVar(&o.inv.Verbosity, "verbosity", "", "sets the verbosity level for debug logging")

	f.BoolVarP(&o.version, "version", "V", false, "prints version, commit, build date and Go version")
}
