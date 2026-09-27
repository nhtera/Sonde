// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package exchange

import "time"

// Protocol is the protocol of a stream. More may be added in a minor
// release: switch on it with a default case.
type Protocol string

// Stream protocols.
const (
	ProtocolSSE       Protocol = "sse"
	ProtocolWebSocket Protocol = "websocket"
)

// StopReason tells why a stream ended normally. More may be added in a
// minor release.
type StopReason string

// Reasons a stream stopped.
const (
	StopCount    StopReason = "count"     // sonde-stream-count events were read
	StopTimeout  StopReason = "timeout"   // sonde-stream-timeout elapsed
	StopMaxBytes StopReason = "max-bytes" // sonde-stream-max-bytes bytes were read
	StopClosed   StopReason = "closed"    // the server ended the stream
	StopScript   StopReason = "script"    // every [SondeMessages] step ran
)

// Stream is what a streamed entry exchanged after the response headers:
// Server-Sent Events, or WebSocket messages.
type Stream struct {
	Protocol Protocol
	// StopReason is empty when the stream failed: the messages are then
	// those exchanged before the failure.
	StopReason StopReason
	// Messages are the SSE events, or the WebSocket messages sent and
	// received, in order.
	Messages []Message
}

// Direction tells whether a message was sent or received.
type Direction int

// Message directions.
const (
	Received Direction = iota
	Sent
)

func (d Direction) String() string {
	if d == Sent {
		return "sent"
	}
	return "received"
}

// Message is one SSE event or WebSocket message.
type Message struct {
	Direction Direction
	// Binary is set for a WebSocket binary message.
	Binary bool
	Data   []byte
	// Event is the SSE event type ("message" when the event has none);
	// ID the last event ID; Retry the event's retry field, when valid.
	Event string
	ID    string
	Retry *int
	// At is the time since the response headers arrived.
	At time.Duration
}

// ReceivedMessages returns the messages received, in order.
func (s *Stream) ReceivedMessages() []Message {
	if s == nil {
		return nil
	}
	var out []Message
	for _, m := range s.Messages {
		if m.Direction == Received {
			out = append(out, m)
		}
	}
	return out
}
