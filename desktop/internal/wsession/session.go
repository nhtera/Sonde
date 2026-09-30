// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package wsession runs interactive WebSocket sessions for the page: an
// entry of a request file dialed exactly as a run of the file would dial
// it (proxy, TLS, cookies, host policy, options), then driven one message
// at a time. A session is not part of any run: it is never recorded in
// history and its messages reach the page only as redacted DTOs, through
// the session's live redactor.
package wsession

import (
	"context"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/view"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/report"
	"github.com/nhtera/sonde/internal/stream"
)

// TopicPrefix + a session id is the topic of the session's events.
const TopicPrefix = "wsession:"

// Event types.
const (
	TypeMessage = "message" // a message sent or received
	TypeClosed  = "closed"  // the session ended; Error says why, "" when closed normally
)

// Event is what a session sends the page.
type Event struct {
	Type    string                `json:"type"`
	Message *report.StreamMessage `json:"message,omitempty"`
	Error   string                `json:"error,omitempty"`
}

// OpenRequest opens a session for entry Entry of a file: the page
// subscribes to "wsession:<SessionID>" first.
type OpenRequest struct {
	SessionID string `json:"sessionId"`
	File      string `json:"file"`
	// Source is the tab's text: the buffer is what is dialed.
	Source string `json:"source"`
	Env    string `json:"env"`
	Entry  int    `json:"entry"`
}

// Opened is an open session.
type Opened struct {
	// URL is the handshake request's URL, redacted.
	URL    string `json:"url"`
	Status int    `json:"status"`
}

// Preparer plans a file's run without running it.
type Preparer func(ctx context.Context, file, source, env string) (*engine.Runner, engine.Job, error)

// Sessions is the open sessions.
type Sessions struct {
	emit    emit.Emitter
	prepare Preparer

	mu   sync.Mutex
	open map[string]*session
}

// session is an open session, or one being dialed (ia nil): its id is
// reserved from the start, so a Close or CloseAll during the dial ends it.
type session struct {
	ia     *stream.Interactive
	cancel context.CancelFunc
}

// New returns the session service's core.
func New(e emit.Emitter, prepare Preparer) *Sessions {
	return &Sessions{emit: e, prepare: prepare, open: map[string]*session{}}
}

// Open dials the entry and starts the session. It lives until Close, the
// server or the connection ends it, or CloseAll; one of them during the
// dial cancels it.
func (m *Sessions) Open(ctx context.Context, req OpenRequest) (*Opened, error) {
	if req.SessionID == "" || strings.ContainsAny(req.SessionID, ":/ ") {
		return nil, apperr.New(apperr.Invalid, "a session needs an id")
	}
	// The session outlives this call: it has its own context.
	sctx, cancel := context.WithCancel(context.Background())
	s := &session{cancel: cancel}
	m.mu.Lock()
	if _, taken := m.open[req.SessionID]; taken {
		m.mu.Unlock()
		cancel()
		return nil, apperr.New(apperr.Busy, "that session is already open")
	}
	m.open[req.SessionID] = s
	m.mu.Unlock()
	fail := func(err error) (*Opened, error) {
		cancel()
		m.drop(req.SessionID, s)
		return nil, err
	}
	runner, job, err := m.prepare(ctx, req.File, req.Source, req.Env)
	if err != nil {
		return fail(err)
	}
	topic := TopicPrefix + req.SessionID
	// Messages may arrive before the dial returns the redactor: they wait
	// for it (the read loop only), and are dropped when the dial failed.
	var (
		redact func(string) string
		ready  = make(chan struct{})
	)
	onMessage := func(msg exchange.Message) {
		<-ready
		if redact == nil {
			return
		}
		sm := view.StreamMessage(msg, redact)
		m.emit.Emit(topic, Event{Type: TypeMessage, Message: &sm})
	}
	ia, r, err := enginex.DialWS(sctx, runner, job, req.Entry, onMessage)
	redact = r
	close(ready)
	if err != nil {
		if redact == nil {
			redact = runner.Redact
		}
		return fail(apperr.New(apperr.Invalid, redact(err.Error())))
	}
	// Closed while dialing: the session ends at once.
	m.mu.Lock()
	kept := m.open[req.SessionID] == s && sctx.Err() == nil
	if kept {
		s.ia = ia
	}
	m.mu.Unlock()
	if !kept {
		cancel()
		<-ia.Done()
		return nil, apperr.New(apperr.Expired, "the session was closed while it opened")
	}
	go func() {
		<-ia.Done()
		ev := Event{Type: TypeClosed}
		if err := ia.Err(); err != nil && sctx.Err() == nil {
			ev.Error = redact(err.Error())
		}
		m.drop(req.SessionID, s)
		cancel()
		m.emit.Emit(topic, ev)
	}()
	return &Opened{URL: redact(ia.Request.URL), Status: ia.Response.Status}, nil
}

