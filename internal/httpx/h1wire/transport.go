// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package h1wire

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// Transport is an http.RoundTripper for HTTP/1.x: plain, over TLS, and
// through an HTTP proxy (absolute-form for http://, a CONNECT tunnel for
// https://). It fires the httptrace hooks net/http fires.
type Transport struct {
	// Dial connects to a server or a proxy (DNS and connect hooks fire
	// from the context it is given).
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// TLSConfig is cloned for each TLS connection; ServerName defaults to
	// the request's host.
	TLSConfig *tls.Config
	// Proxy returns the proxy of a request (nil: none); only http://
	// proxies are supported.
	Proxy func(*http.Request) (*url.URL, error)
	// ProxyHeaders are sent with each CONNECT request, in order.
	ProxyHeaders []exchange.Header
	// OfferH2 offers h2 as well as http/1.1 over TLS. A server choosing
	// h2 gets its connection handed back as an *H2Conn error, for an
	// HTTP/2 client to use.
	OfferH2 bool

	pool pool
}

// H2Conn is the error of a request whose server chose HTTP/2 (OfferH2):
// Conn is the TLS connection, handshake done, to Addr.
type H2Conn struct {
	Conn *tls.Conn
	Addr string
}

func (e *H2Conn) Error() string { return "h1wire: " + e.Addr + " negotiated h2" }

// CloseIdleConnections closes the pooled connections.
func (t *Transport) CloseIdleConnections() { t.pool.closeIdle() }

// RoundTrip sends req and reads the response head; the body is read from
// the response. A request that fails on a reused connection before any
// response byte is sent again once on a new connection when its body can
// be replayed.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	w := wireFrom(ctx)
	if w == nil {
		w = &Wire{}
	}
	route, err := t.route(req)
	if err != nil {
		closeBody(req)
		return nil, err
	}
	r, err := t.request(req, w, route)
	if err == nil {
		err = r.check()
	}
	if err != nil {
		closeBody(req)
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		c, err := t.connect(ctx, route, w)
		if err != nil {
			var h2 *H2Conn
			if !errors.As(err, &h2) { // the request goes on over HTTP/2
				closeBody(req)
			}
			return nil, err
		}
		resp, gotByte, err := t.exchange(ctx, c, req, r, w)
		if err == nil {
			return resp, nil
		}
		_ = c.Close()
		if attempt == 0 && c.reused && !gotByte && ctx.Err() == nil && retryable(r) {
			if r.body, err = replayBody(req); err == nil {
				continue
			}
		}
		closeBody(req)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
}

// route is where a request goes: the server address, the proxy if any,
// and the pool key.
type route struct {
	scheme, addr, host string
	proxy              *url.URL
	key                string
}

func (t *Transport) route(req *http.Request) (*route, error) {
	u := req.URL
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("h1wire: unsupported scheme %q", u.Scheme)
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	host := ASCIIHost(u.Hostname())
	r := &route{scheme: u.Scheme, host: host, addr: net.JoinHostPort(host, port)}
	if t.Proxy != nil {
		p, err := t.Proxy(req)
		if err != nil {
			return nil, err
		}
		if p != nil && p.Scheme != "http" {
			return nil, fmt.Errorf("h1wire: unsupported proxy scheme %q", p.Scheme)
		}
		r.proxy = p
	}
	r.key = r.scheme + "|" + r.addr
	if r.proxy != nil {
		r.key += "|" + r.proxy.String()
	}
	return r, nil
}

// request prepares what goes on the wire.
func (t *Transport) request(req *http.Request, w *Wire, rt *route) (*request, error) {
	r := &request{method: req.Method, minor: 1, length: req.ContentLength}
	if r.method == "" {
		r.method = http.MethodGet
	}
	if w.HTTP10 {
		r.minor = 0
	}
	if req.Body != nil && req.Body != http.NoBody {
		r.body = req.Body
		if r.length == 0 {
			r.length = -1
		}
	}
	r.target = req.URL.RequestURI()
	if rt.proxy != nil && rt.scheme == "http" {
		u := *req.URL
		u.User, u.Fragment = nil, ""
		r.target = u.String()
	}
	if w.RequestHeaders != nil {
		r.headers = w.RequestHeaders
	} else {
		host := req.Host
		if host == "" {
			host = ASCIIHost(req.URL.Hostname())
			if p := req.URL.Port(); p != "" {
				host = net.JoinHostPort(host, p)
			} else if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
		}
		r.headers = []exchange.Header{{Name: "Host", Value: host}}
		names := make([]string, 0, len(req.Header))
		for name := range req.Header {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			for _, v := range req.Header[name] {
				r.headers = append(r.headers, exchange.Header{Name: name, Value: v})
			}
		}
		if r.length > 0 {
			r.headers = append(r.headers, exchange.Header{Name: "Content-Length", Value: fmt.Sprint(r.length)})
		} else if r.length < 0 {
			r.headers = append(r.headers, exchange.Header{Name: "Transfer-Encoding", Value: "chunked"})
		}
	}
	if rt.proxy != nil && rt.scheme == "http" && rt.proxy.User != nil && !hasField(r.headers, "Proxy-Authorization") {
		r.headers = append(slices.Clip(r.headers), exchange.Header{Name: "Proxy-Authorization", Value: basicAuth(rt.proxy.User)})
	}
	return r, nil
}

