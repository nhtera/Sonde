// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/stream"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// Streamed entries (docs/decisions/0004-streaming-protocols.md): an entry
// with a [SondeMessages] section is a WebSocket exchange; an entry with a
// sonde-stream-* option reads its body as Server-Sent Events.

// streamOptions are the sonde-stream-* options of an entry.
type streamOptions struct {
	set  bool // at least one option: the body is read as an event stream
	opts stream.Options
}

// streamOption evaluates a sonde-stream-* option into so.
func (u *unit) streamOption(o *syntax.Option, so *streamOptions) error {
	so.set = true
	var err error
	var n int64
	switch o.Name {
	case "sonde-stream-count":
		n, err = u.naturalOption(o)
		so.opts.Count = int(n)
	case "sonde-stream-max-bytes":
		so.opts.MaxBytes, err = u.naturalOption(o)
	case "sonde-stream-timeout":
		so.opts.Timeout, err = u.durationOption(o)
	}
	return err
}

// messages returns the [SondeMessages] section of a request, or nil.
func messages(r *syntax.Request) *syntax.Section {
	for _, s := range r.Sections {
		if s.Kind == syntax.SectionMessages {
			return s
		}
	}
	return nil
}

// usesQuery reports whether a capture or assert of r uses a query of kind.
func usesQuery(r *syntax.Response, kind syntax.QueryKind) bool {
	if r == nil {
		return false
	}
	for _, s := range r.Sections {
		for _, c := range s.Captures {
			if c.Query.Kind == kind {
				return true
			}
		}
		for _, a := range s.Asserts {
			if a.Query.Kind == kind {
				return true
			}
		}
	}
	return false
}

// send performs the exchange of an entry: a gRPC call, a WebSocket script,
// a streamed or a plain HTTP request. The error is located in the entry.
func (u *unit) send(ctx context.Context, e *syntax.Entry, index int, spec *httpx.RequestSpec, opts *httpx.Options, eo *entryOptions) ([]httpx.Call, *runerr.Error) {
	so := eo.stream.opts
	so.OnMessage = func(m exchange.Message) {
		if m.Direction == exchange.Sent {
			u.emit(MessageSent{Index: index, Message: m})
		} else {
			u.emit(MessageReceived{Index: index, Message: m})
		}
	}
	if s := grpcSection(e.Request); s != nil {
		return u.grpcCall(ctx, e, s, spec, opts, so, eo.stream.set)
	}
	if s := messages(e.Request); s != nil {
		return u.webSocket(ctx, e, s, spec, opts, so)
	}
	var st *exchange.Stream
	if eo.stream.set {
		opts.ReadStream = func(_ exchange.Headers, body io.Reader, stop func()) error {
			s, err := stream.ReadSSE(body, stop, so)
			st = s
			return err
		}
	}
	calls, err := u.client.Execute(ctx, spec, opts)
	if err != nil {
		if st != nil && len(calls) > 0 { // keep what the failed stream read
			calls[len(calls)-1].Response.Stream = st
		}
		return calls, httpError(e.Request.URL.Span, err)
	}
	final := calls[len(calls)-1].Response
	switch {
	case st != nil:
		final.Stream = st
		if st.StopReason == exchange.StopMaxBytes {
			u.log(LogWarning, fmt.Sprintf("the event stream reached sonde-stream-max-bytes after %d event(s)", len(st.Messages)))
		}
	case usesQuery(e.Response, syntax.QuerySondeStream):
		// Without sonde-stream-* options, sondeStream reads the whole body.
		if body, err := final.DecodedBody(); err == nil {
			final.Stream = stream.SSEStream(body)
		}
	}
	return calls, nil
}

// httpError locates a transport error at span. A host outside the
// allowlist is final: retrying the entry would be refused the same way.
func httpError(span syntax.Span, err error) *runerr.Error {
	e := runerr.New(span, runerr.HTTP, false)
	e.Transport = transportClass(err)
	var he *httpx.Error
	if errors.As(err, &he) {
		e.Value, e.Reason = he.Description, he.Msg
		e.Final = he.Kind == httpx.ErrHostDenied
	} else {
		e.Value, e.Reason = "HTTP connection", err.Error()
	}
	return e
}

// transportClass names the class of a transport failure (see
// runerr.Error.Transport).
func transportClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var he *httpx.Error
	if !errors.As(err, &he) {
		return "other"
	}
	switch he.Kind {
	case httpx.ErrConnect:
		return "connect"
	case httpx.ErrResolve:
		return "resolve"
	case httpx.ErrTimeout:
		return "timeout"
	case httpx.ErrTLS:
		return "tls"
	case httpx.ErrHostDenied:
		return "host-denied"
	}
	return "other"
}

