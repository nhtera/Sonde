// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package mocksvc runs the project's OpenAPI mock (the spec of sonde.yaml's
// openapi:) on 127.0.0.1 only, streams its request log on "mock:log", and
// reports which operations the project's runs covered this session. While
// the mock runs, base_url points at it (an override of every run).
package mocksvc

import (
	"bytes"
	"context"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/mock"
	"github.com/nhtera/sonde/internal/openapi"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Events.
const (
	TopicLog    = "mock:log"    // data: one log line
	TopicStatus = "mock:status" // data: Status
)

// Spec is the project's contract.
type Spec struct {
	// File is the spec, project-relative; "" when sonde.yaml has none.
	File       string   `json:"file"`
	Operations []string `json:"operations"`
}

// Status is the mock's state.
type Status struct {
	Running bool   `json:"running"`
	URL     string `json:"url"`
}

// Coverage is the operations the session's runs called.
type Coverage struct {
	Total   int      `json:"total"`
	Covered []string `json:"covered"`
}

// Mocks is the mock service's core.
type Mocks struct {
	emit    func(topic string, data any)
	project func() *sandbox.Root
	setMock func(url string) // the base_url override

	mu      sync.Mutex
	spec    *openapi.Spec
	specDir string
	stop    chan struct{}
	done    chan struct{}
	url     string
	covered map[string]bool
}

// New returns the mock service.
func New(emit func(string, any), project func() *sandbox.Root, setMock func(string)) *Mocks {
	return &Mocks{emit: emit, project: project, setMock: setMock, covered: map[string]bool{}}
}

// load returns the project's spec (cached per project).
func (m *Mocks) load(ctx context.Context) (*openapi.Spec, string, error) {
	root := m.project()
	if root == nil {
		return nil, "", apperr.New(apperr.NotFound, "no project is open")
	}
	var p *config.Project
	for _, name := range []string{"sonde.yaml", "sonde.yml"} {
		if _, err := root.Stat(name); err == nil {
			loaded, err := config.LoadProject(filepath.Join(root.Dir(), name))
			if err != nil {
				return nil, "", apperr.Wrap(apperr.Invalid, err)
			}
			p = loaded
			break
		}
	}
	if p == nil || p.OpenAPI == nil || p.OpenAPI.Spec == "" {
		return nil, "", nil
	}
	m.mu.Lock()
	if m.spec != nil && m.specDir == p.OpenAPI.Spec {
		s := m.spec
		m.mu.Unlock()
		return s, p.OpenAPI.Spec, nil
	}
	m.mu.Unlock()
	spec, err := openapi.Load(ctx, p.OpenAPI.Spec, openapi.LoadOptions{})
	if err != nil {
		return nil, "", apperr.Wrap(apperr.Invalid, err)
	}
	m.mu.Lock()
	m.spec, m.specDir, m.covered = spec, p.OpenAPI.Spec, map[string]bool{}
	m.mu.Unlock()
	return spec, p.OpenAPI.Spec, nil
}

// Spec returns the project's contract and its operations.
func (m *Mocks) Spec(ctx context.Context) (*Spec, error) {
	spec, file, err := m.load(ctx)
	if err != nil || spec == nil {
		return &Spec{Operations: []string{}}, err
	}
	rel := file
	if root := m.project(); root != nil {
		if r, err := filepath.Rel(root.Dir(), file); err == nil {
			rel = filepath.ToSlash(r)
		}
	}
	return &Spec{File: rel, Operations: spec.Operations()}, nil
}

// Start serves the mock on 127.0.0.1:port. A port in use is refused with
// the next free one as the error's data.
func (m *Mocks) Start(ctx context.Context, port int) (*Status, error) {
	spec, _, err := m.load(ctx)
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, apperr.New(apperr.NotFound, "sonde.yaml has no openapi spec")
	}
	if port < 1 || port > 65535 {
		return nil, apperr.New(apperr.Invalid, "port out of range")
	}
	m.mu.Lock()
	running := m.stop != nil
	m.mu.Unlock()
	if running {
		return nil, apperr.New(apperr.Busy, "the mock is running")
	}
	// Loopback only, never every interface.
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, &apperr.Error{Code: apperr.Busy, Message: "port " + strconv.Itoa(port) + " is not available", Data: map[string]int{"next": nextFree(port)}}
	}
	stop, done := make(chan struct{}), make(chan struct{})
	h := mock.Handler(spec.Mock(""), mock.Options{ValidateRequests: true, Log: &lineWriter{emit: m.emit}})
	u := "http://" + ln.Addr().String()
	m.mu.Lock()
	m.stop, m.done, m.url = stop, done, u
	m.mu.Unlock()
	go func() {
		defer close(done)
		_ = mock.Serve(stop, ln, h)
	}()
	m.setMock(u)
	st := &Status{Running: true, URL: u}
	m.emit(TopicStatus, st)
	return st, nil
}

