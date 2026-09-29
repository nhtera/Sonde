// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"encoding/base64"
	"sync"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/report"
)

// Converter turns one job's engine events into DTOs. Call Handle from the
// job's OnEvent callback: each DTO is built and redacted there, then
// passed to out.
//
// Redaction uses the unit's live redactor (from the unit-started host
// event), which learns each secret as the run captures it. An attempt with
// a redacted capture is held: its own logs and messages may print the
// value before the capture makes it secret, so they are converted only
// when the attempt finishes. Events before the unit starts are held the
// same way.
type Converter struct {
	file     string
	fallback Redactor
	bodies   BodyStore
	out      func(dto any)

	mu      sync.Mutex
	redact  Redactor
	entry   int  // the running entry; 0 between entries
	holding bool // hold the events of the current attempt
	held    []heldEvent
}

// heldEvent is an event waiting for its redactor, with the entry it
// arrived in.
type heldEvent struct {
	ev    engine.Event
	entry int
}

// NewConverter returns a converter for the job running file. fallback
// redacts before the unit-started event (normally never needed; pass the
// runner's Redact); bodies stores response bodies (nil: no body URLs).
func NewConverter(file string, fallback Redactor, bodies BodyStore, out func(dto any)) *Converter {
	return &Converter{file: file, fallback: fallback, bodies: bodies, out: out}
}

// Handle converts one engine event.
func (c *Converter) Handle(ev engine.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := enginex.UnitStarted(ev); ok {
		c.redact = r
		c.flush()
		return
	}
	if index, ok := enginex.EntryRedacts(ev); ok {
		if index == c.entry {
			c.holding = true
		}
		return
	}
	if e, ok := ev.(engine.EntryStarted); ok {
		c.entry = e.Index
	}
	_, finished := ev.(engine.EntryFinished)
	if c.redact == nil || c.holding {
		c.held = append(c.held, heldEvent{ev, c.entry})
		if finished && c.redact != nil {
			c.holding = false
			c.flush()
		}
	} else {
		c.emit(ev, c.entry)
	}
	if finished {
		c.entry = 0 // logs up to the next start head the next entry
	}
}

// Flush converts any held events (at the end of the job).
func (c *Converter) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.holding = false
	c.flush()
}

func (c *Converter) flush() {
	held := c.held
	c.held = nil
	for _, h := range held {
		c.emit(h.ev, h.entry)
	}
}

func (c *Converter) redactor() Redactor {
	if c.redact != nil {
		return c.redact
	}
	return c.fallback
}

// emit converts ev, which arrived during entry, and passes it on; events
// without a DTO are dropped.
func (c *Converter) emit(ev engine.Event, entry int) {
	if dto, ok := c.convert(ev, entry); ok {
		c.out(dto)
	}
}

// convert builds the DTO of ev, redacted.
func (c *Converter) convert(ev engine.Event, entry int) (any, bool) {
	redact := c.redactor()
	if index, call, req, ok := enginex.RequestSent(ev); ok {
		return RequestSent{Type: TypeRequestSent, Entry: index, Call: call, Request: request(c.file, req, redact)}, true
	}
	if index, reason, ok := enginex.EntrySkipped(ev); ok {
		return EntrySkipped{Type: TypeEntrySkipped, Entry: index, Reason: reason}, true
	}
	switch e := ev.(type) {
	case engine.EntryStarted:
		return EntryStarted{Type: TypeEntryStarted, Entry: e.Index, Retry: e.Retry, Last: e.Last}, true
	case engine.Log:
		return Log{Type: TypeLog, Entry: entry, Level: levels[e.Level], Text: redact(e.Text)}, true
	case engine.MessageSent:
		return Message{Type: TypeMessage, Entry: e.Index, Message: message(e.Message, redact)}, true
	case engine.MessageReceived:
		return Message{Type: TypeMessage, Entry: e.Index, Message: message(e.Message, redact)}, true
	case engine.EntryFinished:
		if e.Result == nil {
			return nil, false
		}
		return EntryFinished{Type: TypeEntryFinished, Entry: ConvertEntry(c.file, e.Result, redact, c.bodies)}, true
	}
	return nil, false
}

// request projects a request as the report does.
func request(file string, req exchange.Request, redact Redactor) report.Request {
	unit := &engine.UnitResult{File: file, Entries: []*engine.EntryResult{{Calls: []engine.Call{{Request: req}}}}}
	r, err := report.JSON(unit, redact, nil)
	if err != nil || len(r.Entries) == 0 || len(r.Entries[0].Calls) == 0 {
		return report.Request{Method: req.Method, URL: redact(req.URL)}
	}
	return r.Entries[0].Calls[0].Request
}

// message projects a stream message as the report does: binary data is
// redacted, then base64-encoded.
func message(m exchange.Message, redact Redactor) report.StreamMessage {
	out := report.StreamMessage{
		Binary: m.Binary, Direction: m.Direction.String(),
		Event: redact(m.Event), ID: redact(m.ID), Retry: m.Retry, Time: m.At.Milliseconds(),
	}
	if m.Binary {
		out.Data = base64.StdEncoding.EncodeToString(RedactBytes(m.Data, redact))
	} else {
		out.Data = redact(string(m.Data))
	}
	return out
}

// RedactBytes masks every secret in b, text or binary alike.
func RedactBytes(b []byte, redact Redactor) []byte {
	return []byte(redact(string(b)))
}
