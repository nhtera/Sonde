// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

// CurlEntry is one entry rendered to its curl command line by RenderCurl,
// without sending anything.
type CurlEntry struct {
	// Index is the entry's 1-based position in the file, exactly as
	// Options.FromEntry/ToEntry address it; RenderCurl never renumbers.
	Index int
	// Command is the curl command line, secrets redacted exactly like a
	// run's --curl FILE. Empty when Err is set.
	Command string
	// Undefined lists, in first-use order, the variable names this entry
	// referenced with no defined value (typically one only a capture of
	// an earlier entry, never run here, would set): each stays a literal
	// {{name}} in Command instead of failing. Scoped to this entry alone
	// — a name undefined here is reported again for every other entry
	// that also references it, never assumed defined from one entry to
	// the next.
	Undefined []string
	// Skipped says why an entry has no Command and no Err: a WebSocket
	// entry, which curl can not run. Exactly one of Command, Err and
	// Skipped is set.
	Skipped string
	// Err is set instead of Command when this one entry could not be
	// rendered at all: a value with its own strict syntax (an
	// [Options] repeat/skip/max-redirs/... expecting a number or
	// boolean, not a bare {{name}} it has no source for) or a body file
	// this call's FileRoot cannot supply (missing, unreadable, or not a
	// regular file). It is an *Error for an evaluation error (an invalid
	// option value, an undefined variable where no placeholder fits);
	// other failures, such as a body file that can't be read, are plain
	// errors. Every other entry is still attempted.
	Err error
}

// maxUndefinedVariables bounds how many distinct undefined variables
// RenderCurl discovers for one entry before giving up on it (Err, not a
// hang), so a pathological file cannot grow its Undefined list without
// bound; a real file never comes close.
const maxUndefinedVariables = 1000

// RenderCurl renders the entries of the file named name, with content
// src, to their curl command lines without sending anything: those
// selected by Options.FromEntry/ToEntry (1-based, as in a run; both zero
// select every entry). ctx may cancel the call between entries. A file
// that does not parse is returned as an *Error of kind ErrorParse.
//
// Variables, secrets and every HTTP option come from the runner's options
// exactly as a run would use them; its run-only fields (Retry, Delay,
// ...) are never read. Options.FileRoot confines file bodies and
// multipart files exactly like a run (empty: the directory of name).
// Secrets found while rendering (credentials) are added to the runner's.
//
// Every entry is isolated: an invalid FromEntry/ToEntry range is the only
// other error RenderCurl itself returns; a problem specific to one entry
// (an undefined variable in a value with its own strict syntax, a body
// file RenderCurl can't supply) is reported in that entry's own Err, and
// every other entry is still rendered. A body file that exists but is not
// a regular one (a FIFO, a device, a directory) is never opened at all —
// only Stat, which cannot block, checks it — so it reports the same
// error a missing file would rather than hanging forever, since a caller
// has no way to cancel a blocking read once it has started.
//
// A variable no source defines is not an error by itself: it stays a
// literal {{name}} in the rendered command (Undefined names it),
// evaluated once per entry — never written back to the shared variables,
// so one entry's undefined name is never mistaken for defined by another.
// The literal placeholder works in any plain-text value (a header, the
// URL, a JSON or form-urlencoded body, ...); it does not work where the
// file syntax requires a real number or boolean ([Options]
// repeat/skip/max-redirs/limit-rate/...), which is reported through that
// entry's Err instead.
func (r *Runner) RenderCurl(ctx context.Context, name string, src []byte) ([]CurlEntry, error) {
	file, err := syntax.Parse(name, src, syntax.DialectFor(name))
	if err != nil {
		var perr *syntax.Error
		if errors.As(err, &perr) {
			return nil, &Error{parse: perr, file: name, src: src}
		}
		return nil, err
	}
	return r.renderCurl(ctx, name, src, file)
}