// webSocket runs the [SondeMessages] steps of an entry.
func (u *unit) webSocket(ctx context.Context, e *syntax.Entry, s *syntax.Section, spec *httpx.RequestSpec, opts *httpx.Options, so stream.Options) ([]httpx.Call, *runerr.Error) {
	if re := invalidWebSocket(e.Request); re != nil {
		return nil, re
	}
	steps := make([]stream.Step, len(s.Messages))
	for i, m := range s.Messages {
		st, err := u.step(m)
		if err != nil {
			return nil, asRunErr(err, m.Span)
		}
		steps[i] = st
	}
	up, err := u.client.Upgrade(ctx, spec, opts)
	if err != nil {
		return nil, httpError(e.Request.URL.Span, err)
	}
	resp, err := stream.WebSocket(ctx, up, steps, so)
	var calls []httpx.Call
	if resp != nil {
		calls = []httpx.Call{{Request: up.Request, Response: resp, Timings: resp.Timings}}
	}
	var se *stream.StepError
	switch {
	case err == nil:
		return calls, nil
	case errors.As(err, &se):
		re := runerr.New(s.Messages[se.Index].Span, runerr.Stream, false)
		re.Value, re.Reason = "WebSocket", se.Err.Error()
		return calls, re
	case resp != nil:
		re := runerr.New(e.Request.URL.Span, runerr.Stream, false)
		re.Value, re.Reason = "WebSocket", err.Error()
		return calls, re
	}
	return nil, httpError(e.Request.URL.Span, err)
}

// invalidWebSocket checks what a WebSocket entry can not have: a method
// other than GET, or a request body.
func invalidWebSocket(req *syntax.Request) *runerr.Error {
	invalid := func(span syntax.Span, reason string) *runerr.Error {
		re := runerr.New(span, runerr.Stream, false)
		re.Value, re.Reason = "WebSocket", reason
		return re
	}
	if req.Method.Value != "GET" {
		return invalid(req.Method.Span, "a WebSocket entry must use GET")
	}
	if req.Body != nil {
		return invalid(req.Body.Span, "a WebSocket entry has no request body: send messages with `send` steps")
	}
	for _, sec := range req.Sections {
		switch sec.Kind {
		case syntax.SectionFormParams, syntax.SectionMultipart:
			return invalid(sec.Span, "a WebSocket entry has no request body: send messages with `send` steps")
		}
	}
	return nil
}

// step renders a [SondeMessages] step.
func (u *unit) step(m *syntax.MessageStep) (stream.Step, error) {
	switch m.Kind {
	case syntax.StepSend:
		b, err := u.body(m.Value.(syntax.Bytes))
		return stream.Step{Kind: stream.Send, Data: b.Data, Binary: b.Kind != httpx.BodyText}, err
	case syntax.StepReceive:
		n, err := u.stepNumber(m, 1, func(n int) bool { return n >= 1 }, "integer >= 1")
		return stream.Step{Kind: stream.Receive, Count: n}, err
	default:
		n, err := u.stepNumber(m, 1000, syntax.ValidCloseCode, "close code 1000-4999 (not 1004-1006 or 1015)")
		return stream.Step{Kind: stream.Close, Code: n}, err
	}
}

// stepNumber is the value of a receive or close step, def without one. A
// literal was checked by the parser; a placeholder is checked here.
func (u *unit) stepNumber(m *syntax.MessageStep, def int, valid func(int) bool, what string) (int, error) {
	switch v := m.Value.(type) {
	case *syntax.Number:
		return int(v.Int), nil
	case *syntax.Placeholder:
		x, err := u.env.Eval(v.Expr)
		if err != nil {
			return 0, err
		}
		if i, ok := x.(value.Int); ok && valid(int(i)) {
			return int(i), nil
		}
		return 0, invalidType(v.Expr.Span, value.Repr(x), what)
	}
	return def, nil
}

// noCurl says why an entry has no curl equivalent, or "".
func noCurl(r *syntax.Request) string {
	switch {
	case messages(r) != nil:
		return "a WebSocket entry has no curl equivalent"
	case grpcSection(r) != nil:
		return "a gRPC entry has no curl equivalent"
	}
	return ""
}

// entryCurl is the curl command of an entry: none for a WebSocket or gRPC
// entry, one reading without buffering for an event stream.
func (u *unit) entryCurl(e *syntax.Entry, spec *httpx.RequestSpec, opts *httpx.Options, eo *entryOptions) string {
	if noCurl(e.Request) != "" {
		return ""
	}
	cmd := u.curlCommand(spec, opts, eo.output)
	if eo.stream.set {
		cmd = "curl --no-buffer" + strings.TrimPrefix(cmd, "curl")
	}
	return cmd
}

// logStream logs the messages of a streamed response, redacted.
func (u *unit) logStream(calls []Call) {
	if u.verbosity < Verbose || len(calls) == 0 {
		return
	}
	s := calls[len(calls)-1].Response.Stream
	if s == nil {
		return
	}
	for _, m := range s.Messages {
		u.debug(fmt.Sprintf("%s %s", messageArrow(m), messageText(s.Protocol, m)))
	}
	if s.StopReason != "" {
		u.debug("Stream stopped: " + string(s.StopReason))
	}
	u.debug("")
}

func messageArrow(m exchange.Message) string {
	if m.Direction == exchange.Sent {
		return ">>"
	}
	return "<<"
}

// messageText describes a message on one line.
func messageText(protocol exchange.Protocol, m exchange.Message) string {
	data := string(m.Data)
	if m.Binary || !utf8.Valid(m.Data) {
		data = "<" + strconv.Itoa(len(m.Data)) + " bytes>"
	}
	if protocol == exchange.ProtocolSSE {
		text := "event: " + m.Event
		if m.ID != "" {
			text += ", id: " + m.ID
		}
		return text + ", data: " + data
	}
	return data
}
