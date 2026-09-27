// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// Default limits of a stream (sonde-stream-timeout, sonde-stream-max-bytes).
const (
	DefaultTimeout  = 10 * time.Second
	DefaultMaxBytes = 10 << 20
)

// Options bound a stream: zero fields take their defaults, Count zero
// means no count limit. OnMessage, when set, is called with each message
// as it is sent or received.
type Options struct {
	Count     int
	Timeout   time.Duration
	MaxBytes  int64
	OnMessage func(exchange.Message)
}

func (l Options) withDefaults() Options {
	if l.Timeout <= 0 {
		l.Timeout = DefaultTimeout
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = DefaultMaxBytes
	}
	return l
}

func (l Options) observe(m exchange.Message) {
	if l.OnMessage != nil {
		l.OnMessage(m)
	}
}

// ReadSSE reads an event stream from body until a limit is reached or the
// server ends it; each limit is a normal stop. stop must interrupt a
// pending body read (by canceling its request); read errors after stop are
// the stop, not failures. Only a read error the limits did not cause is
// returned as an error, with the events read so far.
func ReadSSE(body io.Reader, stop func(), lim Options) (*exchange.Stream, error) {
	lim = lim.withDefaults()
	start := time.Now()
	s := &exchange.Stream{Protocol: exchange.ProtocolSSE}
	var timedOut atomic.Bool
	timer := time.AfterFunc(lim.Timeout, func() {
		timedOut.Store(true)
		stop()
	})
	defer timer.Stop()

	var (
		read   int64
		parser SSEParser
		buf    = make([]byte, 32<<10)
	)
	for {
		n, err := body.Read(buf)
		chunk := buf[:n]
		full := false
		if rest := lim.MaxBytes - read; int64(n) >= rest {
			chunk, full = chunk[:rest], true
		}
		read += int64(len(chunk))
		at := time.Since(start)
		for _, e := range parser.Write(chunk) {
			m := exchange.Message{Data: []byte(e.Data), Event: e.Type, ID: e.ID, Retry: e.Retry, At: at}
			s.Messages = append(s.Messages, m)
			lim.observe(m)
			if lim.Count > 0 && len(s.Messages) == lim.Count {
				s.StopReason = exchange.StopCount
				stop()
				return s, nil
			}
		}
		switch {
		case full:
			s.StopReason = exchange.StopMaxBytes
			stop()
			return s, nil
		case err == nil:
			continue
		case timedOut.Load():
			s.StopReason = exchange.StopTimeout
			return s, nil
		case errors.Is(err, io.EOF):
			s.StopReason = exchange.StopClosed
			return s, nil
		}
		return s, err
	}
}

// SSEStream builds the stream of a body read whole: the sondeStream query
// of an entry without sonde-stream-* options.
func SSEStream(body []byte) *exchange.Stream {
	s := &exchange.Stream{Protocol: exchange.ProtocolSSE, StopReason: exchange.StopClosed}
	for _, e := range ParseSSE(body) {
		s.Messages = append(s.Messages, exchange.Message{Data: []byte(e.Data), Event: e.Type, ID: e.ID, Retry: e.Retry})
	}
	return s
}
