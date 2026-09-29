// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lspbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/lsp"
)

// Heartbeat is how long a session lives without a Send or Ping: a page
// that reloaded or closed stops pinging, and its server is stopped. A
// hidden browser tab may run its timers only once a minute, so it is
// several minutes; a page whose session ended opens a new one.
const Heartbeat = 3 * time.Minute

// Bridge is the language server bindings: one server per page session.
// Server messages arrive on "lsp:<id>" (the JSON text), and "lsp:<id>:closed"
// once a session ends.
type Bridge struct {
	emit    emit.Emitter
	env     config.Env
	version string
	ttl     time.Duration

	mu       sync.Mutex
	sessions map[string]*entry
	stop     chan struct{}
	once     sync.Once
}

type entry struct {
	s    *Session
	seen time.Time
}

// NewBridge returns the bridge; env is the app's process environment.
func NewBridge(e emit.Emitter, env config.Env, version string) *Bridge {
	b := &Bridge{emit: e, env: env, version: version, ttl: Heartbeat, sessions: map[string]*entry{}, stop: make(chan struct{})}
	go b.reap()
	return b
}

// Open starts a language server for the calling page and returns the
// session id.
func (b *Bridge) Open() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	topic := "lsp:" + id
	s, err := Start(context.Background(), lsp.Options{Version: b.version, Environ: b.env}, func(msg []byte) {
		b.emit.Emit(topic, string(msg))
	})
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.sessions[id] = &entry{s: s, seen: time.Now()}
	b.mu.Unlock()
	go func() {
		<-s.Done()
		b.mu.Lock()
		delete(b.sessions, id)
		b.mu.Unlock()
		b.emit.Emit(topic+":closed", id)
	}()
	return id, nil
}

// Send passes a client message (JSON) to session id.
func (b *Bridge) Send(id, msg string) error {
	s, err := b.touch(id)
	if err != nil {
		return err
	}
	if err := s.Send([]byte(msg)); err != nil {
		return apperr.Wrap(apperr.Expired, err)
	}
	return nil
}

// Ping keeps session id alive.
func (b *Bridge) Ping(id string) error {
	_, err := b.touch(id)
	return err
}

// Configure sets the session's environment and the names it treats as
// defined beyond the files (captures, data columns, overrides).
func (b *Bridge) Configure(id, env string, extraVariables []string) error {
	if extraVariables == nil {
		extraVariables = []string{}
	}
	settings := map[string]any{"sonde": map[string]any{"env": env, "extraVariables": extraVariables}}
	if env == "" {
		settings["sonde"].(map[string]any)["env"] = nil
	}
	msg, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "workspace/didChangeConfiguration",
		"params": map[string]any{"settings": settings},
	})
	if err != nil {
		return err
	}
	return b.Send(id, string(msg))
}

// Close stops session id.
func (b *Bridge) Close(id string) {
	b.mu.Lock()
	e := b.sessions[id]
	delete(b.sessions, id)
	b.mu.Unlock()
	if e != nil {
		_ = e.s.Close()
	}
}

// ServiceShutdown stops every session when the app quits.
func (b *Bridge) ServiceShutdown() error {
	b.once.Do(func() { close(b.stop) })
	b.mu.Lock()
	all := b.sessions
	b.sessions = map[string]*entry{}
	b.mu.Unlock()
	for _, e := range all {
		_ = e.s.Close()
	}
	return nil
}

func (b *Bridge) touch(id string) (*Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.sessions[id]
	if e == nil {
		return nil, apperr.New(apperr.Expired, "the language server session ended; open a new one")
	}
	e.seen = time.Now()
	return e.s, nil
}

// reap stops sessions whose page stopped pinging.
func (b *Bridge) reap() {
	t := time.NewTicker(b.ttl / 6)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case now := <-t.C:
			b.mu.Lock()
			var idle []*Session
			for id, e := range b.sessions {
				if now.Sub(e.seen) > b.ttl {
					idle = append(idle, e.s)
					delete(b.sessions, id)
				}
			}
			b.mu.Unlock()
			for _, s := range idle {
				_ = s.Close()
			}
		}
	}
}
