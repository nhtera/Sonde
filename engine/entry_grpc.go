// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/grpcx"
	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/stream"
	"github.com/nhtera/sonde/internal/syntax"
)

// gRPC calls (docs/decisions/0005-grpc.md): an entry with a [SondeGrpc]
// section calls the method its URL path names, with messages as JSON.

// grpcSection returns the [SondeGrpc] section of a request, or nil.
func grpcSection(r *syntax.Request) *syntax.Section {
	for _, s := range r.Sections {
		if s.Kind == syntax.SectionGrpc {
			return s
		}
	}
	return nil
}

// grpcCache keeps the descriptors loaded from files, keyed by file root and
// files. An entry is reused while every file it read is unchanged, so a
// Runner that outlives a run sees edited files.
type grpcCache struct {
	mu    sync.Mutex
	files map[string]*grpcFiles
}

// grpcFiles are the descriptors of one set of files. mu serializes their
// loading, so that different sets load in parallel.
type grpcFiles struct {
	mu   sync.Mutex
	d    *grpcx.Descriptors
	read map[string][sha256.Size]byte // every file read, and its hash
}

// entry returns the cache entry of key.
func (c *grpcCache) entry(key string) *grpcFiles {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files == nil {
		c.files = map[string]*grpcFiles{}
	}
	f := c.files[key]
	if f == nil {
		f = &grpcFiles{}
		c.files[key] = f
	}
	return f
}

// current reports whether every file the descriptors were loaded from is
// unchanged.
func (f *grpcFiles) current(read func(string) ([]byte, error)) bool {
	if f.d == nil {
		return false
	}
	for name, sum := range f.read {
		data, err := read(name)
		if err != nil || sha256.Sum256(data) != sum {
			return false
		}
	}
	return true
}

// grpcError is a failed gRPC call located at span.
func grpcError(span syntax.Span, value, reason string) *runerr.Error {
	e := runerr.New(span, runerr.GRPC, false)
	e.Value, e.Reason = value, reason
	return e
}

