// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package apperr is the error a binding returns when the frontend must
// act on its kind (a save conflict, a busy file, an expired selection):
// it reaches the page as JSON {code, message}.
package apperr

import (
	"encoding/json"
	"errors"
)

// Codes.
const (
	Conflict = "conflict" // the file changed on disk since it was read
	Busy     = "busy"     // the file already has a run
	Denied   = "denied"   // the path is outside the project or protected
	NotFound = "not-found"
	Invalid  = "invalid" // a bad argument
	Stale    = "stale"   // the edit was computed for an older version
	Expired  = "expired" // a dialog selection or session expired
)

// Error is an error with a code for the frontend.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Data is extra detail for the code (e.g. the file's current hash on
	// a conflict).
	Data any `json:"data,omitempty"`
	err  error
}

func (e *Error) Error() string { return e.Message }

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.err }

// New returns an error with code and message.
func New(code, message string) *Error { return &Error{Code: code, Message: message} }

// Wrap returns err with code; its message is err's.
func Wrap(code string, err error) *Error { return &Error{Code: code, Message: err.Error(), err: err} }

// Marshal is the app's Wails MarshalError: an *Error as its JSON, any
// other error as {code: "error", message}.
func Marshal(err error) []byte {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: "error", Message: err.Error()}
	}
	b, jerr := json.Marshal(e)
	if jerr != nil {
		b, _ = json.Marshal(Error{Code: "error", Message: err.Error()})
	}
	return b
}
