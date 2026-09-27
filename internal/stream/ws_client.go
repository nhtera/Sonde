// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/httpx"
)

// StepKind is the kind of a WebSocket script step.
type StepKind int

// Step kinds.
const (
	Send StepKind = iota
	Receive
	Close
)

// Step is one rendered [SondeMessages] step.
type Step struct {
	Kind StepKind
	// Send: the message, binary or text.
	Data   []byte
	Binary bool
	// Receive: how many messages to wait for (at least 1).
	Count int
	// Close: the status code, 0 for 1000.
	Code int
}

// WebSocket performs the handshake prepared by u and runs steps, then
// closes the connection normally unless a Close step did. The response is
// the handshake response; it has a Stream when the upgrade succeeded. A
// refused upgrade (any status but 101) is not an error: the steps do not
// run and the status asserts report it. The whole exchange after the
// handshake must end within lim.Timeout; a Receive still waiting then fails.
// On a failure the response keeps the messages exchanged so far.
func WebSocket(ctx context.Context, u *httpx.Upgrade, steps []Step, lim Options) (*exchange.Response, error) {
	lim = lim.withDefaults()
	ctx, cancel := context.WithTimeout(ctx, u.MaxTime())
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, u.URL, &websocket.DialOptions{
		HTTPClient: u.Client, HTTPHeader: u.Header, Host: u.Host,
		CompressionMode: websocket.CompressionDisabled,
	})
	if resp == nil {
		return nil, u.Error(err)
	}
	if err != nil {
		r := u.Response(resp)
		if resp.StatusCode == http.StatusSwitchingProtocols {
			return r, fmt.Errorf("invalid WebSocket handshake: %w", err)
		}
		return r, nil
	}
	r := u.Response(resp)
	r.Stream = &exchange.Stream{Protocol: exchange.ProtocolWebSocket}
	s := &session{conn: conn, stream: r.Stream, start: time.Now(), deadline: time.Now().Add(lim.Timeout), opts: lim, notify: make(chan struct{}, 1)}
	conn.SetReadLimit(lim.MaxBytes)
	readCtx, stopRead := context.WithCancel(context.Background())
	s.stopRead = stopRead
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.readLoop(readCtx)
	}()
	defer func() {
		_ = conn.CloseNow()
		stopRead()
		<-done
	}()
	if err := s.run(ctx, steps); err != nil {
		return r, err
	}
	r.Stream.StopReason = exchange.StopScript
	return r, nil
}

// StepError is the failure of a step: Index is its position in the steps.
type StepError struct {
	Index int
	Err   error
}

func (e *StepError) Error() string { return e.Err.Error() }

func (e *StepError) Unwrap() error { return e.Err }

// session is one WebSocket exchange: the steps run on the caller's
// goroutine while readLoop queues incoming messages.
type session struct {
	conn     *websocket.Conn
	stream   *exchange.Stream
	start    time.Time
	deadline time.Time
	opts     Options
	// stopRead cancels readLoop's context, which drops the connection.
	stopRead context.CancelFunc

	mu       sync.Mutex
	queue    []exchange.Message
	received int64 // bytes of every message received, taken or not
	readErr  error
	notify   chan struct{}
	closed   bool // a Close step ran
}

func (s *session) readLoop(ctx context.Context) {
	for {
		typ, data, err := s.conn.Read(ctx)
		s.mu.Lock()
		if err != nil {
			s.readErr = err
		} else {
			s.received += int64(len(data))
			if s.received > s.opts.MaxBytes {
				s.readErr = fmt.Errorf("more than %d bytes received (sonde-stream-max-bytes)", s.opts.MaxBytes)
				err = s.readErr
			} else {
				s.queue = append(s.queue, exchange.Message{Binary: typ == websocket.MessageBinary, Data: data, At: time.Since(s.start)})
			}
		}
		s.mu.Unlock()
		select {
		case s.notify <- struct{}{}:
		default:
		}
		if err != nil {
			return
		}
	}
}

func (s *session) run(ctx context.Context, steps []Step) error {
	ctx, cancel := context.WithDeadline(ctx, s.deadline)
	defer cancel()
	for i, st := range steps {
		var err error
		switch st.Kind {
		case Send:
			err = s.send(ctx, st)
		case Receive:
			err = s.receive(ctx, st)
		case Close:
			err = s.close(ctx, st)
		}
		if err != nil {
			return &StepError{Index: i, Err: err}
		}
	}
	if !s.closed {
		_ = s.closeConn(ctx, websocket.StatusNormalClosure)
	}
	return nil
}

// closeConn runs the close handshake, cut short when ctx ends: the
// library alone would wait up to 10 seconds for an unresponsive server.
// Canceling readLoop's pending read drops the connection, which ends the
// handshake at once (CloseNow can not: the handshake already holds it).
func (s *session) closeConn(ctx context.Context, code websocket.StatusCode) error {
	done := make(chan error, 1)
	go func() { done <- s.conn.Close(code, "") }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		s.stopRead()
		<-done
		return s.cause(ctx, ctx.Err())
	}
}

func (s *session) send(ctx context.Context, st Step) error {
	typ := websocket.MessageText
	if st.Binary {
		typ = websocket.MessageBinary
	}
	m := exchange.Message{Direction: exchange.Sent, Binary: st.Binary, Data: st.Data, At: time.Since(s.start)}
	if err := s.conn.Write(ctx, typ, st.Data); err != nil {
		return s.cause(ctx, err)
	}
	s.stream.Messages = append(s.stream.Messages, m)
	s.opts.observe(m)
	return nil
}

func (s *session) receive(ctx context.Context, st Step) error {
	n := max(st.Count, 1)
	for i := 1; i <= n; i++ {
		for {
			s.mu.Lock()
			if len(s.queue) > 0 {
				m := s.queue[0]
				s.queue = s.queue[1:]
				s.mu.Unlock()
				s.stream.Messages = append(s.stream.Messages, m)
				s.opts.observe(m)
				break
			}
			readErr := s.readErr
			s.mu.Unlock()
			where := fmt.Sprintf("message %d of %d", i, n)
			if readErr != nil {
				if code := websocket.CloseStatus(readErr); code != -1 {
					return fmt.Errorf("the server closed the connection (code %d) while waiting for %s", int(code), where)
				}
				return fmt.Errorf("the connection failed while waiting for %s: %w", where, readErr)
			}
			select {
			case <-s.notify:
			case <-ctx.Done():
				return fmt.Errorf("%w waiting for %s", s.cause(ctx, ctx.Err()), where)
			}
		}
	}
	return nil
}

func (s *session) close(ctx context.Context, st Step) error {
	code := websocket.StatusCode(st.Code)
	if st.Code == 0 {
		code = websocket.StatusNormalClosure
	}
	s.closed = true
	if err := s.closeConn(ctx, code); err != nil && websocket.CloseStatus(err) == -1 {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// errStreamTimeout and errMaxTime name why the exchange's context ended.
var (
	errStreamTimeout = errors.New("timeout")
	errMaxTime       = errors.New("timeout (max-time)")
)

// cause names a timeout of the exchange (sonde-stream-timeout) or of the
// entry (max-time) instead of the error it caused.
func (s *session) cause(ctx context.Context, err error) error {
	switch {
	case ctx.Err() == nil:
		return err
	case !time.Now().Before(s.deadline):
		return errStreamTimeout
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return errMaxTime
	}
	return ctx.Err()
}
