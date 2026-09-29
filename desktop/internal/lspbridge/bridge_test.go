// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lspbridge

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/internal/config"
)

func waitTopic(t *testing.T, rec *emit.Recorder, topic, contains string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range rec.Events() {
			if s, ok := ev.Data.(string); ev.Topic == topic && ok && strings.Contains(s, contains) {
				return s
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %q on %s", contains, topic)
	return ""
}

func TestBridge(t *testing.T) {
	rec := &emit.Recorder{}
	b := NewBridge(rec, config.Env{}, "test")
	defer b.ServiceShutdown() //nolint:errcheck // test cleanup
	id, err := b.Open()
	if err != nil || len(id) != 32 {
		t.Fatalf("open %q %v", id, err)
	}
	if err := b.Send(id, initialize(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	waitTopic(t, rec, "lsp:"+id, `"capabilities"`)
	if err := b.Configure(id, "local", []string{"tok"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Ping(id); err != nil {
		t.Fatal(err)
	}
	b.Close(id)
	waitTopic(t, rec, "lsp:"+id+":closed", id)
	var e *apperr.Error
	if err := b.Send(id, "{}"); !errors.As(err, &e) || e.Code != apperr.Expired {
		t.Errorf("send to a closed session: %v", err)
	}
	if err := b.Ping("nope"); !errors.As(err, &e) || e.Code != apperr.Expired {
		t.Errorf("unknown session: %v", err)
	}
}

// TestIdleSessionReaped: a page that stops pinging loses its server.
func TestIdleSessionReaped(t *testing.T) {
	rec := &emit.Recorder{}
	b := &Bridge{emit: rec, env: config.Env{}, ttl: 60 * time.Millisecond, sessions: map[string]*entry{}, stop: make(chan struct{})}
	go b.reap()
	defer b.ServiceShutdown() //nolint:errcheck // test cleanup
	id, err := b.Open()
	if err != nil {
		t.Fatal(err)
	}
	waitTopic(t, rec, "lsp:"+id+":closed", id)
}
