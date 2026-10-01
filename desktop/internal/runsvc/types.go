// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runsvc

import (
	"time"

	"github.com/nhtera/sonde/desktop/internal/view"
)

// RunRequest runs a file: the page subscribes to "run:<RunID>" first.
type RunRequest struct {
	RunID string `json:"runId"`
	// File is project-relative; Source is the tab's text (the buffer is
	// what runs, saved or not).
	File   string `json:"file"`
	Source string `json:"source"`
	Env    string `json:"env"`
	// To stops after entry To (0: the last).
	To int `json:"to"`
}

// SendRequest runs entry Entry alone with the captures and cookies of the
// file's last full run.
type SendRequest struct {
	RunID  string `json:"runId"`
	File   string `json:"file"`
	Source string `json:"source"`
	Env    string `json:"env"`
	Entry  int    `json:"entry"`
}

// TestRequest runs files in test mode. Sources holds the text of files
// open in tabs; the others are read from disk.
type TestRequest struct {
	RunID   string            `json:"runId"`
	Files   []string          `json:"files"`
	Sources map[string]string `json:"sources"`
	Env     string            `json:"env"`
}

// DataRequest runs a file once per row of a data file: one of the
// project's (DataFile, project-relative) or one picked in a dialog
// (DataHandle). Rows selects rows by 1-based index (empty: all).
type DataRequest struct {
	RunID      string `json:"runId"`
	File       string `json:"file"`
	Source     string `json:"source"`
	Env        string `json:"env"`
	DataFile   string `json:"dataFile"`
	DataHandle string `json:"dataHandle"`
	Rows       []int  `json:"rows"`
	// Secrets are the data file's secret columns (--data-secret).
	Secrets []string `json:"secrets"`
}

// Outcomes.
const (
	Passed   = "passed"
	Failed   = "failed"
	Errored  = "error" // a file that does not parse, or a run that could not start
	Canceled = "canceled"
)

// Summary is the outcome of a run, also sent with its Done event.
type Summary struct {
	RunID string `json:"runId"`
	// Kind is "run", "send", "test" or "data".
	Kind      string    `json:"kind"`
	Env       string    `json:"env"`
	Outcome   string    `json:"outcome"`
	StartedAt time.Time `json:"startedAt"`
	Files     int       `json:"files"`
	Succeeded int       `json:"succeeded"`
	Requests  int       `json:"requests"`
	Duration  int64     `json:"durationMs"`
	Units     []Unit    `json:"units"`
	// BaseRunAt is, for a Send, when the run it reused ran.
	BaseRunAt *time.Time `json:"baseRunAt,omitempty"`
	// Error is why the run could not start (a planning error).
	Error string `json:"error,omitempty"`
	// ErrorCode is Error's code (apperr: "stale", "busy"…), when it has one.
	ErrorCode string `json:"errorCode,omitempty"`
	// Text is a test run's summary as `sonde --test` prints it: a line per
	// file, then the totals.
	Text     string   `json:"text,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Unit is the outcome of one file (and data row).
type Unit struct {
	File        string      `json:"file"`
	Row         int         `json:"row,omitempty"`
	Success     bool        `json:"success"`
	Interrupted bool        `json:"interrupted"`
	Canceled    bool        `json:"canceled"`
	Requests    int         `json:"requests"`
	Duration    int64       `json:"durationMs"`
	ParseError  *view.Error `json:"parseError,omitempty"`
	// Error is why the file could not run (it could not be read…).
	Error string `json:"error,omitempty"`
}
