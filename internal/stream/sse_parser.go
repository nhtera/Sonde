// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package stream reads streaming responses: Server-Sent Events and
// WebSocket message scripts (docs/decisions/0004-streaming-protocols.md).
package stream

import (
	"bytes"
	"strconv"
	"strings"
)

// Event is one dispatched Server-Sent Event.
type Event struct {
	Type  string // "message" when the event has no event field
	Data  string
	ID    string // the last event ID at dispatch
	Retry *int   // this event's valid retry field, if any
}

// SSEParser parses a text/event-stream incrementally, following the WHATWG
// HTML "event stream interpretation": one leading BOM is stripped, CRLF, LF
// and CR end lines, data lines join with LF, an id containing NUL and a
// retry that is not all ASCII digits are ignored, and an event without data
// is not dispatched. A partial event at the end of the stream is dropped.
// The zero value is ready to use.
type SSEParser struct {
	line     []byte // the incomplete line so far
	started  bool   // past the optional BOM
	skipLF   bool   // the previous line ended with CR
	data     strings.Builder
	hasData  bool
	event    string
	lastID   string
	retry    *int
	dispatch []Event
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Write feeds b and returns the events it completed.
func (p *SSEParser) Write(b []byte) []Event {
	p.dispatch = nil
	if !p.started {
		p.line = append(p.line, b...)
		if len(p.line) < len(utf8BOM) && bytes.HasPrefix(utf8BOM, p.line) {
			return nil // could still be a BOM
		}
		p.started = true
		b = bytes.TrimPrefix(p.line, utf8BOM)
		p.line = nil
	}
	for len(b) > 0 {
		if p.skipLF {
			p.skipLF = false
			if b[0] == '\n' {
				b = b[1:]
				continue
			}
		}
		i := bytes.IndexAny(b, "\r\n")
		if i < 0 {
			p.line = append(p.line, b...)
			break
		}
		p.line = append(p.line, b[:i]...)
		p.skipLF = b[i] == '\r'
		p.processLine(string(bytes.ToValidUTF8(p.line, []byte("\uFFFD"))))
		p.line = p.line[:0]
		b = b[i+1:]
	}
	return p.dispatch
}

func (p *SSEParser) processLine(line string) {
	if line == "" {
		p.dispatchEvent()
		return
	}
	if line[0] == ':' {
		return // comment
	}
	field, value, found := strings.Cut(line, ":")
	if found {
		value = strings.TrimPrefix(value, " ")
	}
	switch field {
	case "event":
		p.event = value
	case "data":
		p.data.WriteString(value)
		p.data.WriteByte('\n')
		p.hasData = true
	case "id":
		if !strings.ContainsRune(value, 0) {
			p.lastID = value
		}
	case "retry":
		if value != "" && strings.Trim(value, "0123456789") == "" {
			if n, err := strconv.Atoi(value); err == nil {
				p.retry = &n
			}
		}
	}
}

func (p *SSEParser) dispatchEvent() {
	defer func() {
		p.data.Reset()
		p.hasData, p.event, p.retry = false, "", nil
	}()
	if !p.hasData {
		return
	}
	typ := p.event
	if typ == "" {
		typ = "message"
	}
	p.dispatch = append(p.dispatch, Event{
		Type:  typ,
		Data:  strings.TrimSuffix(p.data.String(), "\n"),
		ID:    p.lastID,
		Retry: p.retry,
	})
}

// ParseSSE parses a whole event stream.
func ParseSSE(b []byte) []Event {
	var p SSEParser
	return p.Write(b)
}
