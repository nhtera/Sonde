// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package h1wire

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// scripted is a TCP server that runs serve on each connection and
// counts them.
type scripted struct {
	ln    net.Listener
	conns atomic.Int32
	wg    sync.WaitGroup
}

func newScripted(t *testing.T, serve func(c net.Conn, br *bufio.Reader)) *scripted {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &scripted{ln: ln}
	s.wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.conns.Add(1)
			s.wg.Go(func() {
				defer c.Close()
				serve(c, bufio.NewReader(c))
			})
		}
	})
	t.Cleanup(func() { _ = ln.Close(); s.wg.Wait() })
	return s
}

func (s *scripted) url() string { return "http://" + s.ln.Addr().String() }

// readRequest reads a request head and its Content-Length body.
func readRequest(br *bufio.Reader) (head string, body []byte, err error) {
	var b strings.Builder
	n := 0
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", nil, err
		}
		b.WriteString(line)
		if line == "\r\n" {
			break
		}
		if v, ok := strings.CutPrefix(strings.ToLower(line), "content-length: "); ok {
			for _, c := range strings.TrimSpace(v) {
				n = n*10 + int(c-'0')
			}
		}
	}
	body = make([]byte, n)
	_, err = io.ReadFull(br, body)
	return b.String(), body, err
}

func newTransport() *Transport {
	d := &net.Dialer{}
	return &Transport{Dial: d.DialContext}
}

func get(t *testing.T, tr *Transport, url string, w *Wire) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if w != nil {
		req = req.WithContext(WithWire(req.Context(), w))
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp, string(b)
}

// TestWritesAndReadsAsGiven checks the request line version and the
// request headers as written, and the response headers in wire order
// and case.
func TestWritesAndReadsAsGiven(t *testing.T) {
	var got string
	srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
		got, _, _ = readRequest(br)
		_, _ = io.WriteString(c, "HTTP/1.0 200 OK\r\nzeta: 1\r\n<script>x</script>: 2\r\nAlpha: 3\r\nContent-Length: 2\r\n\r\nok")
	})
	w := &Wire{HTTP10: true, RequestHeaders: []exchange.Header{
		{Name: "Host", Value: "example.test"}, {Name: "x-lower", Value: "a"}, {Name: "Accept", Value: "*/*"}, {Name: "X-Empty", Value: ""},
	}}
	resp, body := get(t, newTransport(), srv.url()+"/p?q=1", w)
	if want := "GET /p?q=1 HTTP/1.0\r\nHost: example.test\r\nx-lower: a\r\nAccept: */*\r\nX-Empty:\r\n\r\n"; got != want {
		t.Errorf("request\n%q\nwant\n%q", got, want)
	}
	if body != "ok" || resp.Proto != "HTTP/1.0" || resp.Header.Get("Alpha") != "3" {
		t.Errorf("response %s %q %v", resp.Proto, body, resp.Header)
	}
	var names []string
	for _, h := range w.ResponseHeaders {
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "zeta,<script>x</script>,Alpha,Content-Length" {
		t.Errorf("response header order %v", names)
	}
}

// TestRequestChecks checks that a request that would not read back as
// written is refused before anything is sent.
func TestRequestChecks(t *testing.T) {
	srv := newScripted(t, func(net.Conn, *bufio.Reader) {})
	for name, headers := range map[string][]exchange.Header{
		"header injection": {{Name: "Host", Value: "a"}, {Name: "X", Value: "a\r\nInjected: 1"}},
		"bad name":         {{Name: "Host", Value: "a"}, {Name: "X Y", Value: "1"}},
		"two hosts":        {{Name: "Host", Value: "a"}, {Name: "host", Value: "b"}},
		"no host":          {{Name: "X", Value: "1"}},
	} {
		req, _ := http.NewRequest(http.MethodGet, srv.url(), nil)
		req = req.WithContext(WithWire(req.Context(), &Wire{RequestHeaders: headers}))
		var re *RequestError
		if _, err := newTransport().RoundTrip(req); !errors.As(err, &re) {
			t.Errorf("%s: %v, want a RequestError", name, err)
		}
	}
	if srv.conns.Load() != 0 {
		t.Errorf("%d connections opened for refused requests", srv.conns.Load())
	}
}

