// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sync"

	"github.com/nhtera/sonde/internal/httpx/h1wire"
)

// legacyWire reports whether SONDE_HTTP1_WIRE=legacy sends HTTP/1.x
// through net/http, as before the own wire layer (kept for one minor
// release as a fallback).
func legacyWire() bool { return os.Getenv("SONDE_HTTP1_WIRE") == "legacy" }

// dispatcher sends HTTP/1.x requests with h1wire and the rest with
// net/http: HTTP/2 (negotiated with ALPN, or with prior knowledge),
// HTTPS through a proxy unless a 1.x version is forced, and proxies other
// than http://. An https:// request without a forced version first goes
// to h1wire, which offers h2 and http/1.1: a server choosing h2 has its
// connection handed to net/http, and its address is remembered.
type dispatcher struct {
	std    *http.Transport
	h1     *h1wire.Transport
	forced bool // a 1.x version is forced: no h2 over TLS
	proxy  func(*http.Request) (*url.URL, error)

	mu      sync.Mutex
	h2Addrs map[string]bool
	handoff map[string][]*tls.Conn
}

func (d *dispatcher) RoundTrip(req *http.Request) (*http.Response, error) {
	if !d.wire(req) {
		return d.std.RoundTrip(req)
	}
	resp, err := d.h1.RoundTrip(req)
	var h2 *h1wire.H2Conn
	if errors.As(err, &h2) {
		d.mu.Lock()
		d.h2Addrs[h2.Addr] = true
		d.handoff[h2.Addr] = append(d.handoff[h2.Addr], h2.Conn)
		d.mu.Unlock()
		resp, err = d.std.RoundTrip(req)
		// net/http may have used another connection (an idle one, or a
		// concurrent dial): the handed-over one is not kept.
		d.mu.Lock()
		list := d.handoff[h2.Addr]
		if i := slices.Index(list, h2.Conn); i >= 0 {
			d.handoff[h2.Addr] = slices.Delete(list, i, i+1)
			_ = h2.Conn.Close()
		}
		d.mu.Unlock()
	}
	return resp, err
}

// wire reports whether req goes through h1wire.
func (d *dispatcher) wire(req *http.Request) bool {
	if d.proxy != nil {
		p, err := d.proxy(req)
		if err != nil {
			return false // net/http reports it
		}
		if p != nil && (p.Scheme != "http" || req.URL.Scheme == "https" && !d.forced) {
			return false
		}
	}
	if req.URL.Scheme == "https" && !d.forced {
		d.mu.Lock()
		defer d.mu.Unlock()
		return !d.h2Addrs[canonicalAddr(req.URL)]
	}
	return true
}

// dialTLS is net/http's DialTLSContext: a connection h1wire handed over,
// else a new one, with the TLS hooks fired as h1wire fires them.
func (d *dispatcher) dialTLS(dial func(ctx context.Context, network, addr string) (net.Conn, error), cfg *tls.Config) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		d.mu.Lock()
		if list := d.handoff[addr]; len(list) > 0 {
			c := list[len(list)-1]
			d.handoff[addr] = list[:len(list)-1]
			d.mu.Unlock()
			return c, nil
		}
		d.mu.Unlock()
		raw, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		c := cfg.Clone()
		if c.ServerName == "" {
			c.ServerName, _, _ = net.SplitHostPort(addr)
		}
		c.NextProtos = []string{"h2", "http/1.1"}
		return h1wire.HandshakeTLS(ctx, raw, c)
	}
}

// CloseIdleConnections closes the idle connections of both transports and
// any connection handed over but not used.
func (d *dispatcher) CloseIdleConnections() {
	d.std.CloseIdleConnections()
	d.h1.CloseIdleConnections()
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, list := range d.handoff {
		for _, c := range list {
			_ = c.Close()
		}
	}
	d.handoff = map[string][]*tls.Conn{}
}

// canonicalAddr is host:port of u, with the scheme's default port and an
// internationalized host name in ASCII, as net/http keys its dials.
func canonicalAddr(u *url.URL) string {
	return net.JoinHostPort(asciiHost(u.Hostname()), effectivePort(u))
}

// asciiHost is host with an internationalized name in its ASCII
// (punycode) form, as it goes on the wire.
func asciiHost(host string) string { return h1wire.ASCIIHost(host) }
