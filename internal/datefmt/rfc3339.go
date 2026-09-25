// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import "time"

// parseRFC3339Strict parses a strict RFC 3339 date-and-time string (as
// used by DateTime::parse_from_rfc3339), mirroring chrono's
// format::parse::parse_rfc3339. Unlike the %+ specifier, this requires
// exactly two-digit month/day/hour/minute/second fields and a four-digit
// year with no explicit sign.
func parseRFC3339Strict(s string) (time.Time, error) {
	if len(s) < 19 {
		return time.Time{}, errTooShort
	}

	digit := func(i int) (int, error) {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errInvalid
		}
		return int(c - '0'), nil
	}
	digit2 := func(i int) (int, error) {
		hi, err := digit(i)
		if err != nil {
			return 0, err
		}
		lo, err := digit(i + 1)
		if err != nil {
			return 0, err
		}
		return hi*10 + lo, nil
	}

	yh, err := digit2(0)
	if err != nil {
		return time.Time{}, err
	}
	yl, err := digit2(2)
	if err != nil {
		return time.Time{}, err
	}
	year := yh*100 + yl
	if s[4] != '-' {
		return time.Time{}, errInvalid
	}
	month, err := digit2(5)
	if err != nil {
		return time.Time{}, err
	}
	if s[7] != '-' {
		return time.Time{}, errInvalid
	}
	day, err := digit2(8)
	if err != nil {
		return time.Time{}, err
	}
	date, derr := civilDateFromYMD(year, month, day)
	if derr != nil {
		return time.Time{}, errOutOfRange
	}

	switch s[10] {
	case 't', 'T', ' ':
	default:
		return time.Time{}, errInvalid
	}

	hour, err := digit2(11)
	if err != nil {
		return time.Time{}, err
	}
	if s[13] != ':' {
		return time.Time{}, errInvalid
	}
	minute, err := digit2(14)
	if err != nil {
		return time.Time{}, err
	}
	if s[16] != ':' {
		return time.Time{}, errInvalid
	}
	sec, err := digit2(17)
	if err != nil {
		return time.Time{}, err
	}
	extraNanos := int64(0)
	if sec == 60 {
		sec = 59
		extraNanos = 1_000_000_000
	} else if sec > 60 {
		return time.Time{}, errOutOfRange
	}

	rest := s[19:]
	var nano int64
	if len(rest) > 0 && rest[0] == '.' {
		r, n, nerr := scanNanosecond(rest[1:])
		if nerr != nil {
			return time.Time{}, nerr
		}
		rest = r
		nano = extraNanos + int64(n)
	} else {
		nano = extraNanos
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, errOutOfRange
	}

	rest, offset, err := scanTimezoneOffset(rest, func(s string) (string, error) { return scanChar(s, ':') }, true, false, true)
	if err != nil {
		return time.Time{}, err
	}
	if rest != "" {
		return time.Time{}, errTooLong
	}
	if offset <= -86400 || offset >= 86400 {
		return time.Time{}, errOutOfRange
	}

	base := time.Date(date.year, time.Month(date.month), date.day, hour, minute, sec, 0, time.UTC)
	instant := base.Add(time.Duration(nano)).Add(-time.Duration(offset) * time.Second)
	return instant.UTC(), nil
}
