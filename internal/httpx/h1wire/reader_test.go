// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package h1wire

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

func head(t *testing.T, raw, method string) (*responseHead, error) {
	t.Helper()
	return readHead(bufio.NewReader(strings.NewReader(raw)), method)
}

func TestReadHeadFraming(t *testing.T) {
	tests := []struct {
		name, raw, method string
		framing           int
		length            int64
		keepAlive         bool
		anomaly           bool
	}{
		{"content-length", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n", "GET", bodyFixed, 5, true, false},
		{"lf only", "HTTP/1.1 200 OK\nContent-Length: 5\n\n", "GET", bodyFixed, 5, true, false},
		{"chunked", "HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip, Chunked\r\n\r\n", "GET", bodyChunked, 0, true, false},
		{"chunked wins over content-length, closes", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n", "GET", bodyChunked, 0, false, false},
		{"other coding reads to close", "HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip\r\n\r\n", "GET", bodyToClose, 0, false, false},
		{"no length reads to close", "HTTP/1.1 200 OK\r\n\r\n", "GET", bodyToClose, 0, false, false},
		{"head", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n", "HEAD", bodyNone, 0, true, false},
		{"204", "HTTP/1.1 204 No Content\r\n\r\n", "GET", bodyNone, 0, true, false},
		{"304", "HTTP/1.1 304 Not Modified\r\nContent-Length: 5\r\n\r\n", "GET", bodyNone, 0, true, false},
		{"connect tunnel", "HTTP/1.1 200 Connection established\r\n\r\n", "CONNECT", bodyNone, 0, true, false},
		{"connection close", "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", "GET", bodyFixed, 0, false, false},
		{"http/1.0", "HTTP/1.0 200 OK\r\nContent-Length: 0\r\n\r\n", "GET", bodyFixed, 0, false, false},
		{"http/1.0 keep-alive", "HTTP/1.0 200 OK\r\nContent-Length: 0\r\nConnection: Keep-Alive\r\n\r\n", "GET", bodyFixed, 0, true, false},
		{"lenient name", "HTTP/1.1 200 OK\r\n<script>alert('hello')</script>: foo\r\nContent-Length: 0\r\n\r\n", "GET", bodyFixed, 0, true, true},
		{"folded line", "HTTP/1.1 200 OK\r\nX: a\r\n b\r\nContent-Length: 0\r\n\r\n", "GET", bodyFixed, 0, true, true},
		{"line without colon", "HTTP/1.1 200 OK\r\nnonsense\r\nContent-Length: 0\r\n\r\n", "GET", bodyFixed, 0, true, true},
		{"same lengths", "HTTP/1.1 200 OK\r\nContent-Length: 3\r\ncontent-length: 3\r\n\r\n", "GET", bodyFixed, 3, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := head(t, tt.raw, tt.method)
			if err != nil {
				t.Fatal(err)
			}
			if h.framing != tt.framing || h.length != tt.length || h.keepAlive != tt.keepAlive || h.anomaly != tt.anomaly {
				t.Errorf("framing %d length %d keep-alive %v anomaly %v", h.framing, h.length, h.keepAlive, h.anomaly)
			}
		})
	}
}

func TestReadHeadKeepsOrderAndCase(t *testing.T) {
	h, err := head(t, "HTTP/1.1 201 Created Here\r\nZ-Last: 1\r\nx-lower: 2\r\nA-First:  3 \r\nX-Lower: 4\r\nContent-Length: 0\r\n\r\n", "GET")
	if err != nil {
		t.Fatal(err)
	}
	want := []exchange.Header{{Name: "Z-Last", Value: "1"}, {Name: "x-lower", Value: "2"}, {Name: "A-First", Value: "3"}, {Name: "X-Lower", Value: "4"}, {Name: "Content-Length", Value: "0"}}
	if len(h.headers) != len(want) {
		t.Fatalf("headers %v", h.headers)
	}
	for i := range want {
		if h.headers[i] != want[i] {
			t.Errorf("header %d: %v, want %v", i, h.headers[i], want[i])
		}
	}
	if h.status != 201 || h.reason != "Created Here" || h.statusText() != "201 Created Here" {
		t.Errorf("status %d %q", h.status, h.reason)
	}
}

func TestReadHeadRefuses(t *testing.T) {
	for name, raw := range map[string]string{
		"http/2":                 "HTTP/2 200\r\n\r\n",
		"no status":              "HTTP/1.1\r\n\r\n",
		"bad code":               "HTTP/1.1 2x0 OK\r\n\r\n",
		"no space after code":    "HTTP/1.1 200OK\r\n\r\n",
		"trailing space framing": "HTTP/1.1 200 OK\r\nContent-Length : 5\r\n\r\n",
		"ctl in framing":         "HTTP/1.1 200 OK\r\nTransfer-Encoding\x01: chunked\r\n\r\n",
		"bad length":             "HTTP/1.1 200 OK\r\nContent-Length: 5x\r\n\r\n",
		"negative length":        "HTTP/1.1 200 OK\r\nContent-Length: -1\r\n\r\n",
		"conflicting lengths":    "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Length: 6\r\n\r\n",
		"length list":            "HTTP/1.1 200 OK\r\nContent-Length: 5, 5\r\n\r\n",
		"fold first":             "HTTP/1.1 200 OK\r\n folded\r\n\r\n",
		"too large":              "HTTP/1.1 200 OK\r\nX: " + strings.Repeat("a", maxHeadBytes) + "\r\n\r\n",
	} {
		if _, err := head(t, raw, "GET"); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	if _, err := head(t, "HTTP/1.1 200 OK\r\nX: 1\r\n", "GET"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("truncated head: %v", err)
	}
}

func TestChunkedReader(t *testing.T) {
	raw := "5;ext=1\r\nhello\r\n6 \r\n world\r\n0\r\nX-Trailer: t\r\n\r\nNEXT"
	br := bufio.NewReader(strings.NewReader(raw))
	c := &chunkedReader{br: br, budget: maxHeadBytes}
	b, err := io.ReadAll(c)
	if err != nil || string(b) != "hello world" {
		t.Fatalf("body %q, %v", b, err)
	}
	if len(c.trailers) != 1 || c.trailers[0].Name != "X-Trailer" {
		t.Errorf("trailers %v", c.trailers)
	}
	if rest, _ := io.ReadAll(br); string(rest) != "NEXT" {
		t.Errorf("read past the body: %q", rest)
	}
	for name, raw := range map[string]string{
		"bad size":       "zz\r\n",
		"missing crlf":   "5\r\nhelloX0\r\n\r\n",
		"truncated":      "5\r\nhel",
		"huge size":      "fffffffffffffffff\r\n",
		"no final chunk": "5\r\nhello\r\n",
	} {
		c := &chunkedReader{br: bufio.NewReader(strings.NewReader(raw)), budget: maxHeadBytes}
		if _, err := io.ReadAll(c); err == nil {
			t.Errorf("%s: read", name)
		}
	}
}

func TestFixedReaderShortBody(t *testing.T) {
	f := &fixedReader{r: strings.NewReader("abc"), n: 5}
	if _, err := io.ReadAll(f); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("short body: %v", err)
	}
}
