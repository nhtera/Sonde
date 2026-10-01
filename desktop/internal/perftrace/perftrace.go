// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package perftrace measures the shipped app (sonde-desktop --perf-trace
// FILE): the page runs its tour of the budgets (start, tree, typing, a
// large body), reports each measure here, and the app writes them to FILE
// as JSON, then quits. Off (no FILE), the page runs nothing.
package perftrace

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Measure is one measured value, in milliseconds (fps for a frame rate).
type Measure struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// Trace collects the measures of one run of the app.
type Trace struct {
	path    string
	plan    string
	started time.Time
	quit    func()

	mu       sync.Mutex
	measures []Measure
}

// New returns a trace written to path ("" is off) with the tour plan
// (JSON the page reads), started at start; quit ends the app.
func New(path, plan string, start time.Time, quit func()) *Trace {
	return &Trace{path: path, plan: plan, started: start, quit: quit}
}

// Plan is the tour the page runs: "" when tracing is off.
func (t *Trace) Plan() string {
	if t.path == "" {
		return ""
	}
	fmt.Fprintln(os.Stderr, "perf: the tour starts")
	return t.plan
}

// Interactive records the time from the process start to the page's
// first interactive frame (epochMs: the page's clock, Date.now()).
func (t *Trace) Interactive(epochMs float64) {
	ms := epochMs - float64(t.started.UnixNano())/1e6
	t.Record(Measure{Name: "cold start to interactive", Value: ms, Unit: "ms"})
}

// Record adds a measure.
func (t *Trace) Record(m Measure) {
	if t.path == "" {
		return
	}
	// Progress on the terminal the traced app runs in.
	fmt.Fprintf(os.Stderr, "perf: %s: %.1f %s\n", m.Name, m.Value, m.Unit)
	t.mu.Lock()
	t.measures = append(t.measures, m)
	t.mu.Unlock()
}

// Done writes the measures and quits the app.
func (t *Trace) Done() error {
	if t.path == "" {
		return nil
	}
	t.mu.Lock()
	data, err := json.MarshalIndent(map[string]any{"measures": t.measures}, "", "  ")
	t.mu.Unlock()
	if err != nil {
		return err
	}
	// The path the user gave on the command line.
	if err := os.WriteFile(t.path, append(data, '\n'), 0o600); err != nil { //nolint:forbidigo,gosec // G306/G703: a file named by the user's own flag
		return err
	}
	if t.quit != nil {
		go t.quit()
	}
	return nil
}

// Service is the trace the page calls.
type Service struct{ t *Trace }

// NewService returns the page's trace service.
func NewService(t *Trace) *Service { return &Service{t: t} }

// Plan is the tour to run: "" when the app is not traced.
func (s *Service) Plan() string { return s.t.Plan() }

// Interactive records the first interactive frame (the page's Date.now()).
func (s *Service) Interactive(epochMs float64) { s.t.Interactive(epochMs) }

// Record adds a measure.
func (s *Service) Record(name string, value float64, unit string) {
	s.t.Record(Measure{Name: name, Value: value, Unit: unit})
}

// Done writes the measures and quits.
func (s *Service) Done() error { return s.t.Done() }
