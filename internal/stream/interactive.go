// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/httpx"
)

// closeWait bounds the close handshake of an interactive session.
const closeWait = 5 * time.Second

// Interactive is a WebSocket connection its caller drives one step at a
// time (the desktop's live session), dialed exactly like a scripted
// exchange. Send and Close steps are read from Steps; Receive steps are
// ignored, since every message received is reported as it arrives. The
// session ends after a Close step, when the server closes the connection
// or the connection fails, when a message goes past MaxBytes, or when
// the context given to DialInteractive ends.
type Interactive struct {
	// Request is the handshake request as sent.
	Request exchange.Request
	// Response is the handshake response. Its Stream records every
	// message sent and received; read it after Done is closed.
	Response *exchange.Response

	conn     *websocket.Conn
	raw      io.Closer
	opts     Options
	start    time.Time
	steps    chan Step
	done     chan struct{}
	stopRead context.CancelFunc
	readDone chan struct{}

	// obsMu keeps OnMessage calls serial and in order; mu guards the
	// fields below and Stream.Messages. OnMessage runs under obsMu only,
	// so it may call Err.
	obsMu    sync.Mutex
	mu       sync.Mutex
	received int64
	err      error
	// closing is set by a Close step: the close frame the server echoes
	// then ends the session normally.
	closing bool
}

// DialInteractive performs the handshake prepared by u (bounded by its
// max-time) and starts a session that lives until ctx ends or the
// session is closed. lim.OnMessage receives every message, sent and
// received, serially; it must not block for long, since the session can
// not end while it runs. lim.MaxBytes limits the bytes received, and
// lim.Count and lim.Timeout do not apply. A refused upgrade returns the
// handshake response in a session that is already done, with an error;
// a failed handshake returns no session. The stream's StopReason is set
// when the session ends without an error.
func DialInteractive(ctx context.Context, u *httpx.Upgrade, lim Options) (*Interactive, error) {
	lim = lim.withDefaults()
	// The handshake context must outlive the handshake: the upgraded
	// connection belongs to its request. Only the max-time cuts it short.
	hctx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(u.MaxTime(), cancel)
	conn, raw, r, err := dial(hctx, u)
	if !timer.Stop() { // max-time elapsed during the handshake
		if conn != nil {
			_ = conn.CloseNow()
		}
		cancel()
		return nil, u.Error(context.DeadlineExceeded)
	}
	if conn == nil {
		cancel()
		if err == nil {
			err = fmt.Errorf("the server refused the WebSocket upgrade (status %d)", r.Status)
		}
		if r == nil {
			return nil, err
		}
		done := make(chan struct{})
		close(done)
		return &Interactive{Request: u.Request, Response: r, done: done, err: err}, err
	}
	s := &Interactive{
		Request: u.Request, Response: r, conn: conn, raw: raw, opts: lim, start: time.Now(),
		steps: make(chan Step), done: make(chan struct{}), readDone: make(chan struct{}),
	}
	conn.SetReadLimit(lim.MaxBytes)
	readCtx, stopRead := context.WithCancel(context.Background())
	s.stopRead = stopRead
	go func() {
		defer close(s.readDone)
		s.readLoop(readCtx)
	}()
	go func() {
		defer cancel()
		s.run(hctx)
	}()
	return s, nil
}

// Steps is where the caller sends Send and Close steps. A send blocks
// until the previous step is done; select on Done not to block on an
// ended session.
func (s *Interactive) Steps() chan<- Step { return s.steps }

// Done is closed when the session has ended and released its connection.
func (s *Interactive) Done() <-chan struct{} { return s.done }

// Err is why the session ended: nil after a Close step or a normal
// closure by the server; otherwise the failure, the server's close code,
// or the context's error. It is final once Done is closed.
func (s *Interactive) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// fail records the first reason the session ends.
func (s *Interactive) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

// record appends m to the stream and reports it.
func (s *Interactive) record(m exchange.Message) {
	s.obsMu.Lock()
	defer s.obsMu.Unlock()
	s.mu.Lock()
	s.Response.Stream.Messages = append(s.Response.Stream.Messages, m)
	s.mu.Unlock()
	s.opts.observe(m)
}

func (s *Interactive) readLoop(ctx context.Context) {
	for {
		typ, data, err := s.conn.Read(ctx)
		if err != nil {
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			switch code := websocket.CloseStatus(err); {
			case closing || code == websocket.StatusNormalClosure:
			case code != -1:
				s.fail(fmt.Errorf("the server closed the connection (code %d)", int(code)))
			case ctx.Err() == nil:
				s.fail(fmt.Errorf("the connection failed: %w", err))
			}
			return
		}
		s.mu.Lock()
		s.received += int64(len(data))
		over := s.received > s.opts.MaxBytes
		s.mu.Unlock()
		if over {
			s.fail(fmt.Errorf("more than %d bytes received (sonde-stream-max-bytes)", s.opts.MaxBytes))
			return
		}
		s.record(exchange.Message{Binary: typ == websocket.MessageBinary, Data: data, At: time.Since(s.start)})
	}
}

// run applies the steps until the session ends, then releases the
// connection.
func (s *Interactive) run(ctx context.Context) {
	defer close(s.done)
	defer func() {
		_ = s.conn.CloseNow()
		s.stopRead()
		<-s.readDone
		if s.Err() == nil {
			s.Response.Stream.StopReason = exchange.StopScript
		}
	}()
	for {
		select {
		case <-ctx.Done():
			s.fail(ctx.Err())
			return
		case <-s.readDone:
			return
		case st := <-s.steps:
			switch st.Kind {
			case Send:
				if err := s.send(ctx, st); err != nil {
					s.fail(err)
					return
				}
			case Close:
				s.close(ctx, st)
				return
			}
		}
	}
}

func (s *Interactive) send(ctx context.Context, st Step) error {
	typ := websocket.MessageText
	if st.Binary {
		typ = websocket.MessageBinary
	}
	at := time.Since(s.start)
	if err := s.conn.Write(ctx, typ, st.Data); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("send: %w", err)
	}
	s.record(exchange.Message{Direction: exchange.Sent, Binary: st.Binary, Data: st.Data, At: at})
	return nil
}

// close runs the close handshake, cut short after closeWait or when ctx
// ends: dropping the read and the raw connection ends it at once.
func (s *Interactive) close(ctx context.Context, st Step) {
	code := websocket.StatusCode(st.Code)
	if st.Code == 0 {
		code = websocket.StatusNormalClosure
	}
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- s.conn.Close(code, "") }()
	t := time.NewTimer(closeWait)
	defer t.Stop()
	select {
	case err := <-done:
		if err != nil && websocket.CloseStatus(err) == -1 {
			s.fail(fmt.Errorf("close: %w", err))
		}
		return
	case <-ctx.Done():
	case <-t.C:
	}
	s.stopRead()
	if s.raw != nil {
		_ = s.raw.Close()
	}
	<-done
}