// grpcCall performs the gRPC call of an entry.
func (u *unit) grpcCall(ctx context.Context, e *syntax.Entry, s *syntax.Section, spec *httpx.RequestSpec, opts *httpx.Options, so stream.Options, streamed bool) ([]httpx.Call, *runerr.Error) {
	req := e.Request
	urlSpan := req.URL.Span
	if req.Method.Value != "POST" {
		return nil, grpcError(req.Method.Span, "gRPC", "a gRPC call must use POST")
	}
	if len(spec.Form) > 0 || len(spec.Multipart) > 0 {
		return nil, grpcError(urlSpan, "gRPC", "a gRPC call sends its request message as a JSON body, not [Form] or [Multipart]")
	}
	if messages(req) != nil {
		return nil, grpcError(s.Span, "gRPC", "an entry is a gRPC call or a WebSocket exchange: [SondeGrpc] and [SondeMessages] do not go together")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	deadline, sendDeadline := timeout, true
	for _, h := range spec.Headers {
		if grpcx.Reserved(h.Name) {
			return nil, grpcError(urlSpan, "gRPC", "the header "+h.Name+" is set by sonde for a gRPC call")
		}
		if strings.EqualFold(h.Name, "grpc-timeout") {
			d, ok := grpcx.ParseTimeout(h.Value)
			if !ok {
				return nil, grpcError(urlSpan, "gRPC", "invalid grpc-timeout "+strconv.Quote(h.Value)+": expecting up to 8 digits and a unit, H, M, S, m, u or n")
			}
			deadline, sendDeadline = d, false
		}
	}
	target, err := neturl.Parse(spec.URL)
	if err != nil {
		return nil, grpcError(urlSpan, "gRPC", err.Error())
	}
	if _, _, err := grpcx.SplitPath(target.Path); err != nil {
		return nil, grpcError(urlSpan, "gRPC", err.Error())
	}
	metadata := append(append([]exchange.Header(nil), spec.Headers...), grpcx.RequestHeaders(timeout, sendDeadline)...)
	callOpts := *opts
	callOpts.GRPC, callOpts.ReadStream = true, nil

	d, rerr := u.grpcDescriptors(ctx, s, target, metadata, &callOpts, timeout)
	if rerr != nil {
		return nil, rerr
	}
	method, err := d.Method(target.Path)
	if err != nil {
		return nil, grpcError(urlSpan, "gRPC", err.Error())
	}
	if method.ClientStreaming() {
		return nil, grpcError(urlSpan, "gRPC", "client-streaming and bidirectional methods are not supported")
	}
	requestJSON := spec.Body.Data
	msg, err := d.Request(method, requestJSON)
	if err != nil {
		span := urlSpan
		if req.Body != nil {
			span = req.Body.Span
		}
		return nil, grpcError(span, "gRPC request", err.Error())
	}
	spec.Headers = metadata
	spec.Body = httpx.Body{Kind: httpx.BodyBinary, Data: grpcx.Frame(msg)}
	spec.ImplicitContentType = ""

	if !streamed {
		// Without sonde-stream-* options a stream is read to its end, as
		// a body is: only max-time bounds it.
		so.Timeout, so.MaxBytes = timeout+time.Minute, maxBody(opts)
	}
	r := grpcReader{d: d, method: method, lim: so, maxBody: maxBody(opts), start: time.Now(), deadline: deadline}
	*opts = callOpts
	opts.ReadStream = r.read
	calls, err := u.client.Execute(ctx, spec, opts)
	for i := range calls {
		calls[i].Request.Body = requestJSON
	}
	if err != nil {
		if len(calls) > 0 && r.stream != nil {
			calls[len(calls)-1].Response.Stream = r.stream
		}
		// A stream the server reset before its response has no status to
		// assert: the call fails.
		if st, ok := grpcx.ResetStatus(err); ok && len(calls) == 0 {
			return nil, statusError(urlSpan, r.resetStatus(st))
		}
		return calls, httpError(urlSpan, err)
	}
	final := calls[len(calls)-1].Response
	if rerr := r.finish(final, urlSpan); rerr != nil {
		return calls, rerr
	}
	if r.stream != nil && r.stream.StopReason == exchange.StopMaxBytes {
		u.log(LogWarning, fmt.Sprintf("the gRPC stream reached sonde-stream-max-bytes after %d message(s)", len(r.stream.Messages)))
	}
	if st := final.GRPC; st != nil && st.Code != grpcx.CodeOK && !usesQuery(e.Response, syntax.QuerySondeGrpc) {
		return calls, statusError(urlSpan, st)
	}
	return calls, nil
}

// statusError fails a call that ended with a status other than OK.
func statusError(span syntax.Span, st *exchange.GRPCStatus) *runerr.Error {
	reason := st.Status
	if st.Message != "" {
		reason += ": " + st.Message
	}
	return grpcError(span, "gRPC status", reason)
}

// maxBody is the size limit of a reply: max-filesize, or the limit of any
// decoded body.
func maxBody(opts *httpx.Options) int64 {
	if opts.MaxFilesize > 0 {
		return opts.MaxFilesize
	}
	return exchange.DefaultMaxDecodedBody
}

// grpcDescriptors loads the descriptors of a call: from the section's
// files (once per run), or by server reflection (once per unit and
// server).
func (u *unit) grpcDescriptors(ctx context.Context, s *syntax.Section, target *neturl.URL, metadata []exchange.Header, opts *httpx.Options, timeout time.Duration) (*grpcx.Descriptors, *runerr.Error) {
	var files grpcx.Files
	for _, kv := range s.KeyValues {
		key, value, err := u.keyValue(kv)
		if err != nil {
			return nil, asRunErr(err, kv.Value.Span)
		}
		switch key {
		case "proto":
			files.Protos = append(files.Protos, value)
		case "import-path":
			files.ImportPaths = append(files.ImportPaths, value)
		case "protoset":
			files.Protosets = append(files.Protosets, value)
		}
	}
	if len(files.Protos) == 0 && len(files.Protosets) == 0 {
		return u.reflect(ctx, s, target, metadata, opts, timeout)
	}
	f := u.runner.grpc.entry(u.root.Dir() + "\x02" + files.Key())
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.current(u.readFile) {
		return f.d, nil
	}
	read := map[string][sha256.Size]byte{}
	var readMu sync.Mutex
	d, err := files.Load(ctx, func(name string) ([]byte, error) {
		data, err := u.readFile(name)
		if err == nil {
			readMu.Lock()
			read[name] = sha256.Sum256(data)
			readMu.Unlock()
		}
		return data, err
	})
	if err != nil {
		f.d = nil
		return nil, grpcError(s.Span, "gRPC descriptors", err.Error())
	}
	f.d, f.read = d, read
	return d, nil
}

// reflect asks the server for the descriptors of the call's service, with
// the call's metadata and connection options.
func (u *unit) reflect(ctx context.Context, s *syntax.Section, target *neturl.URL, metadata []exchange.Header, opts *httpx.Options, timeout time.Duration) (*grpcx.Descriptors, *runerr.Error) {
	service, _, _ := grpcx.SplitPath(target.Path)
	origin := target.Scheme + "://" + target.Host
	key := origin + "/" + service
	if d := u.reflected[key]; d != nil {
		return d, nil
	}
	// Every reflection call of the entry shares its max-time.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	invoke := func(ctx context.Context, path string, request []byte) ([]byte, error) {
		u.debug("gRPC reflection: POST " + origin + path)
		spec := &httpx.RequestSpec{Method: "POST", URL: origin + path, Headers: metadata,
			Body: httpx.Body{Kind: httpx.BodyBinary, Data: grpcx.Frame(request)}}
		calls, err := u.client.Execute(ctx, spec, opts)
		if err != nil {
			return nil, err
		}
		resp := calls[len(calls)-1].Response
		if st := grpcx.Status(resp.Status, resp.Headers); st.Code != grpcx.CodeOK {
			return nil, &grpcx.StatusError{Status: st}
		}
		msgs, err := grpcMessages(resp.Headers, resp.Body)
		if err != nil {
			return nil, err
		}
		if len(msgs) != 1 {
			return nil, fmt.Errorf("the server replied with %d messages", len(msgs))
		}
		return msgs[0], nil
	}
	d, err := grpcx.Reflect(ctx, service, invoke)
	if err != nil {
		var he *httpx.Error
		if errors.As(err, &he) {
			if he.Kind == httpx.ErrHostDenied {
				return nil, httpError(s.Span, he)
			}
			err = errors.New(he.Msg)
		}
		return nil, grpcError(s.Span, "gRPC reflection", err.Error())
	}
	if u.reflected == nil {
		u.reflected = map[string]*grpcx.Descriptors{}
	}
	u.reflected[key] = d
	return d, nil
}

