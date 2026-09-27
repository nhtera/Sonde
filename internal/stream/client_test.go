// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec // G505: RFC 6455 accept key
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/goleak"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/sandbox"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// sseServer writes n events (all when n < 0, then keeps the connection
// open until the client leaves), then closes when closeAfter is set.
func sseServer(t *testing.T, events []string, closeAfter bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f := w.(http.Flusher)
		for _, e := range events {
			_, _ = fmt.Fprint(w, e)
			f.Flush()
		}
		if !closeAfter {
			<-r.Context().Done()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T) *httpx.Client {
	t.Helper()
	box, err := sandbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = box.Close() })
	c, err := httpx.NewClient(httpx.ClientConfig{Sandbox: box, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// execSSE runs a GET through httpx with ReadSSE as its body reader.
func execSSE(t *testing.T, url string, lim Options) (*exchange.Response, *exchange.Stream, error) {
	t.Helper()
	var s *exchange.Stream
	opts := &httpx.Options{ReadStream: func(body io.Reader, stop func()) error {
		st, err := ReadSSE(body, stop, lim)
		s = st
		return err
	}}
	calls, err := newClient(t).Execute(context.Background(), &httpx.RequestSpec{Method: "GET", URL: url}, opts)
	if err != nil {
		return nil, s, err
	}
	return calls[len(calls)-1].Response, s, nil
}

func TestReadSSE(t *testing.T) {
	events := []string{"event: ready\ndata: 1\n\n", ": keep-alive\n", "data: 2\n\n", "data: 3\n\n"}
	for _, tc := range []struct {
		name       string
		closeAfter bool
		lim        Options
		reason     exchange.StopReason
		count      int
	}{
		{"count", false, Options{Count: 2}, exchange.StopCount, 2},
		{"timeout", false, Options{Timeout: 200 * time.Millisecond}, exchange.StopTimeout, 3},
		{"closed", true, Options{}, exchange.StopClosed, 3},
		{"max-bytes", false, Options{MaxBytes: 30}, exchange.StopMaxBytes, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := sseServer(t, events, tc.closeAfter)
			resp, s, err := execSSE(t, srv.URL, tc.lim)
			if err != nil {
				t.Fatal(err)
			}
			if s.StopReason != tc.reason || len(s.Messages) != tc.count {
				t.Fatalf("stop %q with %d events, want %q with %d", s.StopReason, len(s.Messages), tc.reason, tc.count)
			}
			if m := s.Messages[0]; m.Event != "ready" || string(m.Data) != "1" {
				t.Errorf("first event = %+v", m)
			}
			if resp.Status != 200 || !strings.HasPrefix(string(resp.Body), "event: ready") {
				t.Errorf("response %d %q", resp.Status, resp.Body)
			}
		})
	}
}

// TestReadSSEMaxTime checks max-time still fails a stream: only the stream
// limits are soft.
func TestReadSSEMaxTime(t *testing.T) {
	srv := sseServer(t, []string{"data: 1\n\n"}, false)
	var s *exchange.Stream
	opts := &httpx.Options{Timeout: 200 * time.Millisecond, ReadStream: func(body io.Reader, stop func()) error {
		st, err := ReadSSE(body, stop, Options{Timeout: time.Minute})
		s = st
		return err
	}}
	_, err := newClient(t).Execute(context.Background(), &httpx.RequestSpec{Method: "GET", URL: srv.URL}, opts)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if s == nil || len(s.Messages) != 1 {
		t.Errorf("stream = %+v", s)
	}
}

func TestSSEStream(t *testing.T) {
	s := SSEStream([]byte("data: a\n\nevent: e\ndata: b\n\n"))
	if s.Protocol != exchange.ProtocolSSE || len(s.Messages) != 2 || s.Messages[1].Event != "e" {
		t.Errorf("stream = %+v", s)
	}
}

// wsServer echoes every message, answers "close-me" by closing with 4000,
// and ignores "silent".
func wsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/private" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow() //nolint:errcheck // test
		if r.URL.Path == "/greet" {
			_ = c.Write(r.Context(), websocket.MessageText, []byte("hello "+r.Header.Get("X-Name")))
		}
		for {
			typ, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			switch string(data) {
			case "close-me":
				_ = c.Close(4000, "bye")
				return
			case "silent":
				continue
			}
			if err := c.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runWS(t *testing.T, url string, headers []exchange.Header, steps []Step, lim Options) (*exchange.Response, error) {
	t.Helper()
	c := newClient(t)
	u, err := c.Upgrade(context.Background(), &httpx.RequestSpec{Method: "GET", URL: url, Headers: headers}, &httpx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return WebSocket(context.Background(), u, steps, lim)
}

func wsURL(srv *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + path
}

func received(r *exchange.Response) []string {
	var out []string
	for _, m := range r.Stream.ReceivedMessages() {
		out = append(out, string(m.Data))
	}
	return out
}

func TestWebSocket(t *testing.T) {
	srv := wsServer(t)
	r, err := runWS(t, wsURL(srv, "/greet"), []exchange.Header{{Name: "X-Name", Value: "bob"}}, []Step{
		{Kind: Receive},
		{Kind: Send, Data: []byte(`{"type":"ping"}`)},
		{Kind: Send, Data: []byte{0, 0xff}, Binary: true},
		{Kind: Receive, Count: 2},
		{Kind: Close, Code: 1001},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != 101 || r.Stream.StopReason != exchange.StopScript {
		t.Fatalf("status %d, stop %q", r.Status, r.Stream.StopReason)
	}
	if got := received(r); len(got) != 3 || got[0] != "hello bob" || got[1] != `{"type":"ping"}` || got[2] != "\x00\xff" {
		t.Errorf("received %q", got)
	}
	if msgs := r.Stream.Messages; len(msgs) != 5 || msgs[1].Direction != exchange.Sent || !msgs[4].Binary {
		t.Errorf("transcript %+v", msgs)
	}
}

func TestWebSocketHTTPScheme(t *testing.T) {
	srv := wsServer(t)
	r, err := runWS(t, srv.URL+"/echo", nil, []Step{{Kind: Send, Data: []byte("a")}, {Kind: Receive}}, Options{})
	if err != nil || received(r)[0] != "a" {
		t.Fatalf("err %v", err)
	}
}

func TestWebSocketRefused(t *testing.T) {
	srv := wsServer(t)
	r, err := runWS(t, wsURL(srv, "/private"), nil, []Step{{Kind: Receive}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != 401 || r.Stream != nil || !strings.Contains(string(r.Body), "no") {
		t.Errorf("status %d, stream %v, body %q", r.Status, r.Stream, r.Body)
	}
}

func TestWebSocketErrors(t *testing.T) {
	srv := wsServer(t)
	for _, tc := range []struct {
		name  string
		steps []Step
		lim   Options
		want  string
	}{
		{"timeout", []Step{{Kind: Send, Data: []byte("silent")}, {Kind: Receive}}, Options{Timeout: 200 * time.Millisecond},
			"timeout waiting for message 1 of 1"},
		{"server close", []Step{{Kind: Send, Data: []byte("a")}, {Kind: Send, Data: []byte("close-me")}, {Kind: Receive, Count: 3}}, Options{},
			"the server closed the connection (code 4000) while waiting for message 2 of 3"},
		{"max bytes", []Step{{Kind: Send, Data: []byte("0123456789")}, {Kind: Receive}}, Options{MaxBytes: 5},
			"message 1 of 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := runWS(t, wsURL(srv, "/echo"), nil, tc.steps, tc.lim)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			var se *StepError
			if !errors.As(err, &se) || se.Index != len(tc.steps)-1 {
				t.Errorf("step error = %#v", err)
			}
			if r == nil || r.Stream == nil || r.Stream.StopReason != "" {
				t.Errorf("response %+v", r)
			}
		})
	}
}

func TestWebSocketConnectError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := wsURL(srv, "/")
	srv.Close()
	_, err := runWS(t, url, nil, nil, Options{})
	var he *httpx.Error
	if !errors.As(err, &he) || he.Kind != httpx.ErrConnect {
		t.Errorf("err = %#v", err)
	}
}

// TestWebSocketTLS checks wss:// uses the entry's TLS options, and that the
// handshake stays on HTTP/1.1 when http2 is asked for.
func TestWebSocketTLS(t *testing.T) {
	srv := httptest.NewUnstartedServer(wsServer(t).Config.Handler)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	url := "wss" + strings.TrimPrefix(srv.URL, "https") + "/echo"
	c := newClient(t)
	steps := []Step{{Kind: Send, Data: []byte("tls")}, {Kind: Receive}}
	u, err := c.Upgrade(context.Background(), &httpx.RequestSpec{Method: "GET", URL: url}, &httpx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WebSocket(context.Background(), u, steps, Options{}); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("err = %v, want a certificate error", err)
	}
	u, err = c.Upgrade(context.Background(), &httpx.RequestSpec{Method: "GET", URL: url}, &httpx.Options{Insecure: true, HTTPVersion: httpx.HTTP2})
	if err != nil {
		t.Fatal(err)
	}
	r, err := WebSocket(context.Background(), u, steps, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "HTTP/1.1" || r.Certificate == nil || received(r)[0] != "tls" {
		t.Errorf("version %s, cert %v, received %q", r.Version, r.Certificate, received(r))
	}
}

// TestReadSSEGzip checks a compressed stream is decoded as it arrives, while
// the response keeps the bytes as received.
func TestReadSSEGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		for i := range 3 {
			fmt.Fprintf(zw, "data: %d\n\n", i)
			_ = zw.Flush()
			w.(http.Flusher).Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	resp, s, err := execSSE(t, srv.URL, Options{Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Messages) != 2 || string(s.Messages[1].Data) != "1" || s.StopReason != exchange.StopCount {
		t.Fatalf("stream = %+v", s)
	}
	if !bytes.HasPrefix(resp.Body, []byte{0x1f, 0x8b}) {
		t.Errorf("body is not the gzip bytes received: %q", resp.Body)
	}
}

func TestReadSSEBadEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = io.WriteString(w, "data: not gzip\n\n")
	}))
	t.Cleanup(srv.Close)
	_, _, err := execSSE(t, srv.URL, Options{Count: 1})
	var he *httpx.Error
	if !errors.As(err, &he) || he.Description != "Decompression error" {
		t.Errorf("err = %v", err)
	}
}

// TestWebSocketRefusedWholeBody checks the body of a refused upgrade is
// kept whole.
func TestWebSocketRefusedWholeBody(t *testing.T) {
	big := strings.Repeat("x", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, big, http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	r, err := runWS(t, wsURL(srv, "/"), nil, []Step{{Kind: Receive}}, Options{})
	if err != nil || r.Status != 403 || len(r.Body) != len(big)+1 {
		t.Errorf("status %d, %d body bytes, err %v", r.Status, len(r.Body), err)
	}
}

// TestWebSocketCloseBounded checks a server that never answers a close
// frame does not hold the entry beyond the stream timeout.
func TestWebSocketCloseBounded(t *testing.T) {
	// A raw handshake: the server then reads frames without ever answering
	// the close frame, until the client drops the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")) //nolint:gosec // G401: RFC 6455 accept key
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck // test
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
			base64.StdEncoding.EncodeToString(sum[:]))
		_ = rw.Flush()
		_, _ = io.Copy(io.Discard, rw)
	}))
	t.Cleanup(srv.Close)
	for _, steps := range [][]Step{{{Kind: Close}}, nil} {
		start := time.Now()
		_, err := runWS(t, wsURL(srv, "/"), nil, steps, Options{Timeout: 300 * time.Millisecond})
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("steps %v: took %v", steps, d)
		}
		if steps != nil && (err == nil || !strings.Contains(err.Error(), "timeout")) {
			t.Errorf("close step: err = %v", err)
		}
		if steps == nil && err != nil {
			t.Errorf("implicit close: err = %v", err)
		}
	}
}

// TestWebSocketMaxTime checks max-time is named when it ends a receive.
func TestWebSocketMaxTime(t *testing.T) {
	srv := wsServer(t)
	c := newClient(t)
	u, err := c.Upgrade(context.Background(), &httpx.RequestSpec{Method: "GET", URL: wsURL(srv, "/echo")},
		&httpx.Options{Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = WebSocket(context.Background(), u, []Step{{Kind: Receive}}, Options{Timeout: time.Minute})
	if err == nil || !strings.Contains(err.Error(), "timeout (max-time) waiting for message 1 of 1") {
		t.Errorf("err = %v", err)
	}
}
