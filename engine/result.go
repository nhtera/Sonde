// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/redact"
)

// UnitResult is the result of running one file.
type UnitResult struct {
	File   string
	Source []byte
	// ParseError is set when the file does not parse; nothing ran. Its
	// kind is ErrorParse.
	ParseError *Error
	// Entries has one result per attempt: a retried entry appears once per
	// attempt, repeated entries once per repetition.
	Entries []*EntryResult
	// Interrupted is set when the run was stopped or canceled before its
	// last entry; an interrupted run is not a success.
	Interrupted bool
	Success     bool
	Duration    time.Duration
	// Cookies is the cookie store at the end of the run.
	Cookies []Cookie
	// Timestamp is the start of the run.
	Timestamp time.Time
	// Row is the 1-based index of the data row the file ran with (0:
	// none).
	Row int

	// runSecrets and rowSecrets are the registries Redact masks.
	runSecrets, rowSecrets *redact.Registry
	// redacted tells, for an attempt with a `redact` capture, which of
	// its Captures are secret (enginex.CaptureRedacted).
	redacted map[*EntryResult][]bool
}

// Label names the run in output and reports: the file, and its data row
// as "#row-N".
func (u *UnitResult) Label() string {
	if u.Row > 0 {
		return rowLabel(u.File, u.Row)
	}
	return u.File
}

func rowLabel(file string, row int) string { return file + "#row-" + strconv.Itoa(row) }

// Redact masks in s every secret known for this result, in one pass: the
// runner's (as of the call: sinks written after the run get the final
// union) and those of its data row (Row.Secrets, and the `redact`
// captures and credentials of the row's run), which Runner.Redact does
// not know. Sinks showing a result redact with it. Row secrets exist only
// in a run with data rows; a UnitResult not produced by a Runner redacts
// nothing.
func (u *UnitResult) Redact(s string) string {
	switch {
	case u.runSecrets != nil:
		return u.runSecrets.RedactWith(s, u.rowSecrets)
	case u.rowSecrets != nil:
		return u.rowSecrets.Redact(s)
	}
	return s
}

// HasSecrets reports whether Redact knows any secret.
func (u *UnitResult) HasSecrets() bool {
	return (u.runSecrets != nil && u.runSecrets.Len() > 0) || (u.rowSecrets != nil && u.rowSecrets.Len() > 0)
}

// Errors returns the errors that decided the outcome: those of every
// decisive result (see Decisive). It does not include ParseError.
func (u *UnitResult) Errors() []*Error {
	var errs []*Error
	for i, e := range u.Entries {
		if u.Decisive(i) {
			errs = append(errs, e.Errors...)
		}
	}
	return errs
}

// Decisive reports whether the errors of Entries[i] decide the outcome,
// as the reference decides it: unless the next result is of the same
// entry (a retry, or another repeat of it), and always for the last.
func (u *UnitResult) Decisive(i int) bool { return decisive(u.Entries, i) }

func decisive(entries []*EntryResult, i int) bool {
	return i+1 >= len(entries) || entries[i+1].Index != entries[i].Index
}

// EntryResult is one attempt of an entry.
type EntryResult struct {
	Index int // 1-based
	// Line is the line of the request method.
	Line     int
	Calls    []Call
	Captures []Capture
	Asserts  []Assert
	// Violations are the contract findings of the final response (see
	// Options.Validator): those that are not warnings also appear in
	// Asserts, located at the status line.
	Violations []Violation
	// Errors are the errors of this attempt; an error whose Assert method
	// reports true is an assert failure.
	Errors           []*Error
	TransferDuration time.Duration
	Compressed       bool
	// Curl is the equivalent curl command line of the request.
	Curl string
	// Retried is set when the attempt failed and the entry was run again;
	// its errors do not count.
	Retried bool
}

// Call is one HTTP exchange (redirects produce several).
type Call struct {
	Request  exchange.Request
	Response *exchange.Response
	// Timings are the timings of Response (the same as Response.Timings).
	Timings exchange.Timings
}

// Capture is a captured variable.
type Capture struct {
	Name  string
	Value Value
}

// Assert is the outcome of one assert (implicit or explicit); Err is nil
// when it holds.
type Assert struct {
	Line int
	Err  *Error
}

// Cookie is a cookie of the cookie store (Netscape fields).
type Cookie struct {
	Domain           string
	IncludeSubdomain bool
	Path             string
	HTTPS            bool
	// Expires is a Unix time in seconds; 0 for a session cookie, 1 for a
	// cookie the server expired.
	Expires  int64
	Name     string
	Value    string
	HTTPOnly bool
}

// Netscape formats the cookie as a line of a Netscape cookie file, without
// the line terminator.
func (c Cookie) Netscape() string {
	domain := c.Domain
	if c.HTTPOnly {
		domain = "#HttpOnly_" + domain
	}
	b := func(v bool) string {
		if v {
			return "TRUE"
		}
		return "FALSE"
	}
	return strings.Join([]string{domain, b(c.IncludeSubdomain), c.Path, b(c.HTTPS),
		strconv.FormatInt(c.Expires, 10), c.Name, c.Value}, "\t")
}
