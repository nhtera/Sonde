// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

// DurationUnit is the unit assumed for a duration with no explicit suffix.
type DurationUnit int

// Duration units, matching the upstream duration suffixes.
const (
	Millisecond DurationUnit = iota
	Second
	Minute
	Hour
)

func (u DurationUnit) String() string {
	switch u {
	case Millisecond:
		return "ms"
	case Second:
		return "s"
	case Minute:
		return "m"
	case Hour:
		return "h"
	}
	return "?"
}

func durationUnitFromSuffix(s string) (DurationUnit, error) {
	switch s {
	case "ms":
		return Millisecond, nil
	case "s":
		return Second, nil
	case "m":
		return Minute, nil
	case "h":
		return Hour, nil
	}
	return 0, fmt.Errorf("Invalid duration unit %s", s)
}

var durationPattern = regexp.MustCompile(`^([0-9]+)([a-zA-Z]*)$`)

// ParseDuration parses a duration such as "5", "5s" or "500ms". A bare
// number is read in defaultUnit; the recognized suffixes are ms, s, m and
// h.
func ParseDuration(s string, defaultUnit DurationUnit) (time.Duration, error) {
	m := durationPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, errors.New("Invalid duration")
	}
	n, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0, errors.New("Duration value too large")
	}
	unit := defaultUnit
	if m[2] != "" {
		unit, err = durationUnitFromSuffix(m[2])
		if err != nil {
			return 0, err
		}
	}
	var factor uint64
	switch unit {
	case Millisecond:
		factor = 1
	case Second:
		factor = 1000
	case Minute:
		factor = 1000 * 60
	case Hour:
		factor = 1000 * 60 * 60
	}
	millis, overflow := mulOverflows(n, factor)
	// A Go Duration is int64 nanoseconds, capped near 292 years: reject a
	// millisecond count that would overflow it instead of wrapping.
	if overflow || millis > math.MaxInt64/uint64(time.Millisecond) {
		return 0, errors.New("Duration value too large")
	}
	return time.Duration(millis) * time.Millisecond, nil
}

// mulOverflows multiplies a and b, reporting whether the result overflows
// uint64 (Rust's checked_mul).
func mulOverflows(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	p := a * b
	return p, p/b != a
}
