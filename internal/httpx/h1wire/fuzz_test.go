// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package h1wire

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// fuzzConn serves fixed bytes, then EOF, and swallows writes.
type fuzzConn struct {
	net.Conn
	r *bytes.Reader
}

func (f *fuzzConn) Read(p []byte) (int, error)       { return f.r.Read(p) }
func (f *fuzzConn) Write(p []byte) (int, error)      { return len(p), nil }
func (f *fuzzConn) Close() error                     { return nil }
func (f *fuzzConn) SetDeadline(time.Time) error      { return nil }
func (f *fuzzConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fuzzConn) SetWriteDeadline(time.Time) error { return nil }
func (f *fuzzConn) RemoteAddr() net.Addr             { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

// FuzzReadResponse reads arbitrary bytes as a response: it never panics,
// reads a bounded amount, and pools the connection only for a response
// with trusted framing and no anomaly.
func FuzzReadResponse(f *testing.F) {
	for _, s := range []string{
		"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok",
		"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nok\r\n0\r\nT: 1\r\n\r\n",
		"HTTP/1.0 200 OK\r\n\r\nto the end",
		"HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 204 No Content\r\n\r\n",
		"HTTP/1.1 200 OK\r\nbad name: 1\r\nX: a\r\n folded\r\nContent-Length: 0\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		tr := &Transport{Dial: func(context.Context, string, string) (net.Conn, error) {
			return &fuzzConn{r: bytes.NewReader(data)}, nil
		}}
		req, _ := http.NewRequest(http.MethodGet, "http://fuzz.test/", nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		pooled := len(tr.pool.idle["http|fuzz.test:80"]) > 0
		if !pooled {
			return
		}
		h, err := readFinalHead(bufio.NewReader(bytes.NewReader(data)), "GET", nil)
		if err != nil || h.anomaly || !h.keepAlive || h.framing == bodyToClose {
			t.Fatalf("connection pooled after %q (head %+v, %v)", data, h, err)
		}
	})
}

// FuzzWriteRequest writes a request from arbitrary parts: when it is
// written at all, it reads back as exactly one request with the same
// method, target, headers and body.
func FuzzWriteRequest(f *testing.F) {
	f.Add("GET", "/p?q=1", "X-Name", "value", "")
	f.Add("POST", "/", "Content-Type", "a\r\nInjected: 1", "body")
	f.Add("PUT", "/x y", "X", "v", "b")
	f.Fuzz(func(t *testing.T, method, target, name, value, body string) {
		r := &request{method: method, target: target, minor: 1, headers: []exchange.Header{
			{Name: "Host", Value: "h"}, {Name: name, Value: value},
		}}
		if body != "" {
			r.body = strings.NewReader(body)
			r.length = int64(len(body))
			r.headers = append(r.headers, exchange.Header{Name: "Content-Length", Value: strconv.Itoa(len(body))})
		}
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		if err := writeHead(w, r); err != nil {
			return
		}
		if _, err := url.ParseRequestURI(target); err != nil && target != "*" {
			return // URL escaping is not framing: net/http refuses some targets a server reads
		}
		if err := writeBody(w, r); err != nil {
			t.Fatal(err)
		}
		_ = w.Flush()
		br := bufio.NewReader(&buf)
		got, err := http.ReadRequest(br)
		if err != nil {
			// net/http is stricter than RFC 9110 for a few names and
			// values it treats specially; those are not ours to check.
			if strings.EqualFold(name, "Host") || strings.EqualFold(name, "Content-Length") || strings.EqualFold(name, "Transfer-Encoding") {
				return
			}
			t.Fatalf("written request does not parse: %v\n%q", err, buf.String())
		}
		if got.Method != method || got.RequestURI != target {
			t.Fatalf("read %s %s, wrote %s %s", got.Method, got.RequestURI, method, target)
		}
		if !strings.EqualFold(name, "Host") && !strings.EqualFold(name, "Content-Length") && got.Header.Get(name) != strings.Trim(value, " \t") {
			t.Fatalf("header %q = %q, wrote %q", name, got.Header.Get(name), value)
		}
		b, _ := io.ReadAll(got.Body)
		if string(b) != body {
			t.Fatalf("body %q, wrote %q", b, body)
		}
		if rest, _ := io.ReadAll(br); len(rest) != 0 {
			t.Fatalf("%d bytes after the request", len(rest))
		}
	})
}
