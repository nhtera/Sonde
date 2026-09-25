// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"strings"
	"time"
	"unicode"
)

// scanNumber tries to parse a non-negative number of min..max ASCII
// digits, greedily. The absence of any digit is always an error; a number
// that overflows int64 is OutOfRange.
func scanNumber(s string, minDigits, maxDigits int) (rest string, v int64, err error) {
	if len(s) < minDigits {
		return "", 0, errTooShort
	}
	n := int64(0)
	limit := maxDigits
	if limit > len(s) {
		limit = len(s)
	}
	i := 0
	for ; i < limit; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			if i < minDigits {
				return "", 0, errInvalid
			}
			return s[i:], n, nil
		}
		nn := n*10 + int64(c-'0')
		if nn < n {
			return "", 0, errOutOfRange
		}
		n = nn
	}
	return s[i:], n, nil
}

var nanoScale = [10]uint32{0, 100_000_000, 10_000_000, 1_000_000, 100_000, 10_000, 1_000, 100, 10, 1}
var nanoScale64 = [10]int64{0, 100_000_000, 10_000_000, 1_000_000, 100_000, 10_000, 1_000, 100, 10, 1}

// scanNanosecond consumes at least one digit as a fractional second and
// returns the number of whole nanoseconds (0..999,999,999). Digits beyond
// the ninth are consumed but ignored.
func scanNanosecond(s string) (rest string, v uint32, err error) {
	orig := s
	s, n, err := scanNumber(s, 1, 9)
	if err != nil {
		return "", 0, err
	}
	consumed := len(orig) - len(s)
	// consumed <= 9 and n has exactly `consumed` digits, so n*scale always
	// fits a uint32 (< 10^9); no overflow check is needed here.
	nv := uint32(n) * nanoScale[consumed] //nolint:gosec // G115: see comment above
	// trim any remaining ascii digits
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[i:], nv, nil
}

// scanNanosecondFixed consumes exactly `digits` digits as a fractional
// second and returns the number of whole nanoseconds.
func scanNanosecondFixed(s string, digits int) (rest string, v int64, err error) {
	s, n, err := scanNumber(s, digits, digits)
	if err != nil {
		return "", 0, err
	}
	scale := nanoScale64[digits]
	nv := n * scale
	if scale != 0 && nv/scale != n {
		return "", 0, errOutOfRange
	}
	return s, nv, nil
}

var shortMonthTable = map[[3]byte]uint8{
	{'j', 'a', 'n'}: 0, {'f', 'e', 'b'}: 1, {'m', 'a', 'r'}: 2, {'a', 'p', 'r'}: 3,
	{'m', 'a', 'y'}: 4, {'j', 'u', 'n'}: 5, {'j', 'u', 'l'}: 6, {'a', 'u', 'g'}: 7,
	{'s', 'e', 'p'}: 8, {'o', 'c', 't'}: 9, {'n', 'o', 'v'}: 10, {'d', 'e', 'c'}: 11,
}

// scanShortMonth0 parses the month index (0..11) from its first three
// ASCII letters, case-insensitively.
func scanShortMonth0(s string) (rest string, month0 uint8, err error) {
	if len(s) < 3 {
		return "", 0, errTooShort
	}
	key := [3]byte{s[0] | 32, s[1] | 32, s[2] | 32}
	m, ok := shortMonthTable[key]
	if !ok {
		return "", 0, errInvalid
	}
	return s[3:], m, nil
}

var shortWeekdayTable = map[[3]byte]time.Weekday{
	{'m', 'o', 'n'}: time.Monday, {'t', 'u', 'e'}: time.Tuesday, {'w', 'e', 'd'}: time.Wednesday,
	{'t', 'h', 'u'}: time.Thursday, {'f', 'r', 'i'}: time.Friday, {'s', 'a', 't'}: time.Saturday,
	{'s', 'u', 'n'}: time.Sunday,
}

// scanShortWeekday parses a weekday from its first three ASCII letters,
// case-insensitively.
func scanShortWeekday(s string) (rest string, wd time.Weekday, err error) {
	if len(s) < 3 {
		return "", 0, errTooShort
	}
	key := [3]byte{s[0] | 32, s[1] | 32, s[2] | 32}
	w, ok := shortWeekdayTable[key]
	if !ok {
		return "", 0, errInvalid
	}
	return s[3:], w, nil
}

// longMonthSuffixes are the lowercased month names minus their first three
// characters, indexed by month0.
var longMonthSuffixes = [12]string{
	"uary", "ruary", "ch", "il", "", "e", "y", "ust", "tember", "ober", "ember", "ember",
}

// scanShortOrLongMonth0 parses a month index (0..11) accepting either the
// short or the long form, preferring the long form when both match.
func scanShortOrLongMonth0(s string) (rest string, month0 uint8, err error) {
	s, month0, err = scanShortMonth0(s)
	if err != nil {
		return "", 0, err
	}
	suffix := longMonthSuffixes[month0]
	if len(s) >= len(suffix) && strings.EqualFold(s[:len(suffix)], suffix) {
		s = s[len(suffix):]
	}
	return s, month0, nil
}

// longWeekdaySuffixes are the lowercased weekday names minus their first
// three characters, indexed by "days from Monday" (Mon=0..Sun=6).
var longWeekdaySuffixes = [7]string{"day", "sday", "nesday", "rsday", "day", "urday", "day"}

// scanShortOrLongWeekday parses a weekday accepting either the short or the
// long form, preferring the long form when both match.
func scanShortOrLongWeekday(s string) (rest string, wd time.Weekday, err error) {
	s, wd, err = scanShortWeekday(s)
	if err != nil {
		return "", 0, err
	}
	suffix := longWeekdaySuffixes[daysFromMonday(wd)]
	if len(s) >= len(suffix) && strings.EqualFold(s[:len(suffix)], suffix) {
		s = s[len(suffix):]
	}
	return s, wd, nil
}

