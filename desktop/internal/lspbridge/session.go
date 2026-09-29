// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package lspbridge runs the language server in process, one server per
// frontend session: the editor's client sends and receives bare JSON
// messages, and a session frames them for the server over pipes. A page
// reload opens a new session, so the new client's initialize meets a new
// server.
package lspbridge

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/nhtera/sonde/internal/lsp"
)

// maxMessage bounds one server message.
const maxMessage = 64 << 20

// ErrClosed is returned by Send after the session ended.
var ErrClosed = errors.New("lspbridge: session closed")

// Session is one language server serving one client.
type Session struct {
	mu     sync.Mutex // guards closed and serializes writes to in
	in     *io.PipeWriter
	out    *io.PipeReader
	closed bool

	cancel  context.CancelFunc
	stopped chan struct{} // the server returned
	done    chan struct{} // the server returned and the last message was delivered
	err     error
}

// Start runs a server with opt until ctx ends or Close. onMessage gets each
// server message (JSON, no headers) in order, from one goroutine.
func Start(ctx context.Context, opt lsp.Options, onMessage func(msg []byte)) (*Session, error) {
	srv, err := lsp.NewServer(opt)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &Session{in: inW, out: outR, cancel: cancel, stopped: make(chan struct{}), done: make(chan struct{})}
	go func() {
		err := srv.Run(ctx, inR, outW)
		_ = outW.CloseWithError(io.EOF)
		_ = inR.CloseWithError(ErrClosed)
		s.mu.Lock()
		s.closed = true
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrClosed) {
			s.err = err
		}
		s.mu.Unlock()
		close(s.stopped)
	}()
	go func() {
		r := bufio.NewReader(outR)
		for {
			msg, err := readFrame(r)
			if err != nil {
				_ = outR.CloseWithError(err)
				break
			}
			onMessage(msg)
		}
		<-s.stopped
		cancel()
		close(s.done)
	}()
	return s, nil
}

// Send passes one client message (JSON, no headers) to the server.
func (s *Session) Send(msg []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	frame := make([]byte, 0, len(msg)+32)
	frame = append(frame, "Content-Length: "...)
	frame = strconv.AppendInt(frame, int64(len(msg)), 10)
	frame = append(frame, "\r\n\r\n"...)
	frame = append(frame, msg...)
	if _, err := s.in.Write(frame); err != nil {
		return ErrClosed
	}
	return nil
}

// Close ends the session and waits for the server to stop. It breaks both
// pipes first, so neither a Send blocked on the server nor a server blocked
// on a slow onMessage holds it up; a message being delivered may still
// finish after Close returns.
func (s *Session) Close() error {
	_ = s.in.CloseWithError(ErrClosed)
	_ = s.out.CloseWithError(ErrClosed)
	s.cancel()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	<-s.stopped
	return s.Err()
}

// Done is closed once the server stopped and its last message was
// delivered.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err is why the server stopped, nil after a clean end.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// readFrame reads one Content-Length framed message.
func readFrame(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 || n > maxMessage {
				return nil, fmt.Errorf("lspbridge: bad Content-Length %q", value)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, errors.New("lspbridge: message without Content-Length")
	}
	msg := make([]byte, length)
	if _, err := io.ReadFull(r, msg); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(msg), nil
}