// TestConnectionReuse checks when a connection carries the next request:
// after a framed body read to its end, not after Connection: close, a
// body read to the close, a body not read, or an anomaly.
func TestConnectionReuse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		read     bool
		want     int32
	}{
		{"content-length", "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", true, 1},
		{"chunked", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nok\r\n0\r\n\r\n", true, 1},
		{"connection close", "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok", true, 2},
		{"body not read", "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", false, 2},
		{"lenient header name", "HTTP/1.1 200 OK\r\nbad name: 1\r\nContent-Length: 2\r\n\r\nok", true, 2},
		{"content-length and chunked", "HTTP/1.1 200 OK\r\nContent-Length: 9\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nok\r\n0\r\n\r\n", true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
				for {
					if _, _, err := readRequest(br); err != nil {
						return
					}
					_, _ = io.WriteString(c, tt.response)
				}
			})
			tr := newTransport()
			defer tr.CloseIdleConnections()
			for range 2 {
				req, _ := http.NewRequest(http.MethodGet, srv.url(), nil)
				resp, err := tr.RoundTrip(req)
				if err != nil {
					t.Fatal(err)
				}
				if tt.read {
					if b, err := io.ReadAll(resp.Body); err != nil || string(b) != "ok" {
						t.Fatalf("body %q, %v", b, err)
					}
				}
				_ = resp.Body.Close()
			}
			if got := srv.conns.Load(); got != tt.want {
				t.Errorf("%d connections, want %d", got, tt.want)
			}
		})
	}
}

// TestStaleConnectionRetry checks that an idempotent request failing on
// a reused connection before any response byte is sent again on a new
// one, body included.
func TestStaleConnectionRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
		// Each connection answers one request, then closes without
		// saying so.
		_, body, err := readRequest(br)
		if err != nil {
			return
		}
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	})
	tr := newTransport()
	defer tr.CloseIdleConnections()
	for i := range 3 {
		req, _ := http.NewRequest(http.MethodPut, srv.url(), bytes.NewReader([]byte("payload")))
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 || bodies[2] != "payload" {
		t.Errorf("bodies %q", bodies)
	}
}

// TestLease checks that a leased connection carries the lease's
// requests and never goes to the pool.
func TestLease(t *testing.T) {
	srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
		for {
			if _, _, err := readRequest(br); err != nil {
				return
			}
			_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		}
	})
	tr := newTransport()
	defer tr.CloseIdleConnections()
	lease := &Lease{}
	get(t, tr, srv.url(), &Wire{Lease: lease})
	get(t, tr, srv.url(), &Wire{Lease: lease})
	get(t, tr, srv.url(), nil) // not the leased connection
	if got := srv.conns.Load(); got != 2 {
		t.Errorf("%d connections, want 2", got)
	}
	if err := lease.Close(); err != nil {
		t.Error(err)
	}
}

// TestExpectContinue checks that a body waits for 100 Continue and is
// not sent when the server answers at once.
func TestExpectContinue(t *testing.T) {
	for _, accept := range []bool{true, false} {
		var gotBody []byte
		srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
			head, _ := br.ReadString('\n')
			for line := head; line != "\r\n"; {
				line, _ = br.ReadString('\n')
			}
			if !accept {
				_, _ = io.WriteString(c, "HTTP/1.1 417 Expectation Failed\r\nContent-Length: 0\r\n\r\n")
				return
			}
			_, _ = io.WriteString(c, "HTTP/1.1 100 Continue\r\n\r\n")
			gotBody = make([]byte, 4)
			_, _ = io.ReadFull(br, gotBody)
			_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
		})
		req, _ := http.NewRequest(http.MethodPut, srv.url(), strings.NewReader("data"))
		req = req.WithContext(WithWire(context.Background(), &Wire{RequestHeaders: []exchange.Header{
			{Name: "Host", Value: "x"}, {Name: "Expect", Value: "100-continue"}, {Name: "Content-Length", Value: "4"},
		}}))
		resp, err := newTransport().RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if accept && (resp.StatusCode != 200 || string(gotBody) != "data") {
			t.Errorf("accepted: %d, body %q", resp.StatusCode, gotBody)
		}
		if !accept && resp.StatusCode != 417 {
			t.Errorf("refused: %d", resp.StatusCode)
		}
	}
}

