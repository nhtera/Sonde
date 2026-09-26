// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// UnitResult is the result of running one file.
type UnitResult struct {
	File   string
	Source []byte
	// ParseError is set when the file does not parse; nothing ran.
	ParseError *syntax.Error
	// Entries has one result per attempt: a retried entry appears once per
	// attempt, repeated entries once per repetition.
	Entries  []*EntryResult
	Success  bool
	Duration time.Duration
	// Cookies is the cookie store at the end of the run.
	Cookies []Cookie
	// Timestamp is the start of the run.
	Timestamp time.Time
}

// Errors returns the errors that decided the outcome: those of every
// attempt that was not retried.
func (u *UnitResult) Errors() []*runerr.Error {
	var errs []*runerr.Error
	for _, e := range u.Entries {
		if !e.Retried {
			errs = append(errs, e.Errors...)
		}
	}
	return errs
}

// EntryResult is one attempt of an entry.
type EntryResult struct {
	Index int // 1-based
	// Line is the line of the request method.
	Line     int
	Calls    []Call
	Captures []Capture
	Asserts  []Assert
	// Errors are the errors of this attempt; an error with Assert set is an
	// assert failure.
	Errors           []*runerr.Error
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
	Timings  exchange.Timings
}

// Capture is a captured variable.
type Capture struct {
	Name  string
	Value value.Value
}

// Assert is the outcome of one assert (implicit or explicit); Err is nil
// when it holds.
type Assert struct {
	Line int
	Err  *runerr.Error
}

// Cookie is a cookie of the cookie store (Netscape fields).
type Cookie struct {
	Domain           string
	IncludeSubdomain bool
	Path             string
	HTTPS            bool
	Expires          int64
	Name             string
	Value            string
	HTTPOnly         bool
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
