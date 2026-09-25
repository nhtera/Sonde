// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"strings"
	"time"
)

// Format formats t like chrono's DateTime<Utc>::format(layout). t is
// always treated as a UTC instant. An invalid or unsupported specifier in
// layout returns an *Error with kind BadFormat, mirroring chrono's
// fmt::Error.
func Format(t time.Time, layout string) (string, error) {
	items, err := tokenizeFormat(layout)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := formatItems(&b, t.UTC(), items); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ParseDateTime parses s like DateTime::<FixedOffset>::parse_from_str: an
// offset must be present in the input. It returns the parsed instant in
// UTC.
func ParseDateTime(s, layout string) (time.Time, error) {
	items, err := tokenizeFormat(layout)
	if err != nil {
		return time.Time{}, err
	}
	var p parsed
	if err := parseItems(&p, s, items); err != nil {
		return time.Time{}, err
	}
	return p.toDatetime()
}

// ParseNaiveDateTime parses s like NaiveDateTime::parse_from_str: no
// offset is required, and none is applied even if present in the input by
// way of a %z-like specifier being consumed. The result is interpreted as
// UTC.
func ParseNaiveDateTime(s, layout string) (time.Time, error) {
	items, err := tokenizeFormat(layout)
	if err != nil {
		return time.Time{}, err
	}
	var p parsed
	if err := parseItems(&p, s, items); err != nil {
		return time.Time{}, err
	}
	t, err := p.toNaiveDateTimeWithOffset(0)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// ParseNaiveDate parses s like NaiveDate::parse_from_str. The result is
// midnight UTC on the parsed date.
func ParseNaiveDate(s, layout string) (time.Time, error) {
	items, err := tokenizeFormat(layout)
	if err != nil {
		return time.Time{}, err
	}
	var p parsed
	if err := parseItems(&p, s, items); err != nil {
		return time.Time{}, err
	}
	date, err := p.toNaiveDate()
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(date.year, time.Month(date.month), date.day, 0, 0, 0, 0, time.UTC), nil
}

// ParseRFC3339 parses s like DateTime::parse_from_rfc3339: a strict RFC
// 3339 date-and-time string, e.g. "1996-12-19T16:39:57-08:00".
func ParseRFC3339(s string) (time.Time, error) {
	return parseRFC3339Strict(s)
}

// ParseRFC2822 parses s like DateTime::parse_from_rfc2822: an RFC 2822
// date-and-time string as used by HTTP and email headers, e.g.
// "Tue, 1 Jul 2003 10:52:37 +0200".
func ParseRFC2822(s string) (time.Time, error) {
	var p parsed
	rest, err := parseRFC2822(&p, s)
	if err != nil {
		return time.Time{}, err
	}
	if rest != "" {
		return time.Time{}, errTooLong
	}
	return p.toDatetime()
}