// renderCurl is RenderCurl on a parsed file.
func (r *Runner) renderCurl(ctx context.Context, name string, src []byte, file *syntax.File) ([]CurlEntry, error) {
	opt := r.opt
	entries := file.Entries
	if opt.FromEntry < 0 || opt.ToEntry < 0 {
		return nil, fmt.Errorf("engine: FromEntry/ToEntry must not be negative (got %d, %d)", opt.FromEntry, opt.ToEntry)
	}
	if opt.FromEntry > 0 && opt.ToEntry > 0 && opt.FromEntry > opt.ToEntry {
		return nil, fmt.Errorf("engine: FromEntry (%d) is after ToEntry (%d)", opt.FromEntry, opt.ToEntry)
	}
	first, last := 1, len(entries)
	if opt.FromEntry > 0 {
		first = opt.FromEntry
	}
	if opt.ToEntry > 0 && opt.ToEntry < last {
		last = opt.ToEntry
	}

	vars, err := r.variables(unitIO{})
	if err != nil {
		return nil, err
	}
	rootDir := opt.FileRoot
	if rootDir == "" {
		rootDir = filepath.Dir(name)
	}
	root, err := sandbox.Open(rootDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	client, err := httpx.NewClient(httpx.ClientConfig{
		Sandbox: root, NoCookieStore: opt.NoCookieStore, Version: moduleVersion(), UserAgent: opt.DefaultUserAgent,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	u := &unit{runner: r, file: file, name: name, src: src, root: root, rootDir: rootDir, client: client, forExport: true}
	u.env = &template.Env{Vars: vars, Now: opt.Now, UUID: opt.UUID, ReadFile: u.readFile}

	var out []CurlEntry
	for i := first; i <= last && i <= len(entries); i++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		entry := entries[i-1]
		set, clearAll := cookieCommands(entry.Request)
		for _, s := range set {
			_ = u.client.AddCookie(s)
		}
		if clearAll {
			u.client.ClearCookies()
		}
		eo, spec, undefined, err := u.renderForExport(entry)
		if err != nil {
			if re, ok := err.(*runerr.Error); ok {
				err = &Error{run: re, file: name, src: src, entry: entry.Request.Method.Span.Start.Line}
			}
			out = append(out, CurlEntry{Index: i, Undefined: undefined, Err: err})
			continue
		}
		if eo == nil {
			continue // skip: true, or repeat: 0 — a run never sends it either
		}
		cmd := u.entryCurl(entry, spec, &eo.http, eo)
		if cmd == "" {
			out = append(out, CurlEntry{Index: i, Undefined: undefined, Skipped: "a WebSocket entry has no curl equivalent"})
			continue
		}
		out = append(out, CurlEntry{Index: i, Command: cmd, Undefined: undefined})
	}

	// Redacted once, after every entry, with the run's final secret
	// union — opt.Secrets plus any credential protectCredentials found
	// along the way, from any entry — matching --curl's own "written
	// after the run" contract, rather than each entry's own partial view
	// of the secret registry as it was rendered.
	for i := range out {
		if out[i].Err == nil {
			out[i].Command = r.Redact(out[i].Command)
		}
	}
	return out, nil
}

// renderForExport builds one entry's effective options and request. A
// variable Vars does not define renders as a literal {{name}} instead of
// failing (see RenderCurl and template.Env.Missing); a nil *entryOptions
// with a nil error means the entry is skipped, exactly as a run would
// skip it.
func (u *unit) renderForExport(e *syntax.Entry) (*entryOptions, *httpx.RequestSpec, []string, error) {
	var undefined []string
	seen := map[string]bool{}
	u.env.Missing = func(name string) (value.Value, bool) {
		if !seen[name] {
			if len(undefined) >= maxUndefinedVariables {
				return nil, false // falls through to the normal UndefinedVariable error
			}
			seen[name] = true
			undefined = append(undefined, name)
		}
		return value.String("{{" + name + "}}"), true
	}
	defer func() { u.env.Missing = nil }()

	eo, err := u.entryOptions(e)
	if err != nil {
		return nil, nil, undefined, err
	}
	if eo.skip || (eo.repeat != nil && *eo.repeat == 0) {
		return nil, nil, nil, nil
	}
	u.protectCredentials(eo.http.User)
	spec, err := u.buildRequest(e.Request)
	if err != nil {
		return nil, nil, undefined, err
	}
	return eo, spec, undefined, nil
}