// Stop stops the mock (in-flight requests get 5 s).
func (m *Mocks) Stop() {
	m.mu.Lock()
	stop, done := m.stop, m.done
	m.stop, m.done, m.url = nil, nil, ""
	m.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-done
	m.setMock("")
	m.emit(TopicStatus, &Status{})
}

// Status reports whether the mock runs.
func (m *Mocks) Status() *Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &Status{Running: m.stop != nil, URL: m.url}
}

// Observe records the operations the requests of finished runs matched.
func (m *Mocks) Observe(results []*engine.UnitResult) {
	m.mu.Lock()
	spec := m.spec
	m.mu.Unlock()
	if spec == nil {
		return
	}
	mk := spec.Mock("")
	var hit []string
	for _, res := range results {
		for _, e := range res.Entries {
			for _, c := range e.Calls {
				u, err := url.Parse(c.Request.URL)
				if err != nil {
					continue
				}
				if op, p := mk.Match(c.Request.Method, u.Path); p == nil && op != nil {
					hit = append(hit, op.Name())
				}
			}
		}
	}
	m.mu.Lock()
	for _, h := range hit {
		m.covered[h] = true
	}
	m.mu.Unlock()
}

// Coverage returns the operations this session's runs called.
func (m *Mocks) Coverage(ctx context.Context) (*Coverage, error) {
	spec, _, err := m.load(ctx)
	if err != nil {
		return nil, err
	}
	out := &Coverage{Covered: []string{}}
	if spec == nil {
		return out, nil
	}
	ops := spec.Operations()
	out.Total = len(ops)
	m.mu.Lock()
	for _, op := range ops {
		if m.covered[op] {
			out.Covered = append(out.Covered, op)
		}
	}
	m.mu.Unlock()
	slices.Sort(out.Covered)
	return out, nil
}

// ServiceShutdown stops the mock when the app quits.
func (m *Mocks) ServiceShutdown() error {
	m.Stop()
	return nil
}

// nextFree returns a free loopback port after port.
func nextFree(port int) int {
	for p := port + 1; p <= 65535 && p < port+100; p++ {
		if ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p)); err == nil {
			_ = ln.Close()
			return p
		}
	}
	return 0
}

// lineWriter sends each complete line written to it as a log event.
type lineWriter struct {
	emit func(string, any)
	mu   sync.Mutex
	buf  []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.emit(TopicLog, string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
}

// Service is the mock bindings.
type Service struct{ m *Mocks }

// NewService returns the bindings over m.
func NewService(m *Mocks) *Service { return &Service{m: m} }

// Spec returns the project's contract.
func (s *Service) Spec(ctx context.Context) (*Spec, error) { return s.m.Spec(ctx) }

// Start serves the mock on 127.0.0.1:port.
func (s *Service) Start(ctx context.Context, port int) (*Status, error) { return s.m.Start(ctx, port) }

// Stop stops the mock.
func (s *Service) Stop() { s.m.Stop() }

// Status reports whether the mock runs.
func (s *Service) Status() *Status { return s.m.Status() }

// Coverage returns the operations this session's runs called.
func (s *Service) Coverage(ctx context.Context) (*Coverage, error) { return s.m.Coverage(ctx) }

// ServiceShutdown stops the mock when the app quits.
func (s *Service) ServiceShutdown() error { return s.m.ServiceShutdown() }
