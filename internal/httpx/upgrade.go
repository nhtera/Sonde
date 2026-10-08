// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// Upgrade is a prepared WebSocket opening handshake (RFC 6455): the entry's
// request and a client to send it with. The WebSocket library adds its own
// handshake headers to Header and sends the request with Client.
type Upgrade struct {
	// URL is the request URL, with ws and wss mapped to http and https.
	URL string
	// Host overrides the Host header when the entry sets one.
	Host string
	// Header holds the entry's headers, cookies, credentials and
	// User-Agent, as Execute would send them.
	Header http.Header
	// Client sends the handshake through the entry's transport options
	// (TLS, proxy, resolve), always with HTTP/1.1, without following
	// redirects.
	Client *http.Client
	// Request is the request as reported in results; Response adds the
	// handshake headers the library sent.
	Request exchange.Request

	client *Client
	opts   *Options
	url    *url.URL
	start  time.Time
	// refused is the whole body of a refused upgrade, read before the
	// WebSocket library cuts it to its first bytes.
	refused []byte
}

// upgradeTransport keeps the whole body of a response that refuses the
// upgrade.
type upgradeTransport struct {
	rt http.RoundTripper
	u  *Upgrade
}

func (t *upgradeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.u.opts.OnSend != nil {
		sent := t.u.Request
		sent.Headers = append(slices.Clip(sent.Headers), libraryHeaders(req.Header)...)
		t.u.opts.OnSend(sent)
	}
	resp, err := t.rt.RoundTrip(req)
	if err != nil || resp.StatusCode == http.StatusSwitchingProtocols {
		return resp, err
	}
	limit := t.u.opts.MaxFilesize
	if limit <= 0 {
		limit = exchange.DefaultMaxDecodedBody
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	t.u.refused = body
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// Upgrade prepares the WebSocket handshake of spec. The http2,
// http2-prior-knowledge and http3 options are ignored, with a warning: an
// upgrade needs HTTP/1.1.
func (c *Client) Upgrade(ctx context.Context, spec *RequestSpec, opts *Options) (*Upgrade, error) {
	o := *opts
	switch o.HTTPVersion {
	case HTTP2, HTTP2PriorKnowledge, HTTP3:
		c.warnOnce("the WebSocket handshake uses HTTP/1.1: the http2, http2-prior-knowledge and http3 options are ignored")
	}
	o.HTTPVersion = HTTP11
	if err := checkSupported(&o); err != nil {
		return nil, err
	}
	if o.Digest || o.NTLM || o.Negotiate {
		return nil, newError(ErrUnsupported, "Unsupported option", "digest, ntlm and negotiate are not supported for the WebSocket handshake", nil)
	}
	s := *spec
	s.URL = httpScheme(spec.URL)
	now := time.Now()
	prep, err := buildRequest(ctx, &s, &o, c.cfg, requestContext{client: c, now: now})
	if err != nil {
		return nil, err
	}
	if err := c.allowURL(prep.req.URL); err != nil {
		return nil, err
	}
	built, err := c.transportFor(&o, prep.req.URL.Hostname())
	if err != nil {
		return nil, err
	}
	u := &Upgrade{
		URL:     prep.req.URL.String(),
		Host:    prep.req.Host,
		Header:  prep.req.Header,
		Request: exchange.Request{Method: http.MethodGet, URL: prep.req.URL.String(), Headers: prep.headers},
		client:  c,
		opts:    &o,
		url:     prep.req.URL,
		start:   now,
	}
	u.Client = &http.Client{
		Transport:     &upgradeTransport{rt: built.std, u: u},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return u, nil
}

// Response converts the handshake response and stores its cookies. Its
// body is empty for an accepted upgrade, the whole body otherwise. The
// response has no IP address and no phase timings.
func (u *Upgrade) Response(resp *http.Response) *exchange.Response {
	end := time.Now()
	var cert *exchange.CertInfo
	if resp.TLS != nil {
		cert = certInfo(*resp.TLS)
	}
	r := &exchange.Response{
		Version:        normalizeVersion(resp.Proto),
		Status:         resp.StatusCode,
		Reason:         reasonPhrase(resp.Status, resp.ProtoMajor),
		Headers:        responseHeaders(resp.Header),
		Body:           u.refused,
		URL:            u.URL,
		Duration:       end.Sub(u.start),
		Certificate:    cert,
		MaxDecodedBody: u.opts.MaxFilesize,
		Timings:        exchange.Timings{Begin: u.start, End: end, Total: end.Sub(u.start)},
	}
	if resp.Request != nil {
		u.Request.Headers = append(u.Request.Headers, libraryHeaders(resp.Request.Header)...)
	}
	u.client.jar.updateFromResponse(u.url, r, u.start)
	return r
}

// libraryHeaders returns the handshake headers the WebSocket library
// added to h.
func libraryHeaders(h http.Header) []exchange.Header {
	var out []exchange.Header
	for _, name := range handshakeHeaders {
		for _, v := range h.Values(name) {
			out = append(out, exchange.Header{Name: name, Value: v})
		}
	}
	return out
}

// handshakeHeaders are the headers the WebSocket library adds.
var handshakeHeaders = []string{"Connection", "Upgrade", "Sec-WebSocket-Version", "Sec-WebSocket-Key",
	"Sec-WebSocket-Extensions"}

// MaxTime is the entry's max-time: the limit of the whole exchange.
func (u *Upgrade) MaxTime() time.Duration {
	if u.opts.Timeout <= 0 {
		return 300 * time.Second
	}
	return u.opts.Timeout
}

// Error classifies an error of a handshake that got no response, like a
// failed Execute.
func (u *Upgrade) Error(err error) error {
	return classifyRoundTripError(u.url, err)
}

// httpScheme maps a ws:// or wss:// URL to http:// or https://.
func httpScheme(u string) string {
	switch {
	case hasSchemePrefix(u, "ws://"):
		return "http://" + u[len("ws://"):]
	case hasSchemePrefix(u, "wss://"):
		return "https://" + u[len("wss://"):]
	}
	return u
}

func hasSchemePrefix(u, prefix string) bool {
	return len(u) >= len(prefix) && strings.EqualFold(u[:len(prefix)], prefix)
}