// grpcMessages splits a whole response body into messages.
func grpcMessages(h exchange.Headers, body []byte) ([][]byte, error) {
	enc, _ := h.Get("grpc-encoding")
	p, err := grpcx.NewParser(enc)
	if err != nil {
		return nil, err
	}
	msgs, err := p.Write(body)
	if err != nil {
		return nil, err
	}
	return msgs, p.Close()
}

// grpcReader reads the reply of a call: whole for a unary method, as a
// stream within lim for a server-streaming one.
type grpcReader struct {
	d        *grpcx.Descriptors
	method   *grpcx.Method
	lim      stream.Options
	maxBody  int64         // the size limit of a unary reply
	start    time.Time     // when the call was sent
	deadline time.Duration // the grpc-timeout sent

	notGRPC bool                 // the response is not a gRPC reply
	replies [][]byte             // unary: the messages
	stream  *exchange.Stream     // server streaming
	reset   *exchange.GRPCStatus // the server reset the stream
	err     error                // an invalid reply
}

// read is the httpx.Options.ReadStream of a call.
func (r *grpcReader) read(h exchange.Headers, body io.Reader, stop func()) error {
	if !grpcx.IsGRPC(h) {
		// An error page (a proxy's 502, a 404): its status decides, and
		// its body is kept as received.
		r.notGRPC = true
		_, err := io.Copy(io.Discard, io.LimitReader(body, r.maxBody))
		return err
	}
	enc, _ := h.Get("grpc-encoding")
	p, err := grpcx.NewParser(enc)
	if err != nil {
		r.err = err
		return nil
	}
	if !r.method.ServerStreaming() {
		data, err := io.ReadAll(io.LimitReader(body, r.maxBody+1))
		switch {
		case err != nil:
			return r.readError(err)
		case int64(len(data)) > r.maxBody:
			r.err = fmt.Errorf("the reply is larger than %d bytes", r.maxBody)
		default:
			if r.replies, r.err = p.Write(data); r.err == nil {
				r.err = p.Close()
			}
		}
		return nil
	}
	s, err := stream.Read(body, stop, r.lim, exchange.ProtocolGRPC, func(chunk []byte) ([]exchange.Message, error) {
		raws, err := p.Write(chunk)
		var ms []exchange.Message
		for _, raw := range raws {
			j, jerr := r.d.Reply(r.method, raw)
			if jerr != nil {
				return ms, &replyError{jerr}
			}
			ms = append(ms, exchange.Message{Data: j})
		}
		if err != nil {
			return ms, &replyError{err}
		}
		return ms, nil
	})
	r.stream = s
	if err != nil {
		return r.readError(err)
	}
	if s.StopReason == exchange.StopClosed {
		r.err = p.Close()
	}
	return nil
}

