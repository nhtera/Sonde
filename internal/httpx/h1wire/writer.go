// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package h1wire

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/nhtera/sonde/exchange"
)

// request is a request as it goes on the wire.
type request struct {
	method string
	target string // origin-form, absolute-form (plain HTTP proxy) or authority-form (CONNECT)
	minor  int    // HTTP/1.minor
	// headers are written in order, names as given; exactly one is Host.
	headers []exchange.Header
	body    io.Reader // nil: no body
	// length is the body length; -1 sends the body chunked. A
	// Content-Length or Transfer-Encoding of headers frames it.
	length int64
}

// check validates what writeHead writes, so that nothing it sends reads
// back differently: a token method, a target without spaces or controls,
// token header names, values without CR, LF or other controls, and
// exactly one Host.
func (r *request) check() error {
	if !isToken(r.method) {
		return requestErrorf("invalid method %q", r.method)
	}
	if !validTarget(r.target) || !targetForm(r.method, r.target) {
		return requestErrorf("invalid request target %q", r.target)
	}
	hosts := 0
	for _, h := range r.headers {
		if !isToken(h.Name) {
			return requestErrorf("invalid header name %q", h.Name)
		}
		if !validFieldValue(h.Value) {
			return requestErrorf("invalid value for header %s", h.Name)
		}
		if strings.EqualFold(h.Name, "Host") {
			hosts++
		}
	}
	if hosts != 1 {
		return requestErrorf("a request needs exactly one Host header, not %d", hosts)
	}
	return nil
}

// writeHead writes the request line and the headers, then the blank line.
func writeHead(w *bufio.Writer, r *request) error {
	if err := r.check(); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s %s HTTP/1.%d\r\n", r.method, r.target, r.minor)
	for _, h := range r.headers {
		if h.Value == "" {
			fmt.Fprintf(w, "%s:\r\n", h.Name)
		} else {
			fmt.Fprintf(w, "%s: %s\r\n", h.Name, h.Value)
		}
	}
	_, err := w.WriteString("\r\n")
	return err
}

// writeBody writes the body: as is for a known length, else in chunks.
func writeBody(w *bufio.Writer, r *request) error {
	if r.body == nil {
		return nil
	}
	if r.length >= 0 {
		n, err := io.Copy(w, r.body)
		if err == nil && n != r.length {
			err = fmt.Errorf("request body: %d bytes written, Content-Length is %d", n, r.length)
		}
		return err
	}
	cw := &chunkWriter{w: w}
	if _, err := io.Copy(cw, r.body); err != nil {
		return err
	}
	_, err := w.WriteString("0\r\n\r\n")
	return err
}

// chunkWriter writes each Write as one chunk.
type chunkWriter struct{ w *bufio.Writer }

func (c *chunkWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	fmt.Fprintf(c.w, "%x\r\n", len(p))
	_, _ = c.w.Write(p) // bufio.Writer keeps the first error, returned by the next write
	_, err := c.w.WriteString("\r\n")
	return len(p), err
}

// targetForm reports whether target has one of the forms of RFC 9112:
// origin-form (/path), absolute-form (scheme://...), asterisk-form for
// OPTIONS, or authority-form for CONNECT.
func targetForm(method, target string) bool {
	switch {
	case target[0] == '/':
		return true
	case method == "CONNECT":
		return !strings.ContainsAny(target, "/?#")
	case target == "*":
		return method == "OPTIONS"
	}
	scheme, _, ok := strings.Cut(target, "://")
	return ok && isToken(scheme)
}
