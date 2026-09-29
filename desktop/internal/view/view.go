// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package view builds every value the app hands to the frontend (binding
// results, events, body URLs) and is the one place they are redacted. The
// engine's events are converted inside the run's event callback with the
// unit's live redactor, so a secret never reaches a DTO, a queue or the
// frontend.
package view

import (
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/report"
)

// Redactor masks every known secret in a string.
type Redactor func(string) string

// BodyStore keeps a redacted response body for its URL and returns its id.
type BodyStore interface {
	Put(redacted []byte, contentType string) (id string)
}

// Event types, the "type" of every event DTO.
const (
	TypeLog           = "log"
	TypeEntryStarted  = "entryStarted"
	TypeRequestSent   = "requestSent"
	TypeEntrySkipped  = "entrySkipped"
	TypeMessage       = "message"
	TypeEntryFinished = "entryFinished"
)

// Log is a line of the engine's verbose output, attached to an entry (0
// before the first).
type Log struct {
	Type  string `json:"type"`
	Entry int    `json:"entry"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// EntryStarted is an attempt of an entry starting.
type EntryStarted struct {
	Type  string `json:"type"`
	Entry int    `json:"entry"`
	Retry int    `json:"retry"`
	// Last is the entry's last index in this run, for progress.
	Last int `json:"last"`
}

// RequestSent is a request of the file leaving, before its response.
type RequestSent struct {
	Type    string         `json:"type"`
	Entry   int            `json:"entry"`
	Call    int            `json:"call"`
	Request report.Request `json:"request"`
}

// EntrySkipped is an entry the run did not execute: "option" (skip: true)
// or "repeat-zero".
type EntrySkipped struct {
	Type   string `json:"type"`
	Entry  int    `json:"entry"`
	Reason string `json:"reason"`
}

// Message is a stream message (SSE, WebSocket, gRPC) as it passes.
type Message struct {
	Type    string               `json:"type"`
	Entry   int                  `json:"entry"`
	Message report.StreamMessage `json:"message"`
}

// EntryFinished is an attempt of an entry finished.
type EntryFinished struct {
	Type  string `json:"type"`
	Entry Entry  `json:"entry"`
}

// Entry is an attempt of an entry: the CLI's JSON report shape plus the
// body URLs, errors and outcome the results panel needs.
type Entry struct {
	report.Entry
	// Bodies has one item per call: the redacted response body's id.
	Bodies  []Body  `json:"bodies"`
	Errors  []Error `json:"errors"`
	Success bool    `json:"success"`
	Retried bool    `json:"retried"`
}

// Body is a stored response body, fetched at /_sonde/body/<id>. ID is
// empty when the call had no body.
type Body struct {
	ID          string `json:"id"`
	Size        int    `json:"size"`
	ContentType string `json:"contentType"`
	// Error is why the body could not be decoded (it is then stored as
	// transferred, redacted).
	Error string `json:"error,omitempty"`
}

// Error is an error of an attempt.
type Error struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Kind        string `json:"kind"`
	Assert      bool   `json:"assert"`
	Description string `json:"description"`
	Message     string `json:"message"`
	// Transport is the class of a transport error (connect, resolve,
	// timeout, tls, host-denied, canceled, other), or "".
	Transport string `json:"transport,omitempty"`
}

// levels are the engine's log levels by name.
var levels = map[engine.LogLevel]string{
	engine.LogDebug:          "debug",
	engine.LogDebugImportant: "debugImportant",
	engine.LogRequest:        "request",
	engine.LogResponse:       "response",
	engine.LogDebugError:     "debugError",
	engine.LogWarning:        "warning",
	engine.LogError:          "error",
	engine.LogRequestLine:    "requestLine",
	engine.LogResponseLine:   "responseLine",
	engine.LogCapture:        "capture",
}
