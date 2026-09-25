// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package value

import (
	"math"
	"strconv"
	"strings"
)

// FormatFloat formats f in the shortest form that reads back exactly, without
// exponent; whole numbers keep a ".0" suffix ("1.0", "-2.0", "1e21" prints
// "1000000000000000000000.0"). Non-finite values print "inf", "-inf", "NaN".
func FormatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if f == math.Trunc(f) {
		s += ".0"
	}
	return s
}

// NumberFromText classifies a JSON number by its text: a fraction or exponent
// makes a Float (BigInt when not finite), an integer that fits 64 bits makes
// an Int, anything else a BigInt. A BigInt keeps the text with its exponent
// written `e+N` or `e-N`.
func NumberFromText(s string) Value {
	if strings.ContainsAny(s, ".eE") {
		f, err := strconv.ParseFloat(s, 64)
		if err == nil && !math.IsInf(f, 0) {
			return Float(f)
		}
		if i := strings.IndexAny(s, "eE"); i >= 0 {
			exp := s[i+1:]
			if exp == "" || exp[0] != '+' && exp[0] != '-' {
				exp = "+" + exp
			}
			s = s[:i] + "e" + exp
		}
		return BigInt(s)
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return Int(i)
	}
	return BigInt(s)
}

// Ordering is the result of comparing two values.
type Ordering int

// Orderings. Same means equal; Unordered is returned when a NaN is involved.
const (
	Less Ordering = iota - 1
	Same
	Greater
	Unordered
)

// compareNumbers compares two numbers of any representation. Integers and
// floats compare as float64; BigInt values compare exactly.
func compareNumbers(a, b Value) Ordering {
	switch a := a.(type) {
	case Int:
		switch b := b.(type) {
		case Int:
			return cmpOrdered(a, b)
		case Float:
			return compareFloats(float64(a), float64(b))
		}
	case Float:
		switch b := b.(type) {
		case Int:
			return compareFloats(float64(a), float64(b))
		case Float:
			return compareFloats(float64(a), float64(b))
		}
	}
	return compareExact(a, b)
}

func cmpOrdered[T int64 | Int | float64](a, b T) Ordering {
	switch {
	case a < b:
		return Less
	case a > b:
		return Greater
	}
	return Same
}

func compareFloats(a, b float64) Ordering {
	if math.IsNaN(a) || math.IsNaN(b) {
		return Unordered
	}
	return cmpOrdered(a, b)
}

// compareExact compares numbers when a BigInt is involved: as decimal
// numbers, in time linear in the length of their text. A float takes its
// shortest decimal form.
func compareExact(a, b Value) Ordering {
	da, okA := toDecimal(a)
	db, okB := toDecimal(b)
	if !okA || !okB {
		return Unordered
	}
	return da.cmp(db)
}

// decimal is ±0.digits × 10^exp, or ±infinity; zero has no digits.
type decimal struct {
	neg, inf bool
	digits   string // no leading or trailing zeros
	exp      int64
}

func toDecimal(v Value) (decimal, bool) {
	switch v := v.(type) {
	case Int:
		return parseDecimal(strconv.FormatInt(int64(v), 10)), true
	case Float:
		f := float64(v)
		switch {
		case math.IsNaN(f):
			return decimal{}, false
		case math.IsInf(f, 0):
			return decimal{neg: f < 0, inf: true}, true
		}
		return parseDecimal(strconv.FormatFloat(f, 'e', -1, 64)), true
	case BigInt:
		return parseDecimal(string(v)), true
	}
	return decimal{}, false
}

// parseDecimal reads [-]digits[.digits][(e|E)[+-]digits].
func parseDecimal(s string) decimal {
	var d decimal
	if strings.HasPrefix(s, "-") {
		d.neg, s = true, s[1:]
	}
	mant, expText := s, ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant, expText = s[:i], s[i+1:]
	}
	intPart, frac, _ := strings.Cut(mant, ".")
	exp, err := strconv.ParseInt(expText, 10, 64)
	if err != nil && expText != "" {
		// Beyond int64: saturate, the order is kept.
		exp = math.MaxInt64 / 2
		if strings.HasPrefix(expText, "-") {
			exp = -exp
		}
	}
	digits := intPart + frac
	exp += int64(len(intPart))
	trimmed := strings.TrimLeft(digits, "0")
	exp -= int64(len(digits) - len(trimmed))
	d.digits = strings.TrimRight(trimmed, "0")
	d.exp = exp
	if d.digits == "" {
		d.neg, d.exp = false, 0
	}
	return d
}

func (d decimal) sign() int {
	switch {
	case d.neg:
		return -1
	case d.inf || d.digits != "":
		return 1
	}
	return 0
}

func (d decimal) cmp(e decimal) Ordering {
	if s, t := d.sign(), e.sign(); s != t {
		return cmpOrdered(int64(s), int64(t))
	}
	m := d.cmpMagnitude(e)
	if d.neg {
		m = -m
	}
	return m
}

func (d decimal) cmpMagnitude(e decimal) Ordering {
	switch {
	case d.inf || e.inf:
		return cmpOrdered(boolInt(d.inf), boolInt(e.inf))
	case d.exp != e.exp:
		return cmpOrdered(d.exp, e.exp)
	}
	return Ordering(strings.Compare(d.digits, e.digits))
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func isNumber(v Value) bool {
	switch v.(type) {
	case Int, BigInt, Float:
		return true
	}
	return false
}
