// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/httpx"
)

// dialInteractive dials url through a fresh client with opts.
func dialInteractive(ctx context.Context, t *testing.T, url string, opts *httpx.Options, lim Options) (*Interactive, error) {
	t.Helper()
	u, err := newClient(t).Upgrade(ctx, &httpx.RequestSpec{Method: "GET", URL: url}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return DialInteractive(ctx, u, lim)
}

// collector gathers the messages a session reports.
type collector struct {
	mu   sync.Mutex
	msgs []exchange.Message
	got  chan struct{}
}

func newCollector() *collector { return &collector{got: make(chan struct{}, 16)} }

func (c *collector) on(m exchange.Message) {
	c.mu.Lock()
	c.msgs = append(c.msgs, m)
	c.mu.Unlock()
	c.got <- struct{}{}
}

func (c *collector) wait(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-c.got:
		case <-time.After(5 * time.Second):
			t.Fatal("no message")
		}
	}
}

func waitDone(t *testing.T, s *Interactive) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not end")
	}
}

func TestInteractive(t *testing.T) {
	srv := wsServer(t)
	c := newCollector()
	s, err := dialInteractive(context.Background(), t, wsURL(srv, "/echo"), &httpx.Options{}, Options{OnMessage: c.on})
	if err != nil {
		t.Fatal(err)
	}
	s.Steps() <- Step{Kind: Send, Data: []byte("one")}
	s.Steps() <- Step{Kind: Receive} // ignored
	s.Steps() <- Step{Kind: Send, Data: []byte{0, 1}, Binary: true}
	c.wait(t, 4)
	s.Steps() <- Step{Kind: Close, Code: 1001}
	waitDone(t, s)
	if err := s.Err(); err != nil {
		t.Errorf("Err = %v", err)
	}
	if got := received(s.Response); len(got) != 2 || got[0] != "one" || got[1] != "\x00\x01" {
		t.Errorf("received %q", got)
	}
	if len(s.Response.Stream.Messages) != 4 || s.Response.Stream.StopReason != exchange.StopScript {
		t.Errorf("stream %+v", s.Response.Stream)
	}
	var upgrade bool
	for _, h := range s.Request.Headers {
		upgrade = upgrade || h.Name == "Upgrade"
	}
	if !upgrade {
		t.Errorf("request headers %v", s.Request.Headers)
	}
}

func TestInteractiveServerClose(t *testing.T) {
	srv := wsServer(t)
	s, err := dialInteractive(context.Background(), t, wsURL(srv, "/echo"), &httpx.Options{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s.Steps() <- Step{Kind: Send, Data: []byte("close-me")}
	waitDone(t, s)
	if err := s.Err(); err == nil || !strings.Contains(err.Error(), "code 4000") {
		t.Errorf("Err = %v", err)
	}
}

func TestInteractiveMaxBytes(t *testing.T) {
	srv := wsServer(t)
	s, err := dialInteractive(context.Background(), t, wsURL(srv, "/echo"), &httpx.Options{}, Options{MaxBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	s.Steps() <- Step{Kind: Send, Data: []byte("abc")}
	select {
	case s.Steps() <- Step{Kind: Send, Data: []byte("def")}:
	case <-s.Done():
	}
	waitDone(t, s)
	if err := s.Err(); err == nil {
		t.Error("no error past MaxBytes")
	}
}

func TestInteractiveContext(t *testing.T) {
	srv := wsServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	s, err := dialInteractive(ctx, t, wsURL(srv, "/echo"), &httpx.Options{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	waitDone(t, s)
	if s.Err() == nil {
		t.Error("no error after the context ended")
	}
}

func TestInteractiveRefused(t *testing.T) {
	srv := wsServer(t)
	s, err := dialInteractive(context.Background(), t, wsURL(srv, "/private"), &httpx.Options{}, Options{})
	if err == nil || s == nil || s.Response.Status != 401 {
		t.Fatalf("session %v, err %v", s, err)
	}
	waitDone(t, s)
}

func TestInteractiveConnectError(t *testing.T) {
	srv := wsServer(t)
	url := wsURL(srv, "/echo")
	srv.Close()
	s, err := dialInteractive(context.Background(), t, url, &httpx.Options{}, Options{})
	var he *httpx.Error
	if s != nil || err == nil || !errors.As(err, &he) || he.Kind != httpx.ErrConnect {
		t.Errorf("session %v, err %v", s, err)
	}
}

// TestInteractiveProxy dials through the entry's HTTP proxy.
func TestInteractiveProxy(t *testing.T) {
	srv := wsServer(t)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	var proxied atomic.Int32
	rp := httputil.NewSingleHostReverseProxy(target)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		rp.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	c := newCollector()
	s, err := dialInteractive(context.Background(), t, "ws://sonde-ws.test/echo", &httpx.Options{Proxy: proxy.URL}, Options{OnMessage: c.on})
	if err != nil {
		t.Fatal(err)
	}
	s.Steps() <- Step{Kind: Send, Data: []byte("via proxy")}
	c.wait(t, 2)
	s.Steps() <- Step{Kind: Close}
	waitDone(t, s)
	if proxied.Load() != 1 {
		t.Errorf("%d requests through the proxy", proxied.Load())
	}
}

// TestInteractiveCallbackReentry calls Err from OnMessage, which must not
// deadlock.
func TestInteractiveCallbackReentry(t *testing.T) {
	srv := wsServer(t)
	var s *Interactive
	ready := make(chan struct{})
	got := make(chan error, 4)
	s, err := dialInteractive(context.Background(), t, wsURL(srv, "/echo"), &httpx.Options{}, Options{OnMessage: func(exchange.Message) {
		<-ready
		got <- s.Err()
	}})
	if err != nil {
		t.Fatal(err)
	}
	close(ready)
	s.Steps() <- Step{Kind: Send, Data: []byte("x")}
	for range 2 {
		select {
		case err := <-got:
			if err != nil {
				t.Errorf("Err = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("OnMessage calling Err deadlocks")
		}
	}
	s.Steps() <- Step{Kind: Close}
	waitDone(t, s)
}