// connect returns a connection for the route: the lease's, an idle one,
// or a new one.
func (t *Transport) connect(ctx context.Context, rt *route, w *Wire) (*conn, error) {
	trace := httptrace.ContextClientTrace(ctx)
	if trace != nil && trace.GetConn != nil {
		trace.GetConn(rt.addr)
	}
	var c *conn
	if w.Lease != nil {
		c = w.Lease.take(rt.key)
	}
	if c == nil {
		c = t.pool.get(rt.key)
	}
	if c != nil {
		if trace != nil && trace.GotConn != nil {
			trace.GotConn(httptrace.GotConnInfo{Conn: c.Conn, Reused: true, WasIdle: true, IdleTime: time.Since(c.idleSince)})
		}
		return c, nil
	}
	c, err := t.dial(ctx, rt)
	if err != nil {
		return nil, err
	}
	if trace != nil && trace.GotConn != nil {
		trace.GotConn(httptrace.GotConnInfo{Conn: c.Conn})
	}
	return c, nil
}

// dial opens a connection: to the proxy (with a CONNECT tunnel for
// https://) or to the server, then TLS for https://.
func (t *Transport) dial(ctx context.Context, rt *route) (*conn, error) {
	addr := rt.addr
	if rt.proxy != nil {
		addr = rt.proxy.Host
		if rt.proxy.Port() == "" {
			addr = net.JoinHostPort(rt.proxy.Hostname(), "80")
		}
	}
	raw, err := t.Dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if rt.proxy != nil && rt.scheme == "https" {
		if err := t.tunnel(ctx, raw, rt); err != nil {
			_ = raw.Close()
			return nil, err
		}
	}
	if rt.scheme != "https" {
		return newConn(raw, rt.key), nil
	}
	cfg := t.TLSConfig.Clone()
	if cfg == nil {
		cfg = &tls.Config{} //nolint:gosec // G402: the caller's config is cloned; this is the zero default
	}
	if cfg.ServerName == "" {
		cfg.ServerName = rt.host
	}
	cfg.NextProtos = []string{"http/1.1"}
	if t.OfferH2 && rt.proxy == nil {
		cfg.NextProtos = []string{"h2", "http/1.1"}
	}
	tc, err := HandshakeTLS(ctx, raw, cfg)
	if err != nil {
		return nil, err
	}
	state := tc.ConnectionState()
	if state.NegotiatedProtocol == "h2" {
		return nil, &H2Conn{Conn: tc, Addr: rt.addr}
	}
	c := newConn(tc, rt.key)
	c.tls = &state
	return c, nil
}

// HandshakeTLS runs the TLS handshake of a client connection, firing the
// TLS httptrace hooks; the connection is closed when it fails.
func HandshakeTLS(ctx context.Context, raw net.Conn, cfg *tls.Config) (*tls.Conn, error) {
	trace := httptrace.ContextClientTrace(ctx)
	if trace != nil && trace.TLSHandshakeStart != nil {
		trace.TLSHandshakeStart()
	}
	tc := tls.Client(raw, cfg)
	err := tc.HandshakeContext(ctx)
	if trace != nil && trace.TLSHandshakeDone != nil {
		trace.TLSHandshakeDone(tc.ConnectionState(), err)
	}
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return tc, nil
}

// ProxyError is a CONNECT request the proxy refused.
type ProxyError struct{ Status string }

func (e *ProxyError) Error() string { return "CONNECT tunnel failed, response " + e.Status }

