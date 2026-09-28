// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/codec"
)

// Execute sends a request, following redirects when opts ask for it, and
// returns every call in order (the last one is the final response). A
// transport failure returns an *Error; calls completed before it are
// returned too.
func (c *Client) Execute(ctx context.Context, spec *RequestSpec, opts *Options) ([]Call, error) {
	if err := checkSupported(opts); err != nil {
		return nil, err
	}

	originalURL, err := buildURL(spec, opts.PathAsIs)
	if err != nil {
		return nil, err
	}
	if err := c.allowURL(originalURL); err != nil {
		return nil, err
	}

	var calls []Call
	curSpec := spec
	curOpts := *opts
	redirectCount := 0
	maxRedirects := opts.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = 50
	}

	for {
		call, err := c.executeOne(ctx, curSpec, &curOpts)
		if err != nil {
			if call.Response != nil { // a stream that failed after its headers
				calls = append(calls, call)
			}
			return calls, err
		}
		calls = append(calls, call)

		status := call.Response.Status
		loc, _ := call.Response.Headers.Get("Location")
		if !followsRedirect(opts, status, loc) {
			return calls, nil
		}
		curURL, err := url.Parse(call.Response.URL)
		if err != nil {
			return calls, invalidURLError(call.Response.URL, err.Error())
		}
		redirectURL, err := curURL.Parse(loc)
		if err != nil {
			return calls, invalidURLError(loc, "could not resolve redirect Location")
		}

		redirectCount++
		if maxRedirects != -1 && redirectCount > maxRedirects {
			return calls, tooManyRedirectsError()
		}
		if err := c.allowURL(redirectURL); err != nil {
			return calls, err
		}

		newMethod := redirectMethod(status, curSpec.Method)
		stripCreds := shouldStripCredentials(originalURL, redirectURL, opts.LocationTrusted)
		// Explicit Authorization and Cookie headers stay with the original
		// host; the entry's cookies follow the redirect, as curl's do.
		headers := curSpec.Headers
		newOpts := curOpts
		if stripCreds {
			headers = filterHeaders(headers, "Authorization", "Cookie")
			newOpts.Headers = filterHeaders(curOpts.Headers, "Authorization", "Cookie")
			newOpts.User = ""
		}

		next := *curSpec
		next.URL = redirectURL.String()
		next.Method = newMethod
		next.Headers = headers
		next.Query = nil
		if newMethod != curSpec.Method {
			next.Form = nil
			next.Multipart = nil
			next.Body = Body{}
			next.ImplicitContentType = ""
		}
		curSpec = &next
		curOpts = newOpts
	}
}

// checkSupported rejects the options sonde does not implement yet.
func checkSupported(opts *Options) error {
	switch {
	case opts.AWSSigV4 != "":
		return unsupportedError("aws-sigv4")
	case opts.Digest:
		return unsupportedError("digest")
	case opts.NTLM:
		return unsupportedError("ntlm")
	case opts.Negotiate:
		return unsupportedError("negotiate")
	case opts.HTTPVersion == HTTP10:
		return unsupportedError("http1.0")
	case opts.HTTPVersion == HTTP3:
		return newError(ErrUnsupported, "Unsupported HTTP version", "HTTP/3 is not supported, check --version", nil)
	case opts.GRPC && opts.HTTPVersion == HTTP11:
		return newError(ErrUnsupported, "Unsupported HTTP version", "a gRPC call uses HTTP/2: the http1.1 option does not apply", nil)
	}
	return nil
}

// redirectMethod applies curl's redirect method change: 301-303 always
// become GET, every other redirected status keeps the original method.
func redirectMethod(status int, method string) string {
	if status >= 301 && status <= 303 {
		return "GET"
	}
	return method
}

