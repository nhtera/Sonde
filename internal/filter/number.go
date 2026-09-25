// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"math"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/value"
)

func (c call) toInt(v value.Value) (value.Value, error) {
	switch v := v.(type) {
	case value.Int:
		return v, nil
	case value.Float:
		return value.Int(saturateInt(float64(v))), nil
	case value.String:
		if i, ok := parseInt(string(v)); ok {
			return value.Int(i), nil
		}
		return nil, c.invalidValue(value.Repr(v))
	}
	return nil, c.typeError(v, "float, integer or string")
}

// saturateInt truncates f toward zero, clamping to the int64 range; NaN is 0.
func saturateInt(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// parseInt accepts an optional sign followed by decimal digits.
func parseInt(s string) (int64, bool) {
	digits := strings.TrimLeft(s, "+-")
	if len(s)-len(digits) > 1 || digits == "" {
		return 0, false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	i, err := strconv.ParseInt(s, 10, 64)
	return i, err == nil
}

func (c call) toFloat(v value.Value) (value.Value, error) {
	switch v := v.(type) {
	case value.Float:
		return v, nil
	case value.Int:
		return value.Float(float64(v)), nil
	case value.String:
		if f, ok := parseFloat(string(v)); ok {
			return value.Float(f), nil
		}
		return nil, c.invalidValue(value.Repr(v))
	case value.BigInt:
		return nil, c.invalidValue(value.Repr(v) + " is too big to be cast as a float")
	}
	return nil, c.typeError(v, "float, integer or string")
}

// parseFloat accepts an optional sign followed by `inf`, `infinity`, `nan`
// (any case) or a decimal number with optional fraction and exponent
// ("1", "1.", ".5", "1e-3"). Out of range values become infinities.
func parseFloat(s string) (float64, bool) {
	body := s
	if body != "" && (body[0] == '+' || body[0] == '-') {
		body = body[1:]
	}
	switch strings.ToLower(body) {
	case "inf", "infinity":
		return math.Copysign(math.Inf(1), signOf(s)), true
	case "nan":
		return math.NaN(), true
	}
	if !isDecimal(body) {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func signOf(s string) float64 {
	if strings.HasPrefix(s, "-") {
		return -1
	}
	return 1
}

// isDecimal matches digits* [. digits*] [(e|E) [+-] digits+] with at least
// one mantissa digit.
func isDecimal(s string) bool {
	i, n := 0, 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i, n = i+1, n+1
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i, n = i+1, n+1
		}
	}
	if n == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}

func (c call) toString(v value.Value) (value.Value, error) {
	if s, ok := value.Render(v); ok {
		return value.String(s), nil
	}
	return nil, c.invalidValue(value.Repr(v) + " can not be converted to a string")
}