func daysFromMonday(wd time.Weekday) int { return (int(wd) + 6) % 7 }

// scanChar consumes exactly one given ASCII byte.
func scanChar(s string, c byte) (rest string, err error) {
	if s == "" {
		return "", errTooShort
	}
	if s[0] != c {
		return "", errInvalid
	}
	return s[1:], nil
}

// scanSpace consumes one or more Unicode whitespace characters.
func scanSpace(s string) (rest string, err error) {
	trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)
	if len(trimmed) < len(s) {
		return trimmed, nil
	}
	if s == "" {
		return "", errTooShort
	}
	return "", errInvalid
}

// scanColonOrSpace consumes any number (including zero) of colons or
// Unicode whitespace.
func scanColonOrSpace(s string) string {
	return strings.TrimLeftFunc(s, func(r rune) bool { return r == ':' || unicode.IsSpace(r) })
}

// scanTimezoneOffset parses a timezone offset and returns the offset in
// seconds east of UTC. consumeColon consumes a mandatory or optional ':'
// separator between the hour and minute offsets. allowMissingMinutes lets
// the minutes be absent. allowTzMinusSign additionally accepts U+2212
// MINUS SIGN as the negative sign, as required by RFC 3339 / ISO 8601.
func scanTimezoneOffset(
	s string,
	consumeColon func(string) (string, error),
	allowZulu, allowMissingMinutes, allowTzMinusSign bool,
) (rest string, offsetSeconds int32, err error) {
	if allowZulu && len(s) > 0 && (s[0] == 'Z' || s[0] == 'z') {
		return s[1:], 0, nil
	}

	digits2 := func(s string) (byte, byte, error) {
		if len(s) < 2 {
			return 0, 0, errTooShort
		}
		return s[0], s[1], nil
	}

	var negative bool
	switch {
	case strings.HasPrefix(s, "+"):
		s = s[1:]
		negative = false
	case strings.HasPrefix(s, "-"):
		s = s[1:]
		negative = true
	case strings.HasPrefix(s, "−"): // MINUS SIGN
		if !allowTzMinusSign {
			return "", 0, errInvalid
		}
		s = s[len("−"):]
		negative = true
	case s == "":
		return "", 0, errTooShort
	default:
		return "", 0, errInvalid
	}

	h1, h2, err := digits2(s)
	if err != nil {
		return "", 0, err
	}
	if h1 < '0' || h1 > '9' || h2 < '0' || h2 > '9' {
		return "", 0, errInvalid
	}
	hours := int32(h1-'0')*10 + int32(h2-'0')
	s = s[2:]

	s, err = consumeColon(s)
	if err != nil {
		return "", 0, err
	}

	var minutes int32
	if m1, m2, derr := digits2(s); derr == nil {
		switch {
		case m1 >= '0' && m1 <= '5' && m2 >= '0' && m2 <= '9':
			minutes = int32(m1-'0')*10 + int32(m2-'0')
		case m1 >= '6' && m1 <= '9' && m2 >= '0' && m2 <= '9':
			return "", 0, errOutOfRange
		default:
			return "", 0, errInvalid
		}
	} else if allowMissingMinutes {
		minutes = 0
	} else {
		return "", 0, errTooShort
	}
	switch {
	case len(s) >= 2:
		s = s[2:]
	case len(s) == 0:
		// leave s as-is
	default:
		return "", 0, errTooShort
	}

	seconds := hours*3600 + minutes*60
	if negative {
		seconds = -seconds
	}
	return s, seconds, nil
}

// timezoneNames2822 maps RFC 2822 legacy North American timezone names
// (case-insensitively) to their offsets in hours east of UTC.
var timezoneNames2822 = map[string]int32{
	"gmt": 0, "ut": 0, "z": 0,
	"edt": -4,
	"est": -5, "cdt": -5,
	"cst": -6, "mdt": -6,
	"mst": -7, "pdt": -7,
	"pst": -8,
}

// scanTimezoneOffset2822 is like scanTimezoneOffset but also accepts RFC
// 2822 legacy timezone names. A single-letter military zone (other than
// "j"/"J") is consumed but treated as -0000, per RFC 2822 section 4.3.
func scanTimezoneOffset2822(s string) (rest string, offsetSeconds int32, err error) {
	upto := 0
	for upto < len(s) && isASCIIAlpha(s[upto]) {
		upto++
	}
	if upto == 0 {
		return scanTimezoneOffset(s, func(s string) (string, error) { return s, nil }, false, false, false)
	}

	name := s[:upto]
	rest = s[upto:]
	lower := strings.ToLower(name)
	if h, ok := timezoneNames2822[lower]; ok {
		return rest, h * 3600, nil
	}
	if len(name) == 1 {
		c := lower[0]
		if (c >= 'a' && c <= 'i') || (c >= 'k' && c <= 'y') {
			return rest, 0, nil
		}
	}
	return "", 0, errInvalid
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// scanComment2822 consumes an RFC 2822 comment, including any preceding
// whitespace, and returns the string after the closing parenthesis.
func scanComment2822(s string) (rest string, err error) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	const (
		stStart = iota
		stNext
		stEscape
	)
	state := stStart
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case state == stStart && c == '(':
			state, depth = stNext, 1
		case state == stNext && depth == 1 && c == ')':
			return s[i+1:], nil
		case state == stNext && c == '\\':
			state = stEscape
		case state == stNext && c == '(':
			depth++
		case state == stNext && c == ')':
			depth--
		case state == stNext, state == stEscape:
			state = stNext
		default:
			return "", errInvalid
		}
	}
	return "", errTooShort
}
