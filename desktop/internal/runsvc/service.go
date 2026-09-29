// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runsvc

import "context"

// Service is the run bindings. Each run method returns when the run ends;
// its events arrive before, on "run:<runId>", then Done. Canceling the
// call (or Cancel) stops the run.
type Service struct{ r *Runs }

// NewService returns the bindings over r.
func NewService(r *Runs) *Service { return &Service{r: r} }

// Run runs a file.
func (s *Service) Run(ctx context.Context, req RunRequest) (*Summary, error) {
	return s.r.Run(ctx, req)
}

// Send runs one entry again with the last run's captures and cookies.
func (s *Service) Send(ctx context.Context, req SendRequest) (*Summary, error) {
	return s.r.Send(ctx, req)
}

// RunTest runs files in test mode.
func (s *Service) RunTest(ctx context.Context, req TestRequest) (*Summary, error) {
	return s.r.RunTest(ctx, req)
}

// RunData runs a file once per row of a data file.
func (s *Service) RunData(ctx context.Context, req DataRequest) (*Summary, error) {
	return s.r.RunData(ctx, req)
}

// Cancel stops run runID.
func (s *Service) Cancel(runID string) { s.r.Cancel(runID) }