// readError keeps a stream reset by the server as the call's status, and
// an invalid reply as the call's error; other errors fail the transfer.
func (r *grpcReader) readError(err error) error {
	if st, ok := grpcx.ResetStatus(err); ok {
		r.reset = r.resetStatus(st)
		return nil
	}
	var re *replyError
	if errors.As(err, &re) {
		r.err = re.err
		return nil
	}
	return err
}

// resetStatus is the status of a stream the server reset: a server
// cancels a call whose deadline passed, so that is DEADLINE_EXCEEDED.
// Servers time the deadline with their own clock, hence the margin.
func (r *grpcReader) resetStatus(st *exchange.GRPCStatus) *exchange.GRPCStatus {
	if st.Code == grpcx.CodeCancelled && time.Since(r.start)+r.deadline/10 >= r.deadline {
		return grpcx.NewStatus(grpcx.CodeDeadlineExceeded, "the deadline of "+r.deadline.String()+" expired")
	}
	return st
}

// replyError is a reply message that could not be read, as opposed to a
// failed transfer.
type replyError struct{ err error }

func (e *replyError) Error() string { return e.err.Error() }

// finish sets the status, body and stream of the final response.
func (r *grpcReader) finish(final *exchange.Response, span syntax.Span) *runerr.Error {
	switch {
	case r.reset != nil:
		final.GRPC = r.reset
	case r.notGRPC || r.stream == nil || r.stream.StopReason == exchange.StopClosed:
		final.GRPC = grpcx.Status(final.Status, final.Headers)
	}
	if r.notGRPC {
		return nil
	}
	if r.stream != nil {
		final.Stream = r.stream
		final.Body = jsonArray(r.stream.Messages)
	}
	if st := final.GRPC; r.err != nil && (st == nil || st.Code == grpcx.CodeOK) {
		return grpcError(span, "gRPC reply", r.err.Error())
	}
	if r.stream != nil || final.GRPC == nil || final.GRPC.Code != grpcx.CodeOK {
		if r.stream == nil {
			final.Body = nil
		}
		return nil
	}
	if len(r.replies) != 1 {
		return grpcError(span, "gRPC reply", fmt.Sprintf("the server replied with %d messages to a unary call", len(r.replies)))
	}
	j, err := r.d.Reply(r.method, r.replies[0])
	if err != nil {
		return grpcError(span, "gRPC reply", err.Error())
	}
	final.Body = j
	return nil
}

// jsonArray joins the JSON of messages into an array.
func jsonArray(ms []exchange.Message) []byte {
	parts := make([][]byte, len(ms))
	for i, m := range ms {
		parts[i] = m.Data
	}
	return append(append([]byte{'['}, bytes.Join(parts, []byte{','})...), ']')
}
