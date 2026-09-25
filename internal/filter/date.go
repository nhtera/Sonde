// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"time"

	"github.com/nhtera/sonde/internal/datefmt"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/value"
)

func (c call) now() time.Time {
	if c.env.Now != nil {
		return c.env.Now()
	}
	return time.Now()
}

// daysFromNow counts whole days (truncated toward zero) from now to the date
// (future) or from the date to now (past).
func (c call) daysFromNow(v value.Value, future bool) (value.Value, error) {
	d, ok := v.(value.Date)
	if !ok {
		return nil, c.typeError(v, "date")
	}
	from, to := c.now(), d.UTC()
	if !future {
		from, to = to, from
	}
	// Whole seconds between the instants, truncated toward zero.
	secs := to.Unix() - from.Unix()
	nanos := to.Nanosecond() - from.Nanosecond()
	switch {
	case secs < 0 && nanos > 0:
		secs++
	case secs > 0 && nanos < 0:
		secs--
	}
	return value.Int(secs / 86400), nil
}

func (c call) dateFormat(v value.Value) (value.Value, error) {
	layout, err := c.arg()
	if err != nil {
		return nil, err
	}
	d, ok := v.(value.Date)
	if !ok {
		return nil, c.typeError(v, "date")
	}
	s, err := datefmt.Format(d.UTC(), layout)
	if err != nil {
		e := runerr.New(c.f.Span, runerr.FilterInvalidFormatSpecifier, c.assert)
		e.Value = layout
		return nil, e
	}
	return value.String(s), nil
}

// toDate parses a string with a date layout, trying a date with time and
// offset, then a date with time (UTC), then a date alone (midnight UTC).
func (c call) toDate(v value.Value) (value.Value, error) {
	layout, err := c.arg()
	if err != nil {
		return nil, err
	}
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	for _, parse := range []func(string, string) (time.Time, error){
		datefmt.ParseDateTime, datefmt.ParseNaiveDateTime, datefmt.ParseNaiveDate,
	} {
		if t, err := parse(string(s), layout); err == nil {
			return value.Date(t), nil
		}
	}
	e := runerr.New(c.f.Span, runerr.FilterDateParsing, c.assert)
	e.Value, e.Reason = string(s), layout
	return nil, e
}
