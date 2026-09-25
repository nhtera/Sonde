// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"math"
	"strconv"
)

// parsedNumber is a JSON number literal, classified the same way the
// grammar distinguishes them: a fraction or exponent always makes a
// float; otherwise it is an integer, kept as a string once it overflows
// int64.
type parsedNumber struct {
	kind numberKind
	i    int64
	big  string
	f    float64
}

// minSafeInt and maxSafeInt bound the index and step values JSONPath
// accepts: I-JSON's safe integer range, ±(2^53-1).
const (
	minSafeInt int64 = -9007199254740991
	maxSafeInt int64 = 9007199254740991
)

// tryInteger parses an index or step value: a decimal integer within the
// I-JSON safe range. Number literals used as comparables (which may be
// arbitrarily large) are parsed by tryNumber instead.
func tryInteger(r *reader) (int64, bool, error) {
	savedPos := r.cursor()
	s, ok, err := tryStringInteger(r)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	v, perr := strconv.ParseInt(s, 10, 64)
	if perr != nil {
		return 0, false, newParseError(savedPos, "expecting a valid integer")
	}
	if v < minSafeInt || v > maxSafeInt {
		return 0, false, newParseError(savedPos,
			"expecting integer value inside the allowed range ±(2⁵³-1, 2⁵³-1)")
	}
	return v, true, nil
}

// tryNumber parses a JSON number literal.
func tryNumber(r *reader) (parsedNumber, bool, error) {
	save := r.cursor()

	var stringInteger string
	switch {
	case matchStr("-0", r):
		stringInteger = "-0"
	default:
		s, ok, err := tryStringInteger(r)
		if err != nil {
			return parsedNumber{}, false, err
		}
		if !ok {
			r.seek(save)
			return parsedNumber{}, false, nil
		}
		stringInteger = s
	}

	fraction, hasFraction, err := tryFraction(r)
	if err != nil {
		return parsedNumber{}, false, err
	}
	exponent, hasExponent, err := tryExponent(r)
	if err != nil {
		return parsedNumber{}, false, err
	}

	if !hasFraction && !hasExponent {
		if iv, perr := strconv.ParseInt(stringInteger, 10, 64); perr == nil {
			return parsedNumber{kind: numInt, i: iv}, true, nil
		}
		return parsedNumber{kind: numBig, big: stringInteger}, true, nil
	}

	// The whole literal is parsed at once, keeping the sign of the
	// fraction and rounding exactly once.
	text := stringInteger
	if hasFraction {
		text += "." + fraction
	}
	if hasExponent {
		text += "e" + exponent
	}
	v, perr := strconv.ParseFloat(text, 64)
	if perr != nil && !math.IsInf(v, 0) {
		return parsedNumber{}, false, newParseError(save, "expecting a valid number")
	}
	return parsedNumber{kind: numFloat, f: v}, true, nil
}

// tryStringInteger parses a decimal integer (no fraction or exponent) as
// text, keeping any minus sign. "0" is the only representation allowed
// for zero; other leading zeros are rejected.
func tryStringInteger(r *reader) (string, bool, error) {
	if matchStr("0", r) {
		return "0", true, nil
	}
	negative := matchStr("-", r)
	savedPos := r.cursor()
	digits := r.readWhile(isASCIIDigit)
	if digits == "" || digits[0] == '0' {
		if negative {
			return "", false, newParseError(savedPos, "expecting strictly positive digit")
		}
		return "", false, nil
	}
	if negative {
		return "-" + digits, true, nil
	}
	return digits, true, nil
}

// tryFraction parses `.digits` and returns the digits.
func tryFraction(r *reader) (string, bool, error) {
	if !matchStr(".", r) {
		return "", false, nil
	}
	digits := r.readWhile(isASCIIDigit)
	if digits == "" {
		return "", false, newParseError(r.cursor(), "expecting digit after decimal point")
	}
	return digits, true, nil
}

// tryExponent parses `e[+-]digits` and returns the signed digits.
func tryExponent(r *reader) (string, bool, error) {
	if !matchStr("e", r) && !matchStr("E", r) {
		return "", false, nil
	}
	sign := ""
	switch {
	case matchStr("+", r):
		sign = "+"
	case matchStr("-", r):
		sign = "-"
	}
	digits := r.readWhile(isASCIIDigit)
	if digits == "" {
		return "", false, newParseError(r.cursor(), "expecting digit in exponent")
	}
	return sign + digits, true, nil
}

func isASCIIDigit(c rune) bool { return c >= '0' && c <= '9' }
