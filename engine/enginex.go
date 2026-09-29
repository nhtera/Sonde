// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/grpcx"
	"github.com/nhtera/sonde/internal/netpolicy"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/stream"
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

	enginex.EnableHostEvents = func(runner any) { runner.(*Runner).hostEvents = true }
	enginex.UnitStarted = func(ev any) (func(string) string, bool) {
		e, ok := ev.(unitStarted)
		return e.redact, ok
	}
	enginex.RequestSent = func(ev any) (int, int, exchange.Request, bool) {
		e, ok := ev.(requestSent)
		return e.index, e.call, e.req, ok
	}
	enginex.EntryRedacts = func(ev any) (int, bool) {
		e, ok := ev.(entryRedacts)
		return e.index, ok
	}
	enginex.EntrySkipped = func(ev any) (int, string, bool) {
		e, ok := ev.(entrySkipped)
		return e.index, e.reason, ok
	}
	enginex.Transport = func(err any) string {
		var e *Error
		if er, ok := err.(error); ok && errors.As(er, &e) && e.run != nil {
			return e.run.Transport
		}
		return ""
	}
	enginex.CaptureRedacted = func(res any, entry, capture int) bool {
		u, _ := res.(*UnitResult)
		if u == nil || entry < 0 || entry >= len(u.Entries) {
			return false
		}
		secret := u.redacted[u.Entries[entry]]
		return capture >= 0 && capture < len(secret) && secret[capture]
	}
	enginex.SeedCookies = func(runner any, cookies any) {
		runner.(*Runner).seedCookies = append([]Cookie(nil), cookies.([]Cookie)...)
	}
	enginex.RevealCurl = func(runner any) { runner.(*Runner).revealCurl = true }
	enginex.DialWS = func(ctx context.Context, runner any, job any, entry int, onMessage func(exchange.Message)) (*stream.Interactive, func(string) string, error) {
		return runner.(*Runner).dialWS(ctx, job.(Job), entry, onMessage)
	}
	enginex.GRPCDescriptors = func(ctx context.Context, runner any, job any, entry int) (*grpcx.Descriptors, error) {
		return runner.(*Runner).grpcDescriptorFiles(ctx, job.(Job), entry)
	}
}