// shouldStripCredentials reports whether Authorization/Cookie must be
// dropped across a redirect: curl only forwards them to the original
// scheme+host+port, unless --location-trusted is set.
func shouldStripCredentials(original, redirect *url.URL, locationTrusted bool) bool {
	if locationTrusted {
		return false
	}
	return original.Scheme != redirect.Scheme ||
		!strings.EqualFold(original.Hostname(), redirect.Hostname()) ||
		effectivePort(original) != effectivePort(redirect)
}

// allowURL checks the host of a request or redirect URL against
// ClientConfig.Hosts. The connection is checked again when it is dialed
// (dialOptions.dialContext): a proxy or a connect-to rule can send the
// bytes to another host than the one the URL names.
func (c *Client) allowURL(u *url.URL) error {
	if c.cfg.Hosts == nil {
		return nil
	}
	if err := c.cfg.Hosts.Allow(u.Hostname(), effectivePort(u)); err != nil {
		return hostDeniedError(err)
	}
	return nil
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func filterHeaders(headers []exchange.Header, drop ...string) []exchange.Header {
	out := make([]exchange.Header, 0, len(headers))
	for _, h := range headers {
		skip := false
		for _, d := range drop {
			if strings.EqualFold(h.Name, d) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, h)
		}
	}
	return out
}