// drop forgets session s of id (if id is still s).
func (m *Sessions) drop(id string, s *session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.open[id] == s {
		delete(m.open, id)
	}
}

// get returns an open session (not one still dialing).
func (m *Sessions) get(id string) (*stream.Interactive, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.open[id]
	if !ok || s.ia == nil {
		return nil, apperr.New(apperr.NotFound, "the session is not open")
	}
	return s.ia, nil
}

// Send sends a message: text, or binary as hex digits ("01 ff").
func (m *Sessions) Send(id, data string, binary bool) error {
	ia, err := m.get(id)
	if err != nil {
		return err
	}
	step := stream.Step{Kind: stream.Send, Data: []byte(data)}
	if binary {
		b, err := hex.DecodeString(strings.Join(strings.Fields(data), ""))
		if err != nil {
			return apperr.New(apperr.Invalid, "binary messages are hex digits, e.g. 01 ff")
		}
		step.Data, step.Binary = b, true
	}
	select {
	case ia.Steps() <- step:
		return nil
	case <-ia.Done():
		return apperr.New(apperr.NotFound, "the session has ended")
	}
}

// Close closes a session normally (its closed event follows); a session
// still dialing is canceled.
func (m *Sessions) Close(id string) {
	m.mu.Lock()
	s, ok := m.open[id]
	var ia *stream.Interactive
	if ok {
		ia = s.ia
		if ia == nil {
			delete(m.open, id)
		}
	}
	m.mu.Unlock()
	switch {
	case !ok:
	case ia == nil:
		s.cancel()
	default:
		select {
		case ia.Steps() <- stream.Step{Kind: stream.Close}:
		case <-ia.Done():
		}
	}
}

// CloseAll ends every session at once, those dialing too (another
// project was opened, the app quits).
func (m *Sessions) CloseAll() {
	type ending struct {
		cancel context.CancelFunc
		ia     *stream.Interactive
	}
	m.mu.Lock()
	all := make([]ending, 0, len(m.open))
	for id, s := range m.open {
		all = append(all, ending{s.cancel, s.ia})
		delete(m.open, id)
	}
	m.mu.Unlock()
	for _, e := range all {
		e.cancel()
		if e.ia != nil {
			<-e.ia.Done()
		}
	}
}

// Service is the session bindings.
type Service struct{ m *Sessions }

// NewService returns the bindings over m.
func NewService(m *Sessions) *Service { return &Service{m: m} }

// Open dials an entry and starts a session.
func (s *Service) Open(ctx context.Context, req OpenRequest) (*Opened, error) {
	return s.m.Open(ctx, req)
}

// Send sends a message on a session (binary: hex digits).
func (s *Service) Send(id, data string, binary bool) error { return s.m.Send(id, data, binary) }

// Close closes a session.
func (s *Service) Close(id string) { s.m.Close(id) }

// ServiceShutdown ends every session when the app quits.
func (s *Service) ServiceShutdown() error {
	s.m.CloseAll()
	return nil
}
