// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package enginex builds engine result values from internal types, for
// internal tests that need results without running a file, and reaches
// engine settings that are not part of its public API. Package engine
// sets these functions when it is initialized; they take and return
// engine types as any, since this package cannot import engine.
package enginex

import (
	"github.com/nhtera/sonde/internal/netpolicy"
	"github.com/nhtera/sonde/internal/runerr"
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
)
