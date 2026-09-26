// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package engine runs request files: it renders each entry, sends the
// request, and evaluates captures and asserts on the response. It never
// prints; everything a front end shows comes from events and results.
package engine

import (
	"io"
	"time"
)

// Verbosity is the level of debug information emitted as log events.
type Verbosity int

// Verbosity levels.
const (
	Quiet Verbosity = iota
	Brief
	Verbose
	VeryVerbose
)

// Options configure a run. Command line options map to these fields; an
// entry's [Options] section overrides them for that entry.
type Options struct {
	// Variables are typed: string, bool, nil, int, int64 or float64.
	Variables map[string]any
	// Secrets are string variables whose values are redacted everywhere.
	Secrets map[string]string
	// FileRoot confines the files a request file reads or writes; empty
	// means the directory of the file (or the working directory).
	FileRoot string
	HTTP     HTTPOptions
	// CookieFile is read before the first request (Netscape format).
	CookieFile    string
	NoCookieStore bool

	// Retry is the number of retries of a failing entry (-1: unlimited).
	Retry         int
	RetryInterval time.Duration // zero: 1s
	// Delay is the pause before each entry (not before retries).
	Delay           time.Duration
	FromEntry       int // 1-based, 0: first
	ToEntry         int // 1-based, 0: last
	NoAssert        bool
	ContinueOnError bool
	Verbosity       Verbosity
	// BufferedLogs tells that log events are held until their unit ends
	// and then redacted with Runner.Redact, which allows `redact` captures
	// in verbose mode.
	BufferedLogs bool

	// Stdout receives responses written with `output: -`. Nil: discarded.
	Stdout io.Writer
	// Version is the sonde version, for the default User-Agent.
	Version string
	// DefaultUserAgent replaces `sonde/<version>` as the User-Agent sent
	// when neither the entry nor the options set one.
	DefaultUserAgent string
	// Validator checks every final response against a contract, after
	// the explicit asserts (skipped with NoAssert). Nil: no contract.
	// Job.Validator replaces it for one job.
	Validator ResponseValidator
	// OnEvent receives events serially, in order. Nil: discarded.
	OnEvent func(Event)

	// Now and UUID replace the clock and UUID source of the template
	// functions (tests). Nil: system clock, random UUIDs.
	Now  func() time.Time
	UUID func() string
}

// HTTPOptions are the transport options.
type HTTPOptions struct {
	AWSSigV4          string
	CACert            string
	ClientCert        string
	ClientKey         string
	Compressed        bool
	ConnectTimeout    time.Duration
	ConnectTo         []string
	Digest            bool
	FollowLocation    bool
	LocationTrusted   bool
	Headers           []string // "Name: value"
	HTTPVersion       HTTPVersion
	Insecure          bool
	IPResolve         IPResolve
	MaxFilesize       int64
	LimitRate         int64
	MaxRedirects      int // -1: unlimited; 0 here means the default (50)
	Negotiate         bool
	Netrc             bool
	NetrcFile         string
	NetrcOptional     bool
	NetrcAllowReroute bool
	NoProxy           string
	NTLM              bool
	PathAsIs          bool
	PinnedPublicKey   string
	Proxy             string
	Resolve           []string
	Timeout           time.Duration
	UnixSocket        string
	User              string
	UserAgent         string
}

// HTTPVersion is the requested protocol.
type HTTPVersion int

// Protocol versions.
const (
	HTTPDefault HTTPVersion = iota
	HTTP10
	HTTP11
	HTTP2
	HTTP3
)

// IPResolve restricts name resolution to one IP family.
type IPResolve int

// IP families.
const (
	IPAny IPResolve = iota
	IPv4
	IPv6
)