// executeOne performs exactly one HTTP exchange: build the request, run
// it through the cached transport for opts, and turn the result into a
// Call.
func (c *Client) executeOne(ctx context.Context, spec *RequestSpec, opts *Options) (Call, error) {
	now := time.Now()
	prep, err := buildRequest(ctx, spec, opts, c.cfg, requestContext{client: c, now: now})
	if err != nil {
		return Call{}, err
	}
	built, err := c.transportFor(opts, prep.req.URL.Hostname())
	if err != nil {
		return Call{}, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stopStream := func() {}
	if opts.ReadStream != nil {
		reqCtx, stopStream = context.WithCancel(reqCtx)
		defer stopStream()
	}

	start := time.Now()
	timings := &hopTimings{start: start}
	// Hooks may fire from a dial goroutine that outlives the request; do
	// serializes them and ignores those after RoundTrip returned.
	do := timings.do
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { do(func() { timings.dnsStart = time.Now() }) },
		DNSDone:  func(httptrace.DNSDoneInfo) { do(func() { timings.mark(&timings.nameLookup) }) },
		ConnectStart: func(string, string) {
			do(func() {
				if timings.connectStart.IsZero() {
					timings.connectStart = time.Now()
				}
			})
		},
		ConnectDone:       func(string, string, error) { do(func() { timings.mark(&timings.connect) }) },
		TLSHandshakeStart: func() { do(func() { timings.tlsStart = time.Now() }) },
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			do(func() {
				timings.mark(&timings.appConnect)
				if err == nil && opts.Verbose && c.cfg.Debug != nil {
					c.cfg.Debug(fmt.Sprintf("TLS connection using %s / %s", tlsVersionName(state.Version), tls.CipherSuiteName(state.CipherSuite)))
				}
			})
		},
		GotConn: func(info httptrace.GotConnInfo) {
			do(func() {
				timings.mark(&timings.preTransfer)
				if info.Conn != nil {
					timings.remoteIP = remoteIP(info.Conn)
					if opts.Verbose && c.cfg.Debug != nil {
						host, port, _ := net.SplitHostPort(info.Conn.RemoteAddr().String())
						c.cfg.Debug(fmt.Sprintf("Connected to %s (%s) port %s", prep.req.URL.Hostname(), host, port))
					}
				}
			})
		},
		GotFirstResponseByte: func() { do(func() { timings.mark(&timings.startTransfer) }) },
	}
	reqCtx = httptrace.WithClientTrace(reqCtx, trace)
	prep.req = prep.req.WithContext(reqCtx)

	if opts.MaxSendSpeed > 0 && len(prep.body) > 0 {
		prep.req.Body = io.NopCloser(newRateLimitedReader(reqCtx, prep.req.Body, opts.MaxSendSpeed))
	}

	resp, err := built.rt.RoundTrip(prep.req)
	timings.stop()
	if err != nil {
		return Call{}, classifyRoundTripError(prep.req.URL, err)
	}
	defer resp.Body.Close()

	bodyReader := io.Reader(resp.Body)
	if opts.MaxRecvSpeed > 0 {
		bodyReader = newRateLimitedReader(reqCtx, bodyReader, opts.MaxRecvSpeed)
	}
	var respBody []byte
	var streamErr error // a streamed body that failed: the call is still returned
	if opts.ReadStream != nil && !followsRedirect(opts, resp.StatusCode, resp.Header.Get("Location")) {
		respBody, streamErr = readStream(resp, bodyReader, opts.ReadStream, stopStream)
		if streamErr != nil {
			streamErr = classifyRoundTripError(prep.req.URL, streamErr)
		}
	} else {
		limit := opts.MaxFilesize
		if limit <= 0 {
			limit = exchange.DefaultMaxDecodedBody
		}
		respBody, err = io.ReadAll(io.LimitReader(bodyReader, limit+1))
		if err == nil && int64(len(respBody)) > limit {
			return Call{}, maxFilesizeError()
		}
	}
	if err != nil {
		return Call{}, classifyRoundTripError(prep.req.URL, err)
	}
	timings.end = time.Now()

	version := normalizeVersion(resp.Proto)
	var cert *exchange.CertInfo
	if resp.TLS != nil {
		cert = certInfo(*resp.TLS)
	}

	headers := responseHeaders(resp.Header)
	if opts.GRPC { // the status of a gRPC call is in its trailers
		headers = append(headers, responseHeaders(resp.Trailer)...)
	}
	response := &exchange.Response{
		Version:        version,
		Status:         resp.StatusCode,
		Reason:         reasonPhrase(resp.Status, resp.ProtoMajor),
		Headers:        headers,
		Body:           respBody,
		URL:            prep.req.URL.String(),
		IP:             timings.remoteIP,
		Duration:       timings.end.Sub(start),
		Certificate:    cert,
		MaxDecodedBody: opts.MaxFilesize,
		Timings: exchange.Timings{
			Begin:         start,
			End:           timings.end,
			NameLookup:    timings.nameLookup,
			Connect:       timings.connect,
			AppConnect:    timings.appConnect,
			PreTransfer:   timings.preTransfer,
			StartTransfer: timings.startTransfer,
			Total:         timings.end.Sub(start),
		},
	}

	c.jar.updateFromResponse(prep.req.URL, response, now)

	return Call{
		Request: exchange.Request{
			Method:  prep.req.Method,
			URL:     prep.req.URL.String(),
			Headers: prep.headers,
			Body:    prep.body,
		},
		Response: response,
		Timings:  response.Timings,
	}, streamErr
}

// followsRedirect reports whether Execute follows a response with status
// and Location header loc to another URL.
func followsRedirect(opts *Options, status int, loc string) bool {
	return opts.FollowLocation && !opts.GRPC && status >= 300 && status < 400 && loc != ""
}

// readStream runs read on the decoded body of a streamed response and
// returns the bytes as received.
func readStream(resp *http.Response, body io.Reader, read func(exchange.Headers, io.Reader, func()) error, stop func()) ([]byte, error) {
	var raw bytes.Buffer
	header := responseHeaders(resp.Header)
	d := &lazyDecoder{
		codings: (&exchange.Response{Headers: header}).ContentEncodings(),
		r:       io.TeeReader(body, &raw),
	}
	defer d.close()
	err := read(header, d, stop)
	return raw.Bytes(), err
}

// lazyDecoder removes content codings from r, starting at the first Read:
// a decoder reads its header at once, which must not block before the
// stream's own limits apply.
type lazyDecoder struct {
	codings []string
	r       io.Reader
	started bool
	release func()
	err     error
}

