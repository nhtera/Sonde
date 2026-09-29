// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package emit sends app events to the frontend. Services depend on the
// Emitter interface; the window app sends through Wails events, server
// mode through its own guarded event stream (Wails' event socket is
// outside the server's auth checks), and tests record.
package emit

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Emitter sends an event to the frontend. Data must be a view DTO: it is
// sent as JSON as is, without further redaction.
type Emitter interface {
	Emit(topic string, data any)
}

// Wails emits through Wails events, to every window of the app.
type Wails struct{}

// Emit sends the event to every window.
func (Wails) Emit(topic string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(topic, data)
	}
}

// Event is one recorded or streamed event.
type Event struct {
	Topic string `json:"topic"`
	Data  any    `json:"data"`
}

// Recorder keeps every event, for tests.
type Recorder struct {
	mu     sync.Mutex
	events []Event
}

// Emit records the event.
func (r *Recorder) Emit(topic string, data any) {
	r.mu.Lock()
	r.events = append(r.events, Event{Topic: topic, Data: data})
	r.mu.Unlock()
}

// Events returns the events recorded so far.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

// StreamPath is where server mode serves its event stream; it is under
// the guard's token-checked app endpoints.
const StreamPath = "/_sonde/events"

// A page that lags this many events, or bytes, behind is dropped; it
// reconnects and resynchronizes (run events carry a sequence number).
const (
	clientBuffer = 4096
	clientBytes  = 32 << 20
)

// client is one connected page.
type client struct {
	ch      chan []byte
	pending int64 // bytes queued, guarded by Stream.mu
}

// Stream is server mode's emitter: an http.Handler (mounted at StreamPath)
// that streams each event as a line of JSON to every connected page.
type Stream struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

// NewStream returns a stream with no clients.
func NewStream() *Stream { return &Stream{clients: map[*client]struct{}{}} }

// Emit sends the event to every connected page. A page too slow to keep
// up is disconnected rather than allowed to block the app.
func (s *Stream) Emit(topic string, data any) {
	line, err := json.Marshal(Event{Topic: topic, Data: data})
	if err != nil {
		return
	}
	line = append(line, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if c.pending+int64(len(line)) > clientBytes {
			s.drop(c)
			continue
		}
		select {
		case c.ch <- line:
			c.pending += int64(len(line))
		default:
			s.drop(c)
		}
	}
}

// drop disconnects c (s.mu held).
func (s *Stream) drop(c *client) {
	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)
		close(c.ch)
	}
}

// ServeHTTP streams events to one page until it goes away.
func (s *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	rc := http.NewResponseController(w)
	// The server's write timeout would end the stream.
	_ = rc.SetWriteDeadline(time.Time{})
	c := &client{ch: make(chan []byte, clientBuffer)}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.drop(c)
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case line, ok := <-c.ch:
			if !ok {
				return // dropped as too slow
			}
			s.mu.Lock()
			c.pending -= int64(len(line))
			s.mu.Unlock()
			if _, err := w.Write(line); err != nil {
				return
			}
			_ = rc.Flush()
		}
	}
}

// Clients reports how many pages are connected.
func (s *Stream) Clients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}
