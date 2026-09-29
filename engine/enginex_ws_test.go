// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/netpolicy"
	"github.com/nhtera/sonde/internal/stream"
)

// TestDialWS opens an interactive session for a [SondeMessages] entry,
// with the job's variables and the runner's seed cookies, sends a message,
// gets its echo and closes.
func TestDialWS(t *testing.T) {
	srv := streamServer(t)
	ws := "ws" + strings.TrimPrefix(srv.URL, "http")
	src := "GET {{base}}/hello\n\nGET {{ws}}/ws\nX-Token: {{tok}}\n[SondeMessages]\nsend: `scripted`\n"
	job := Job{Name: filepath.Join(t.TempDir(), "t.sonde"), Source: []byte(src), Variables: map[string]any{"ws": ws}}
	r := NewRunner(Options{Secrets: map[string]string{"tok": "tok-sentinel"}})
	host := strings.TrimPrefix(srv.URL, "http://")
	enginex.SeedCookies(r, []Cookie{{Domain: strings.Split(host, ":")[0], Path: "/", Name: "sid", Value: "seeded"}})

	var mu sync.Mutex
	var msgs []exchange.Message
	got := make(chan struct{}, 4)
	s, redact, err := enginex.DialWS(context.Background(), r, job, 2, func(m exchange.Message) {
		mu.Lock()
		msgs = append(msgs, m)
		mu.Unlock()
		got <- struct{}{}
	})
	if err != nil {
		t.Fatal(err)
	}
	if redact("x tok-sentinel") != "x ***" {
		t.Errorf("redactor: %q", redact("x tok-sentinel"))
	}
	var header, cookie bool
	for _, h := range s.Request.Headers {
		header = header || h.Name == "X-Token" && h.Value == "tok-sentinel"
		cookie = cookie || h.Name == "Cookie" && h.Value == "sid=seeded"
	}
	if !header || !cookie || s.Response.Status != 101 {
		t.Errorf("handshake %+v -> %d", s.Request.Headers, s.Response.Status)
	}
	s.Steps() <- stream.Step{Kind: stream.Send, Data: []byte("hello")}
	for range 2 {
		select {
		case <-got:
		case <-time.After(5 * time.Second):
			t.Fatal("no echo")
		}
	}
	s.Steps() <- stream.Step{Kind: stream.Close}
	<-s.Done()
	if err := s.Err(); err != nil {
		t.Errorf("Err = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(msgs) != 2 || msgs[0].Direction != exchange.Sent || string(msgs[1].Data) != "hello" {
		t.Errorf("messages %+v", msgs)
	}
	if n := len(s.Response.Stream.Messages); n != 2 {
		t.Errorf("stream has %d messages", n)
	}
}

// TestDialWSRefused checks the host policy, a non-WebSocket entry and a
// context that ends the session.
func TestDialWSRefused(t *testing.T) {
	srv := streamServer(t)
	ws := "ws" + strings.TrimPrefix(srv.URL, "http")
	src := "GET {{ws}}/ws\n[SondeMessages]\nreceive\n\nGET {{ws}}/other\n\nGET {{ws}}/ws\n[SondeMessages]\nreceive\n\n```\nbody\n```\n"
	job := Job{Name: filepath.Join(t.TempDir(), "t.sonde"), Source: []byte(src), Variables: map[string]any{"ws": ws}}

	r := NewRunner(Options{})
	policy, err := netpolicy.Parse([]string{"allowed.test"})
	if err != nil {
		t.Fatal(err)
	}
	enginex.SetHosts(r, policy)
	if _, _, err := enginex.DialWS(context.Background(), r, job, 1, nil); enginex.Transport(err) != "host-denied" {
		t.Errorf("denied host: %v", err)
	}
	if _, _, err := enginex.DialWS(context.Background(), NewRunner(Options{}), job, 2, nil); err == nil || !strings.Contains(err.Error(), "not a WebSocket entry") {
		t.Errorf("HTTP entry: %v", err)
	}

	if _, _, err := enginex.DialWS(context.Background(), NewRunner(Options{}), job, 3, nil); err == nil || !strings.Contains(err.Error(), "no request body") {
		t.Errorf("entry with a body: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s, _, err := enginex.DialWS(ctx, NewRunner(Options{}), job, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the session outlives its context")
	}
	if s.Err() == nil {
		t.Error("no error after the context ended")
	}
}
