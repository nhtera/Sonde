// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"math"
	"strconv"
)

// naturalDigits reads digits without a superfluous leading zero; a missing
// first digit is recoverable.
func naturalDigits(r *reader, what string) (string, *Error) {
	start := r.pos
	c, ok := r.read()
	if !ok || !isDigit(c) {
		return "", expecting(start, true, what)
	}
	afterFirst := r.pos
	rest := r.readWhile(isDigit)
	if c == '0' && rest != "" {
		return "", expecting(afterFirst, false, what)
	}
	return r.slice(start), nil
}

// natural parses a non-negative integer.
func natural(r *reader) (*Number, *Error) {
	start := r.pos
	digits, err := naturalDigits(r, "natural")
	if err != nil {
		return nil, err
	}
	u, perr := strconv.ParseUint(digits, 10, 64)
	if perr != nil {
		return nil, expecting(start, false, "natural")
	}
	if u > math.MaxInt64 {
		return &Number{Kind: NumberBigInteger, Source: digits}, nil
	}
	return &Number{Kind: NumberInteger, Int: int64(u), Source: digits}, nil
}

// integer parses an optionally negative integer.
func integer(r *reader) (*Number, *Error) {
	start := r.pos
	neg := r.consume("-")
	digits, err := naturalDigits(r, "integer")
	if err != nil {
		return nil, err
	}
	if neg {
		digits = "-" + digits
	}
	v, perr := strconv.ParseInt(digits, 10, 64)
	if perr != nil {
		return nil, expecting(start, false, "integer")
	}
	return &Number{Kind: NumberInteger, Int: v, Source: r.slice(start)}, nil
}

// number parses an integer, a float (digits.digits) or a big integer.
func number(r *reader) (*Number, *Error) {
	start := r.pos
	r.consume("-")
	intDigits := r.readWhile(isDigit)
	if intDigits == "" {
		return nil, expecting(r.pos, true, "number")
	}
	if len(intDigits) > 1 && intDigits[0] == '0' {
		return nil, expecting(r.pos, false, "natural")
	}
	if r.consume(".") {
		if r.readWhile(isDigit) == "" {
			return nil, expecting(r.pos, false, "decimal digits")
		}
		src := r.slice(start)
		f, _ := strconv.ParseFloat(src, 64)
		return &Number{Kind: NumberFloat, Float: f, Source: src}, nil
	}
	src := r.slice(start)
	if v, err := strconv.ParseInt(src, 10, 64); err == nil {
		return &Number{Kind: NumberInteger, Int: v, Source: src}, nil
	}
	return &Number{Kind: NumberBigInteger, Source: src}, nil
}

// duration parses a natural number with an optional unit: ms, s, m or h.
func duration(r *reader) (*Duration, *Error) {
	n, err := natural(r)
	if err != nil {
		return nil, err
	}
	pos := r.pos
	unit := r.readWhile(func(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' })
	switch unit {
	case "", "ms", "s", "m", "h":
		return &Duration{Value: n, Unit: unit}, nil
	}
	return nil, errAt(pos, false, ErrInvalidDurationUnit, unit)
}
