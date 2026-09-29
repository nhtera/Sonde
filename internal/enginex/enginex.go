// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package enginex builds engine result values from internal types, for
// internal tests that need results without running a file, and reaches
// engine settings that are not part of its public API. Package engine
// sets these functions when it is initialized; they take and return
// engine types as any, since this package cannot import engine.
package enginex

import (
	"context"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/grpcx"
	"github.com/nhtera/sonde/internal/netpolicy"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/stream"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

var (
	// RunError returns an *engine.Error for err, raised by the entry whose
	// request is at line entryLine of file, with content src.
	RunError func(err *runerr.Error, file string, src []byte, entryLine int) any
	// ParseError returns an *engine.Error for a parse error of file.
	ParseError func(err *syntax.Error, file string, src []byte) any
	// Value returns an engine.Value holding v.
	Value func(v value.Value) any
	// SetHosts restricts the hosts an *engine.Runner may contact
	// (`sonde mcp --allow-host`): every request URL, redirect and
	// connection is checked, and the entry options that would reroute a
	// connection, send netrc credentials or write a file are refused.
	// It is kept out of engine.Options so the public API does not freeze
	// the policy's shape.
	SetHosts func(runner any, hosts *netpolicy.Policy)

	// Hooks of the desktop app, kept out of the public API so that their
	// shapes do not freeze. Runner is an *engine.Runner, events are
	// engine.Event values, jobs are engine.Job values. The settings
	// (EnableHostEvents, SeedCookies, RevealCurl) are made before the
	// runner's first run, like SetHosts. Raw values handed out (requests,
	// messages) are shared with results: they must not be modified.

	// EnableHostEvents sends the host events below to the event handlers
	// of runner (Options.OnEvent, RunAllOptions.Started). Without it the
	// events of a run are unchanged.
	EnableHostEvents func(runner any)
	// UnitStarted reports whether ev is the first event of a unit and
	// returns the unit's live redactor: it masks the run's secrets, the
	// data row's, and the `redact` captures and credentials found so far.
	// A handler that redacts every event of the unit with it as the event
	// arrives never shows a secret known at that point; see EntryRedacts
	// for the secrets an entry's own captures find.
	UnitStarted func(ev any) (redact func(string) string, ok bool)
	// EntryRedacts reports whether ev says that the attempt of entry index
	// that just started has a `redact` capture. Until its EntryFinished,
	// the attempt's events may hold a value that capture then makes
	// secret: a handler holds them and redacts them at EntryFinished.
	EntryRedacts func(ev any) (index int, ok bool)
	// RequestSent reports whether ev is the sending of a request of entry
	// index: call is its 1-based position in the attempt (a redirect is a
	// new call), req the request as sent, raw. gRPC reflection calls are
	// not reported.
	RequestSent func(ev any) (index, call int, req exchange.Request, ok bool)
	// EntrySkipped reports whether ev is entry index not running: reason
	// is "option" (skip) or "repeat-zero" (repeat: 0).
	EntrySkipped func(ev any) (index int, reason string, ok bool)
	// Transport classifies the transport failure of an *engine.Error:
	// connect, resolve, timeout, tls, host-denied, canceled or other; ""
	// when it is not a transport failure.
	Transport func(err any) string
	// CaptureRedacted reports whether capture (0-based) of the attempt at
	// position entry (0-based) of the *engine.UnitResult res is a `redact`
	// capture.
	CaptureRedacted func(res any, entry, capture int) bool
	// SeedCookies stores cookies ([]engine.Cookie) in the cookie store of
	// every unit runner runs, before its first entry.
	SeedCookies func(runner any, cookies any)
	// RevealCurl makes runner's RenderCurl leave secrets in clear.
	RevealCurl func(runner any)
	// DialWS opens an interactive WebSocket session for entry (1-based) of
	// job, a [SondeMessages] entry, rendered and dialed exactly as a run
	// of job would (proxy, TLS, cookies, host policy, options); its steps
	// are not run. onMessage receives each message, raw; redact is the
	// unit's live redactor. The session ends when ctx ends. A refused
	// upgrade returns an ended session, holding the handshake response,
	// with the error.
	DialWS func(ctx context.Context, runner any, job any, entry int, onMessage func(exchange.Message)) (s *stream.Interactive, redact func(string) string, err error)
	// GRPCDescriptors loads the descriptors the [SondeGrpc] section of
	// entry (1-based) of job names in files, as a run would. It returns
	// nil when the section names no file: server reflection is not used.
	GRPCDescriptors func(ctx context.Context, runner any, job any, entry int) (*grpcx.Descriptors, error)
)
