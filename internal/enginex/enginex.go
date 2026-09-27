// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package enginex builds engine result values from internal types, for
// internal tests that need results without running a file. Package engine
// sets the constructors when it is initialized; they return an
// *engine.Error or an engine.Value as any, since this package cannot
// import engine.
package enginex

import (
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
)
