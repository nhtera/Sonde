// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/netpolicy"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

func init() {
	enginex.RunError = func(err *runerr.Error, file string, src []byte, entryLine int) any {
		return &Error{run: err, file: file, src: src, entry: entryLine}
	}
	enginex.ParseError = func(err *syntax.Error, file string, src []byte) any {
		return &Error{parse: err, file: file, src: src}
	}
	enginex.Value = func(v value.Value) any { return Value{v: v} }
	enginex.SetHosts = func(runner any, hosts *netpolicy.Policy) { runner.(*Runner).hosts = hosts }
}