// tunnel opens a CONNECT tunnel to the server through the proxy.
func (t *Transport) tunnel(ctx context.Context, raw net.Conn, rt *route) error {
	stop := context.AfterFunc(ctx, func() { _ = raw.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	headers := []exchange.Header{{Name: "Host", Value: rt.addr}}
	if rt.proxy.User != nil && !hasField(t.ProxyHeaders, "Proxy-Authorization") {
		headers = append(headers, exchange.Header{Name: "Proxy-Authorization", Value: basicAuth(rt.proxy.User)})
	}
	for _, h := range t.ProxyHeaders {
		if !strings.EqualFold(h.Name, "Host") {
			headers = append(headers, h)
		}
	}
	bw := bufio.NewWriter(raw)
	if err := writeHead(bw, &request{method: "CONNECT", target: rt.addr, minor: 1, headers: headers}); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	// The tunnel starts right after the response head: read it byte by
	// byte so that nothing of the tunnel is buffered here.
	br := bufio.NewReaderSize(oneByteReader{raw}, 16)
	head, err := readHead(br, "CONNECT")
	if err != nil {
		return err
	}
	if head.status < 200 || head.status > 299 {
		return &ProxyError{Status: head.statusText()}
	}
	return nil
}

// oneByteReader reads at most one byte at a time.
type oneByteReader struct{ r io.Reader }

func (o oneByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return o.r.Read(p)
}

// exchange writes the request on c and reads the response head. gotByte
// reports whether any response byte was read (no retry after that).
func (t *Transport) exchange(ctx context.Context, c *conn, req *http.Request, r *request, w *Wire) (resp *http.Response, gotByte bool, err error) {
	trace := httptrace.ContextClientTrace(ctx)
	stop := context.AfterFunc(ctx, func() { _ = c.SetDeadline(time.Unix(1, 0)) })
	defer func() {
		if err != nil {
			stop()
		}
	}()
	if err := writeHead(c.bw, r); err != nil {
		return nil, false, err
	}
	if trace != nil && trace.WroteHeaders != nil {
		trace.WroteHeaders()
	}
	expect := r.body != nil && fieldIs(r.headers, "Expect", "100-continue")
	sendBody := true
	if expect {
		if err := c.bw.Flush(); err != nil {
			return nil, false, err
		}
		if sendBody, err = waitContinue(ctx, c); err != nil {
			return nil, false, err
		}
	}
	if sendBody {
		if err := writeBody(c.bw, r); err != nil {
			return nil, false, err
		}
	}
	err = c.bw.Flush()
	if trace != nil && trace.WroteRequest != nil {
		trace.WroteRequest(httptrace.WroteRequestInfo{Err: err})
	}
	if err != nil {
		return nil, false, err
	}
	if _, err := c.br.Peek(1); err != nil {
		return nil, false, err
	}
	if trace != nil && trace.GotFirstResponseByte != nil {
		trace.GotFirstResponseByte()
	}
	head, err := readFinalHead(c.br, r.method, trace)
	if err != nil {
		return nil, true, err
	}
	// A request whose body was not sent (refused Expect) leaves the
	// connection out of step.
	reusable := head.keepAlive && !head.anomaly && sendBody && !requestCloses(r)
	resp = &http.Response{
		Status:        head.statusText(),
		StatusCode:    head.status,
		Proto:         fmt.Sprintf("HTTP/1.%d", head.minor),
		ProtoMajor:    1,
		ProtoMinor:    head.minor,
		Header:        canonical(head.headers),
		ContentLength: -1,
		Close:         !reusable,
		Request:       req,
		TLS:           c.tls,
	}
	w.ResponseHeaders = head.headers
	w.Sent = fmt.Sprintf("HTTP/1.%d", r.minor)
	b := &body{ctx: ctx, t: t, c: c, w: w, resp: resp, reusable: reusable, stop: stop}
	switch {
	case head.framing == bodyNone, head.framing == bodyFixed && head.length == 0:
		resp.ContentLength = 0
		resp.Body = http.NoBody
		b.finish(true)
		return resp, true, nil
	case head.framing == bodyFixed:
		resp.ContentLength = head.length
		b.r = &fixedReader{r: c.br, n: head.length}
	case head.framing == bodyChunked:
		b.chunked = &chunkedReader{br: c.br, budget: maxHeadBytes}
		b.r = b.chunked
	default:
		b.r = c.br
		b.reusable = false
	}
	resp.Body = b
	return resp, true, nil
}

// readFinalHead reads the response heads up to the final one, skipping
// at most maxInformational 1xx responses (101 is final).
func readFinalHead(br *bufio.Reader, method string, trace *httptrace.ClientTrace) (*responseHead, error) {
	for i := 0; ; i++ {
		head, err := readHead(br, method)
		if err != nil {
			return nil, err
		}
		if head.status >= 200 || head.status == 101 {
			return head, nil
		}
		if i == maxInformational {
			return nil, responseErrorf("more than %d informational responses", maxInformational)
		}
		if trace != nil && trace.Got1xxResponse != nil {
			if err := trace.Got1xxResponse(head.status, textproto.MIMEHeader(canonical(head.headers))); err != nil {
				return nil, err
			}
		}
	}
}

// waitContinue waits up to a second for the answer to Expect:
// 100-continue: a 100 (or no answer) sends the body, a final response
// does not. Other 1xx responses are skipped.
func waitContinue(ctx context.Context, c *conn) (bool, error) {
	deadline := time.Now().Add(time.Second)
	for range maxInformational + 1 {
		_ = c.SetReadDeadline(deadline)
		_, err := c.br.Peek(1)
		_ = c.SetReadDeadline(time.Time{})
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		b, _ := c.br.Peek(12)
		if len(b) < 12 || !strings.HasPrefix(string(b), "HTTP/1.") || b[9] != '1' {
			return false, nil // a final response: it is read as such
		}
		if _, err := readHead(c.br, ""); err != nil {
			return false, err
		}
		if string(b[9:12]) == "100" {
			return true, nil
		}
	}
	return false, responseErrorf("more than %d informational responses", maxInformational)
}

// body reads a response body, then puts the connection back in the pool
// (or the lease) when it can carry another request, and closes it
// otherwise.
type body struct {
	ctx      context.Context
	t        *Transport
	c        *conn
	w        *Wire
	resp     *http.Response
	r        io.Reader
	chunked  *chunkedReader
	reusable bool
	stop     func() bool
	done     bool
}

func (b *body) Read(p []byte) (int, error) {
	if b.done {
		return 0, io.EOF
	}
	n, err := b.r.Read(p)
	switch {
	case errors.Is(err, io.EOF) && b.r == io.Reader(b.c.br):
		b.finish(false) // the server closed the connection to end the body
	case errors.Is(err, io.EOF):
		b.finish(true)
	case err != nil:
		b.finish(false)
		if ctxErr := b.ctx.Err(); ctxErr != nil {
			err = ctxErr // the request's deadline or cancellation, not the conn's
		}
	}
	return n, err
}

func (b *body) Close() error {
	if !b.done {
		b.finish(false)
	}
	return nil
}

// finish releases the connection: clean tells that the body was read to
// its end.
func (b *body) finish(clean bool) {
	b.done = true
	fired := !b.stop()
	if b.chunked != nil && clean {
		b.w.Trailers = b.chunked.trailers
		if len(b.chunked.trailers) > 0 {
			b.resp.Trailer = canonical(b.chunked.trailers)
		}
		if b.chunked.anomaly {
			clean = false
		}
	}
	// Bytes after the body belong to no request: never reuse.
	if !clean || !b.reusable || fired || b.c.br.Buffered() > 0 {
		_ = b.c.Close()
		return
	}
	b.c.reused = true
	if b.w.Lease != nil {
		b.w.Lease.keep(b.c)
		return
	}
	b.t.pool.put(b.c)
}

// requestCloses reports whether the request ends its connection:
// Connection: close, or HTTP/1.0 without Connection: keep-alive.
func requestCloses(r *request) bool {
	closeTok, keepAlive := false, false
	for _, f := range r.headers {
		if !strings.EqualFold(f.Name, "Connection") {
			continue
		}
		for tok := range strings.SplitSeq(f.Value, ",") {
			switch strings.ToLower(strings.TrimSpace(tok)) {
			case "close":
				closeTok = true
			case "keep-alive":
				keepAlive = true
			}
		}
	}
	return closeTok || (r.minor == 0 && !keepAlive)
}

// canonical builds net/http's header map from an ordered list.
func canonical(fields []exchange.Header) http.Header {
	h := make(http.Header, len(fields))
	for _, f := range fields {
		k := textproto.CanonicalMIMEHeaderKey(f.Name)
		h[k] = append(h[k], f.Value)
	}
	return h
}

func hasField(fields []exchange.Header, name string) bool {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return true
		}
	}
	return false
}

// fieldIs reports whether a header name has a value equal to value
// (ASCII case-insensitive).
func fieldIs(fields []exchange.Header, name, value string) bool {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) && strings.EqualFold(strings.TrimSpace(f.Value), value) {
			return true
		}
	}
	return false
}

func basicAuth(u *url.Userinfo) string {
	pw, _ := u.Password()
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(u.Username()+":"+pw))
}

// retryable reports whether a request that may have reached the server
// can be sent again: an idempotent method, or an idempotency key, as
// net/http decides.
func retryable(r *request) bool {
	switch r.method {
	case "GET", "HEAD", "OPTIONS", "TRACE", "PUT", "DELETE":
		return true
	}
	return hasField(r.headers, "Idempotency-Key") || hasField(r.headers, "X-Idempotency-Key")
}

// replayBody returns a new copy of the request body for a retry.
func replayBody(req *http.Request) (io.Reader, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("h1wire: the request body cannot be sent again")
	}
	return req.GetBody()
}

func closeBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}
