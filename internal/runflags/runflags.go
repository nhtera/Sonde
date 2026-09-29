// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package runflags renders a run's invocation back to the command line
// that builds the same run: the inverse of runplan. The desktop app uses
// it to copy a run as a `sonde` command.
package runflags

import (
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/runplan"
)

// flag is one flag of `sonde run` and the Invocation field it sets.
// direct flags apply their value whether or not they were given (runplan
// reads the field as is); the others matter only when given, falling
// through to the environment otherwise.
type flag struct {
	name   string
	direct bool
	b      *bool
	s      *string
	i      *int
	list   *[]string
}

// flags lists every flag of inv, in the order of `sonde run --help`.
func flags(inv *runplan.Invocation) []flag {
	d := func(name string, f flag) flag { f.name, f.direct = name, true; return f }
	p := func(name string, f flag) flag { f.name = name; return f }
	return []flag{
		d("variable", flag{list: &inv.Variables}),
		d("variables-file", flag{list: &inv.VariablesFiles}),
		d("secret", flag{list: &inv.Secrets}),
		d("secrets-file", flag{list: &inv.SecretsFiles}),
		d("cacert", flag{s: &inv.CACert}),
		d("cert", flag{s: &inv.Cert}),
		d("key", flag{s: &inv.Key}),
		p("compressed", flag{b: &inv.Compressed}),
		p("connect-timeout", flag{s: &inv.ConnectTimeout}),
		d("connect-to", flag{list: &inv.ConnectTo}),
		p("insecure", flag{b: &inv.Insecure}),
		p("ipv4", flag{b: &inv.IPv4}),
		p("ipv6", flag{b: &inv.IPv6}),
		d("http1.0", flag{b: &inv.HTTP10}),
		p("http1.1", flag{b: &inv.HTTP11}),
		p("http2", flag{b: &inv.HTTP2}),
		p("http3", flag{b: &inv.HTTP3}),
		p("limit-rate", flag{s: &inv.LimitRate}),
		p("location", flag{b: &inv.Location}),
		p("location-trusted", flag{b: &inv.LocationTrusted}),
		p("max-filesize", flag{s: &inv.MaxFilesize}),
		p("max-redirs", flag{s: &inv.MaxRedirs}),
		p("max-time", flag{s: &inv.MaxTime}),
		d("netrc", flag{b: &inv.Netrc}),
		d("netrc-file", flag{s: &inv.NetrcFile}),
		d("netrc-optional", flag{b: &inv.NetrcOptional}),
		d("no-proxy", flag{s: &inv.NoProxy}),
		d("path-as-is", flag{b: &inv.PathAsIs}),
		d("pinnedpubkey", flag{s: &inv.PinnedPubKey}),
		d("proxy", flag{s: &inv.Proxy}),
		d("resolve", flag{list: &inv.Resolve}),
		d("unix-socket", flag{s: &inv.UnixSocket}),
		p("user", flag{s: &inv.User}),
		p("user-agent", flag{s: &inv.UserAgent}),
		d("header", flag{list: &inv.Header}),
		d("digest", flag{b: &inv.Digest}),
		d("negotiate", flag{b: &inv.Negotiate}),
		d("ntlm", flag{b: &inv.NTLM}),
		d("ssl-no-revoke", flag{b: &inv.SSLNoRevoke}),
		d("aws-sigv4", flag{s: &inv.AWSSigV4}),
		d("cookie", flag{s: &inv.Cookie}),
		d("cookie-jar", flag{s: &inv.CookieJar}),
		p("no-cookie-store", flag{b: &inv.NoCookieStore}),
		p("continue-on-error", flag{b: &inv.ContinueOnError}),
		p("delay", flag{s: &inv.Delay}),
		d("from-entry", flag{i: &inv.FromEntry}),
		d("to-entry", flag{i: &inv.ToEntry}),
		p("no-assert", flag{b: &inv.NoAssert}),
		p("repeat", flag{s: &inv.Repeat}),
		p("retry", flag{s: &inv.Retry}),
		p("retry-interval", flag{s: &inv.RetryInterval}),
		d("file-root", flag{s: &inv.FileRoot}),
		d("glob", flag{list: &inv.Glob}),
		p("jobs", flag{i: &inv.Jobs}),
		p("parallel", flag{b: &inv.Parallel}),
		d("progress-bar", flag{b: &inv.ProgressBar}),
		p("test", flag{b: &inv.Test}),
		d("env", flag{s: &inv.Env}),
		d("config", flag{s: &inv.Config}),
		d("data", flag{s: &inv.Data}),
		d("data-secret", flag{list: &inv.DataSecrets}),
		d("openapi", flag{s: &inv.OpenAPI.Spec}),
		d("openapi-server", flag{s: &inv.OpenAPI.Server}),
		d("openapi-strict", flag{b: &inv.OpenAPI.Strict}),
		d("openapi-allow-remote", flag{b: &inv.OpenAPI.AllowRemote}),
		p("include", flag{b: &inv.Include}),
		p("json", flag{b: &inv.JSON}),
		p("no-output", flag{b: &inv.NoOutput}),
		p("no-pretty", flag{b: &inv.NoPretty}),
		p("pretty", flag{b: &inv.Pretty}),
		d("output", flag{s: &inv.Output}),
		p("error-format", flag{s: &inv.ErrorFormat}),
		d("curl", flag{s: &inv.Curl}),
		d("report-html", flag{s: &inv.ReportHTML}),
		d("report-json", flag{s: &inv.ReportJSON}),
		d("report-junit", flag{s: &inv.ReportJUnit}),
		d("report-tap", flag{s: &inv.ReportTAP}),
		p("verbose", flag{b: &inv.Verbose}),
		p("very-verbose", flag{b: &inv.VeryVerbose}),
		p("verbosity", flag{s: &inv.Verbosity}),
	}
}

// Names returns the long name of every flag Args renders.
func Names() []string {
	var names []string
	for _, f := range flags(&runplan.Invocation{}) {
		names = append(names, f.name)
	}
	return names
}

// Args returns the arguments of `sonde` (without the program name) that
// build the same run as inv: the subcommand ("run", or "test"), the flags,
// then the files. A flag is rendered when it was given (Set) or, for a
// flag whose value applies whether or not it was given, when its value
// is set; a list flag once per element. Values are raw: secrets included.
func Args(inv *runplan.Invocation) []string {
	args := []string{"run"}
	if inv.Cmd == "test" {
		args[0] = "test"
	}
	for _, f := range flags(inv) {
		given := inv.Changed(f.name)
		switch {
		case f.list != nil:
			for _, v := range *f.list {
				args = appendValue(args, f.name, v)
			}
		case f.b != nil:
			if given || f.direct && *f.b {
				if *f.b {
					args = append(args, "--"+f.name)
				} else {
					args = append(args, "--"+f.name+"=false")
				}
			}
		case f.s != nil:
			if given || f.direct && *f.s != "" {
				args = appendValue(args, f.name, *f.s)
			}
		case f.i != nil:
			if given || f.direct && *f.i != 0 {
				args = append(args, "--"+f.name, strconv.Itoa(*f.i))
			}
		}
	}
	for _, file := range inv.Files {
		if strings.HasPrefix(file, "-") && file != "-" {
			args = append(args, "--")
			break
		}
	}
	return append(args, inv.Files...)
}

// appendValue appends a flag with its value, joined when the value could
// be read as a flag.
func appendValue(args []string, name, v string) []string {
	if strings.HasPrefix(v, "-") {
		return append(args, "--"+name+"="+v)
	}
	return append(args, "--"+name, v)
}
