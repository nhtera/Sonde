// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/grpcx"
	"github.com/nhtera/sonde/internal/redact"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/stream"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
)

// Host entry helpers (enginex.DialWS, enginex.GRPCDescriptors) render one
// entry of a job exactly as a run of the job would, without running the
// entries before it: same variables (job, options, data row), file root,
// cookie store (seed cookies included), host policy and options.

// hostUnit prepares a unit for job and returns it with its entry number
// n (1-based). cleanup releases the unit's sandbox and client.
func (r *Runner) hostUnit(job Job, n int) (u *unit, e *syntax.Entry, cleanup func(), err error) {
	src := job.Source
	if src == nil {
		if src, err = readSource(job.Name); err != nil {
			return nil, nil, nil, err
		}
	}
	f, err := syntax.Parse(job.Name, src, syntax.DialectFor(job.Name))
	if err != nil {
		var perr *syntax.Error
		if errors.As(err, &perr) {
			return nil, nil, nil, &Error{parse: perr, file: job.Name, src: src}
		}
		return nil, nil, nil, err
	}
	if n < 1 || n > len(f.Entries) {
		return nil, nil, nil, fmt.Errorf("engine: entry %d out of range (the file has %d)", n, len(f.Entries))
	}
	rootDir := r.opt.FileRoot
	if rootDir == "" {
		rootDir = filepath.Dir(job.Name)
	}
	root, err := sandbox.Open(rootDir)
	if err != nil {
		return nil, nil, nil, err
	}
	uio := unitIO{vars: job.Variables, secrets: job.Secrets, row: job.Row}
	u = &unit{runner: r, file: f, name: job.Name, src: src, root: root, rootDir: rootDir, io: uio, verbosity: r.opt.Verbosity}
	if job.Row != nil {
		u.rowSecrets = redact.New()
		for name, v := range job.Row.Secrets {
			u.rowSecrets.Add(name, v)
		}
	}
	client, err := u.newClient()
	if err != nil {
		_ = root.Close()
		return nil, nil, nil, err
	}
	u.client = client
	cleanup = func() {
		_ = client.Close()
		_ = root.Close()
	}
	vars, err := r.variables(uio)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	u.env = &template.Env{Vars: vars, Now: r.opt.Now, UUID: r.opt.UUID, ReadFile: u.readFile}
	return u, f.Entries[n-1], cleanup, nil
}

// entryError locates a runtime error in the unit's file.
func (u *unit) entryError(err error, e *syntax.Entry, span syntax.Span) *Error {
	return &Error{run: asRunErr(err, span), file: u.name, src: u.src, entry: e.Request.Method.Span.Start.Line}
}

// dialWS opens an interactive WebSocket session for entry n of job, an
// entry with a [SondeMessages] section (whose steps are not run). Its
// messages go to onMessage; redact masks the unit's secrets.
func (r *Runner) dialWS(ctx context.Context, job Job, n int, onMessage func(exchange.Message)) (_ *stream.Interactive, redactFn func(string) string, err error) {
	u, e, cleanup, err := r.hostUnit(job, n)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	req := e.Request
	if messages(req) == nil {
		return nil, nil, fmt.Errorf("engine: entry %d is not a WebSocket entry", n)
	}
	if re := invalidWebSocket(req); re != nil {
		return nil, nil, u.entryError(re, e, req.Span)
	}
	eo, err := u.entryOptions(e)
	if err != nil {
		return nil, nil, u.entryError(err, e, req.Span)
	}
	u.protectCredentials(eo.http.User)
	spec, err := u.buildRequest(req)
	if err != nil {
		return nil, nil, u.entryError(err, e, req.URL.Span)
	}
	set, clearAll := cookieCommands(req)
	for _, s := range set {
		_ = u.client.AddCookie(s)
	}
	if clearAll {
		u.client.ClearCookies()
	}
	opts := eo.http
	up, err := u.client.Upgrade(ctx, spec, &opts)
	if err != nil {
		return nil, nil, &Error{run: httpError(req.URL.Span, err), file: u.name, src: u.src, entry: req.Method.Span.Start.Line}
	}
	so := eo.stream.opts
	so.OnMessage = onMessage
	ia, err := stream.DialInteractive(ctx, up, so)
	if err != nil {
		var re *runerr.Error
		if ia == nil { // no handshake response: a transport failure
			re = httpError(req.URL.Span, err)
		} else {
			re = runerr.New(req.URL.Span, runerr.Stream, false)
			re.Value, re.Reason = "WebSocket", err.Error()
		}
		return ia, nil, &Error{run: re, file: u.name, src: u.src, entry: req.Method.Span.Start.Line}
	}
	go func() {
		<-ia.Done()
		cleanup()
	}()
	return ia, u.redact, nil
}

// grpcDescriptorFiles loads the descriptors the [SondeGrpc] section of
// entry n of job names in files, as a run would. It returns nil when the
// section names no file (a run would use server reflection).
func (r *Runner) grpcDescriptorFiles(ctx context.Context, job Job, n int) (*grpcx.Descriptors, error) {
	u, e, cleanup, err := r.hostUnit(job, n)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	s := grpcSection(e.Request)
	if s == nil {
		return nil, fmt.Errorf("engine: entry %d is not a gRPC entry", n)
	}
	files, rerr := u.grpcFiles(s)
	if rerr == nil && len(files.Protos) == 0 && len(files.Protosets) == 0 {
		return nil, nil
	}
	var d *grpcx.Descriptors
	if rerr == nil {
		d, rerr = u.loadGrpcFiles(ctx, s, files)
	}
	if rerr != nil {
		return nil, &Error{run: rerr, file: u.name, src: u.src, entry: e.Request.Method.Span.Start.Line}
	}
	return d, nil
}