func (d *lazyDecoder) Read(p []byte) (int, error) {
	if !d.started {
		d.started = true
		var err error
		if d.r, d.release, err = codec.Chain(d.codings, d.r); err != nil {
			d.err = newError(ErrOther, "Decompression error",
				"could not uncompress the event stream ("+strings.Join(d.codings, ", ")+")", err)
		}
	}
	if d.err != nil {
		return 0, d.err
	}
	return d.r.Read(p)
}

func (d *lazyDecoder) close() {
	if d.release != nil {
		d.release()
	}
}

// hopTimings accumulates the httptrace timestamps of one call.
type hopTimings struct {
	start                            time.Time
	dnsStart, connectStart, tlsStart time.Time
	nameLookup, connect, appConnect  time.Duration
	preTransfer, startTransfer       time.Duration
	end                              time.Time
	remoteIP                         string

	mu      sync.Mutex
	stopped bool
}

// do runs f unless the exchange is over.
func (t *hopTimings) do(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.stopped {
		f()
	}
}

// stop ends the recording of connection events.
func (t *hopTimings) stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

func (t *hopTimings) mark(d *time.Duration) {
	if *d == 0 {
		*d = time.Since(t.start)
	}
}

// remoteIP extracts the bare IP address a connection was made to.
func remoteIP(conn net.Conn) string {
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return conn.RemoteAddr().String()
	}
	return host
}

// normalizeVersion maps Go's response proto string to the four values
// exchange.Response.Version accepts.
func normalizeVersion(proto string) string {
	switch {
	case strings.HasPrefix(proto, "HTTP/1.0"):
		return "HTTP/1.0"
	case strings.HasPrefix(proto, "HTTP/2"):
		return "HTTP/2"
	case strings.HasPrefix(proto, "HTTP/3"):
		return "HTTP/3"
	default:
		return "HTTP/1.1"
	}
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLSv1.3"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS11:
		return "TLSv1.1"
	case tls.VersionTLS10:
		return "TLSv1.0"
	default:
		return "TLS"
	}
}

// responseHeaders rebuilds an ordered header list from Go's http.Header
// map. net/http does not preserve the wire order across different header
// names (it is a map), so this is a best-effort, deterministic
// reconstruction: names sorted alphabetically, values of a repeated name
// kept in the order they were received. Callers that need the exact wire
// order (e.g. Set-Cookie sequencing within a name) still get it.
func responseHeaders(h map[string][]string) exchange.Headers {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	slices.Sort(names)
	var out exchange.Headers
	for _, name := range names {
		for _, v := range h[name] {
			out = append(out, exchange.Header{Name: name, Value: v})
		}
	}
	return out
}

// classifyRoundTripError turns a RoundTrip failure into the matching
// *Error kind (our own dial errors already are one).
func classifyRoundTripError(u *url.URL, err error) error {
	var httpErr *Error
	if errors.As(err, &httpErr) {
		return httpErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return timeoutError("the configured timeout", err)
	}
	if errors.Is(err, context.Canceled) {
		return newError(ErrOther, "Canceled", "the request was canceled", err)
	}
	if isTLSError(err) {
		return tlsError(err)
	}
	host := ""
	if u != nil {
		host = u.Hostname()
	}
	port := 0
	if u != nil {
		port, _ = strconv.Atoi(effectivePort(u))
	}
	return connectError(host, port, err)
}

func isTLSError(err error) bool {
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return true
	}
	return isCertVerifyError(err)
}

// reasonPhrase returns the reason phrase of an HTTP/1.x status line.
func reasonPhrase(status string, protoMajor int) string {
	if protoMajor >= 2 {
		return ""
	}
	_, reason, _ := strings.Cut(status, " ")
	if !utf8.ValidString(reason) { // bytes read as ISO-8859-1
		r := make([]rune, len(reason))
		for i := 0; i < len(reason); i++ {
			r[i] = rune(reason[i])
		}
		return string(r)
	}
	return reason
}
