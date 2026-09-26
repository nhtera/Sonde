// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

// Event is something a front end may show while a unit runs. Events are
// delivered serially. Log texts are redacted; results carried by events
// (and returned by the runner) hold raw values, to be passed through
// UnitResult.Redact (or Runner.Redact, for a run without data rows)
// before they are shown.
type Event interface{ isEvent() }

// LogLevel tells how a log line is presented.
type LogLevel int

// Log levels.
const (
	// LogDebug is verbose detail ("* " prefix).
	LogDebug LogLevel = iota
	// LogDebugImportant is a verbose heading ("* " prefix, emphasized).
	LogDebugImportant
	// LogRequest is a request header as sent, "Name: value" ("> "
	// prefix); an empty text ends the headers.
	LogRequest
	// LogResponse is a response header as received, "Name: value" ("< "
	// prefix); an empty text ends the headers.
	LogResponse
	// LogDebugError is a rendered error of a retried attempt, shown in
	// verbose mode only.
	LogDebugError
	// LogWarning is a warning ("warning: " prefix).
	LogWarning
	// LogError is a rendered error ("error: " prefix, followed by a blank
	// line).
	LogError
	// LogRequestLine is the request line as sent ("> " prefix).
	LogRequestLine
	// LogResponseLine is the status line as received ("< " prefix).
	LogResponseLine
	// LogCapture is a captured variable, "name: value" ("* " prefix).
	LogCapture
)

// Log is a line (or rendered error) of diagnostic output.
type Log struct {
	Level LogLevel
	Text  string
	// Color is Text with ANSI colors, set for LogError and LogDebugError.
	Color string
}

// EntryStarted is sent before an entry (or one of its retries) runs.
type EntryStarted struct {
	Index int
	Retry int
	// Last is the index of the last entry the run executes (the file's
	// entry count, or the --to-entry limit).
	Last int
}

// EntryFinished is sent after an attempt of an entry.
type EntryFinished struct {
	Result *EntryResult
}

func (Log) isEvent()           {}
func (EntryStarted) isEvent()  {}
func (EntryFinished) isEvent() {}