// TestNoRetryForPOST checks that a POST the server took but did not
// answer is not sent again.
func TestNoRetryForPOST(t *testing.T) {
	var requests atomic.Int32
	srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
		if _, _, err := readRequest(br); err != nil {
			return
		}
		requests.Add(1)
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		if _, _, err := readRequest(br); err == nil {
			requests.Add(1) // taken, then the connection drops unanswered
		}
	})
	tr := newTransport()
	defer tr.CloseIdleConnections()
	get(t, tr, srv.url(), nil)
	req, _ := http.NewRequest(http.MethodPost, srv.url(), bytes.NewReader([]byte("once")))
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("unanswered POST succeeded")
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("server took %d requests, want 2 (the POST once)", got)
	}
}

// TestLeftoverBytesNotReused checks that bytes after a response's body
// are never read as the next request's response.
func TestLeftoverBytesNotReused(t *testing.T) {
	for name, extra := range map[string]string{
		"pipelined response": "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nEVIL!",
		"late 408":           "HTTP/1.1 408 Request Timeout\r\nContent-Length: 0\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			first := true
			var mu sync.Mutex
			srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
				for {
					if _, _, err := readRequest(br); err != nil {
						return
					}
					mu.Lock()
					f := first
					first = false
					mu.Unlock()
					if f {
						_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
						time.Sleep(20 * time.Millisecond)
						_, _ = io.WriteString(c, extra)
						continue
					}
					_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nreal")
				}
			})
			tr := newTransport()
			defer tr.CloseIdleConnections()
			get(t, tr, srv.url(), nil)
			time.Sleep(50 * time.Millisecond)
			if _, body := get(t, tr, srv.url(), nil); body != "real" {
				t.Errorf("second response %q", body)
			}
		})
	}
}

// TestLongChunkedBody checks that the trailer budget does not limit the
// number of chunks.
func TestLongChunkedBody(t *testing.T) {
	const n = 400_000
	srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
		if _, _, err := readRequest(br); err != nil {
			return
		}
		w := bufio.NewWriter(c)
		_, _ = io.WriteString(w, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n")
		for range n {
			_, _ = io.WriteString(w, "1\r\na\r\n")
		}
		_, _ = io.WriteString(w, "0\r\n\r\n")
		_ = w.Flush()
	})
	if _, body := get(t, newTransport(), srv.url(), nil); len(body) != n {
		t.Errorf("read %d bytes, want %d", len(body), n)
	}
}

// TestRequestConnectionClose checks that a request with Connection: close,
// or an HTTP/1.0 one, does not leave its connection for the next request.
func TestRequestConnectionClose(t *testing.T) {
	for name, w := range map[string]*Wire{
		"connection close": {RequestHeaders: []exchange.Header{{Name: "Host", Value: "h"}, {Name: "Connection", Value: "close"}}},
		"http/1.0":         {HTTP10: true},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newScripted(t, func(c net.Conn, br *bufio.Reader) {
				for {
					if _, _, err := readRequest(br); err != nil {
						return
					}
					_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
				}
			})
			tr := newTransport()
			defer tr.CloseIdleConnections()
			get(t, tr, srv.url(), w)
			get(t, tr, srv.url(), nil)
			if got := srv.conns.Load(); got != 2 {
				t.Errorf("%d connections, want 2", got)
			}
		})
	}
}
