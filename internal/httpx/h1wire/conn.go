// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package h1wire

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// Wire carries what net/http's types cannot hold between the caller and
// RoundTrip: the request headers in order and case, the version, and the
// response headers and trailers as received. It is passed in the
// request's context (WithWire).
type Wire struct {
	// HTTP10 writes an HTTP/1.0 request line.
	HTTP10 bool
	// RequestHeaders, when set, are written as they are, in order: they
	// must hold exactly one Host and the body's Content-Length. Nil: the
	// request's Host and Header (sorted) and Content-Length.
	RequestHeaders []exchange.Header
	// ResponseHeaders and Trailers are set by RoundTrip, in wire order
	// with the names as received.
	ResponseHeaders []exchange.Header
	Trailers        []exchange.Header
	// Lease, when set, keeps the connection for the next request of the
	// same lease instead of the shared pool (see Lease).
	Lease *Lease
}

type wireKey struct{}

// WithWire returns ctx carrying w for RoundTrip.
func WithWire(ctx context.Context, w *Wire) context.Context {
	return context.WithValue(ctx, wireKey{}, w)
}

func wireFrom(ctx context.Context) *Wire {
	w, _ := ctx.Value(wireKey{}).(*Wire)
	return w
}

// conn is one connection to a server (or through a proxy tunnel).
type conn struct {
	net.Conn
	br  *bufio.Reader
	bw  *bufio.Writer
	key string
	tls *tls.ConnectionState
	// reused tells that the connection has carried a request before.
	reused    bool
	idleSince time.Time
	// watch receives the result of the read that watches an idle
	// connection (nil when it is not idle).
	watch chan error
}

// startWatch watches an idle connection: a read that returns means the
// server closed it or sent something no request asked for.
func (c *conn) startWatch() {
	ch := make(chan error, 1)
	c.watch = ch
	go func() {
		_, err := c.br.Peek(1)
		ch <- err
	}()
}

// stopWatch ends the watch of an idle connection and reports whether it
// can carry a request: the watching read was still waiting.
func (c *conn) stopWatch() bool {
	if c.watch == nil {
		return c.br.Buffered() == 0
	}
	_ = c.SetReadDeadline(time.Unix(1, 0)) // wakes the read up
	err := <-c.watch
	c.watch = nil
	_ = c.SetReadDeadline(time.Time{})
	var ne net.Error
	return c.br.Buffered() == 0 && errors.As(err, &ne) && ne.Timeout()
}

func newConn(c net.Conn, key string) *conn {
	return &conn{Conn: c, br: bufio.NewReaderSize(c, 32<<10), bw: bufio.NewWriterSize(c, 32<<10), key: key}
}

// pool keeps idle connections by key, the most recent first.
type pool struct {
	mu   sync.Mutex
	idle map[string][]*conn
}

// maxIdlePerKey bounds the idle connections kept for one key.
const maxIdlePerKey = 8

// get returns an idle connection for key that is still open and has
// nothing unread, closing those that are not.
func (p *pool) get(key string) *conn {
	for {
		p.mu.Lock()
		list := p.idle[key]
		if len(list) == 0 {
			p.mu.Unlock()
			return nil
		}
		c := list[len(list)-1]
		p.idle[key] = list[:len(list)-1]
		p.mu.Unlock()
		// The server must have neither closed it nor sent anything since
		// (a late response, a 408 before closing): that would be read as
		// the answer to the next request.
		if c.stopWatch() {
			return c
		}
		_ = c.Close()
	}
}

func (p *pool) put(c *conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.idle == nil {
		p.idle = map[string][]*conn{}
	}
	if len(p.idle[c.key]) >= maxIdlePerKey {
		_ = c.Close()
		return
	}
	c.idleSince = time.Now()
	c.startWatch()
	p.idle[c.key] = append(p.idle[c.key], c)
}

func (p *pool) closeIdle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, list := range p.idle {
		for _, c := range list {
			_ = c.Close()
		}
	}
	p.idle = nil
}

// Lease holds one connection across the requests of an exchange that is
// bound to it (a connection-based authentication handshake): requests
// made with the lease in their Wire use and keep its connection, which
// never goes to the shared pool. Close releases it.
type Lease struct {
	mu sync.Mutex
	c  *conn
}

func (l *Lease) take(key string) *conn {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.c == nil || l.c.key != key {
		return nil
	}
	c := l.c
	l.c = nil
	if !c.stopWatch() {
		_ = c.Close()
		return nil
	}
	return c
}

func (l *Lease) keep(c *conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.c != nil {
		_ = l.c.Close()
	}
	c.startWatch()
	l.c = c
}

// Close closes the leased connection, if any.
func (l *Lease) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.c == nil {
		return nil
	}
	err := l.c.Close()
	l.c = nil
	return err
}
