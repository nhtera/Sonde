// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// parseItems tries to fill p from s using the given formatting items,
// mirroring chrono's format::parse. It is greedy and padding-agnostic for
// numeric items. Every item must be consumed and the input must be fully
// consumed too, or an error is returned.
func parseItems(p *parsed, s string, items []item) error {
	rest, err := parseItemsPrefix(p, s, items)
	if err != nil {
		return err
	}
	if rest != "" {
		return errTooLong
	}
	return nil
}

// parseItemsPrefix is like parseItems but returns the unconsumed remainder
// instead of requiring it to be empty.
func parseItemsPrefix(p *parsed, s string, items []item) (string, error) {
	for _, it := range items {
		var err error
		s, err = parseOneItem(p, s, it)
		if err != nil {
			return "", err
		}
	}
	return s, nil
}

func parseOneItem(p *parsed, s string, it item) (string, error) {
	switch it.kind {
	case itLiteral:
		if len(s) < len(it.text) {
			return "", errTooShort
		}
		if !strings.HasPrefix(s, it.text) {
			return "", errInvalid
		}
		return s[len(it.text):], nil

	case itSpace:
		return strings.TrimLeftFunc(s, unicode.IsSpace), nil

	case itNumeric:
		return parseNumericItem(p, s, it.num)

	case itFixed:
		return parseFixedItem(p, s, it.fx)
	}
	return s, nil
}

// numericField describes how to scan and where to store a numeric item.
type numericField struct {
	width  int
	signed bool
	set    func(*parsed, int64) error
}

func numericFieldFor(n numKind) numericField {
	switch n {
	case numYear:
		return numericField{4, true, (*parsed).setYear}
	case numYearDiv100:
		return numericField{2, false, (*parsed).setYearDiv100}
	case numYearMod100:
		return numericField{2, false, (*parsed).setYearMod100}
	case numIsoYear:
		return numericField{4, true, (*parsed).setIsoYear}
	case numIsoYearMod100:
		return numericField{2, false, (*parsed).setIsoYearMod100}
	case numQuarter:
		return numericField{1, false, (*parsed).setQuarter}
	case numMonth:
		return numericField{2, false, (*parsed).setMonth}
	case numDay:
		return numericField{2, false, (*parsed).setDay}
	case numWeekFromSun:
		return numericField{2, false, (*parsed).setWeekFromSun}
	case numWeekFromMon:
		return numericField{2, false, (*parsed).setWeekFromMon}
	case numIsoWeek:
		return numericField{2, false, (*parsed).setIsoWeek}
	case numNumDaysFromSun:
		return numericField{1, false, (*parsed).setWeekdayFromNumDaysFromSun}
	case numWeekdayFromMon:
		return numericField{1, false, (*parsed).setWeekdayFromNumberFromMonday}
	case numOrdinal:
		return numericField{3, false, (*parsed).setOrdinal}
	case numHour:
		return numericField{2, false, (*parsed).setHour}
	case numHour12:
		return numericField{2, false, (*parsed).setHour12}
	case numMinute:
		return numericField{2, false, (*parsed).setMinute}
	case numSecond:
		return numericField{2, false, (*parsed).setSecond}
	case numNanosecond:
		return numericField{9, false, (*parsed).setNanosecond}
	case numTimestamp:
		return numericField{maxScanWidth, false, (*parsed).setTimestamp}
	}
	return numericField{}
}

// maxScanWidth stands in for chrono's usize::MAX maximal scan width.
const maxScanWidth = 1 << 30

func parseNumericItem(p *parsed, s string, n numKind) (string, error) {
	f := numericFieldFor(n)

	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	var v int64
	var err error
	switch {
	case f.signed && strings.HasPrefix(s, "-"):
		s, v, err = scanNumber(s[1:], 1, maxScanWidth)
		if err != nil {
			return "", err
		}
		// v is always in [0, math.MaxInt64] (scanNumber rejects overflow),
		// so negating it can never overflow.
		v = -v
	case f.signed && strings.HasPrefix(s, "+"):
		s, v, err = scanNumber(s[1:], 1, maxScanWidth)
		if err != nil {
			return "", err
		}
	default:
		s, v, err = scanNumber(s, 1, f.width)
		if err != nil {
			return "", err
		}
	}

	if err := f.set(p, v); err != nil {
		return "", err
	}
	return s, nil
}

func parseFixedItem(p *parsed, s string, fx fixKind) (string, error) {
	switch fx {
	case fxShortMonthName:
		rest, m0, err := scanShortMonth0(s)
		if err != nil {
			return "", err
		}
		if err := p.setMonth(int64(m0) + 1); err != nil {
			return "", err
		}
		return rest, nil

	case fxLongMonthName:
		rest, m0, err := scanShortOrLongMonth0(s)
		if err != nil {
			return "", err
		}
		if err := p.setMonth(int64(m0) + 1); err != nil {
			return "", err
		}
		return rest, nil

	case fxShortWeekdayName:
		rest, wd, err := scanShortWeekday(s)
		if err != nil {
			return "", err
		}
		if err := p.setWeekday(wd); err != nil {
			return "", err
		}
		return rest, nil

	case fxLongWeekdayName:
		rest, wd, err := scanShortOrLongWeekday(s)
		if err != nil {
			return "", err
		}
		if err := p.setWeekday(wd); err != nil {
			return "", err
		}
		return rest, nil

	case fxLowerAmPm, fxUpperAmPm:
		if len(s) < 2 {
			return "", errTooShort
		}
		var pm bool
		switch {
		case s[0]|32 == 'a' && s[1]|32 == 'm':
			pm = false
		case s[0]|32 == 'p' && s[1]|32 == 'm':
			pm = true
		default:
			return "", errInvalid
		}
		if err := p.setAmPm(pm); err != nil {
			return "", err
		}
		return s[2:], nil

	case fxNanosecond:
		if strings.HasPrefix(s, ".") {
			rest, n, err := scanNanosecond(s[1:])
			if err != nil {
				return "", err
			}
			if err := p.setNanosecond(int64(n)); err != nil {
				return "", err
			}
			return rest, nil
		}
		return s, nil

	case fxNanosecond3, fxNanosecond6, fxNanosecond9:
		if strings.HasPrefix(s, ".") {
			digits := map[fixKind]int{fxNanosecond3: 3, fxNanosecond6: 6, fxNanosecond9: 9}[fx]
			rest, n, err := scanNanosecondFixed(s[1:], digits)
			if err != nil {
				return "", err
			}
			if err := p.setNanosecond(n); err != nil {
				return "", err
			}
			return rest, nil
		}
		return s, nil

	case fxNanosecond3NoDot, fxNanosecond6NoDot, fxNanosecond9NoDot:
		digits := map[fixKind]int{fxNanosecond3NoDot: 3, fxNanosecond6NoDot: 6, fxNanosecond9NoDot: 9}[fx]
		if len(s) < digits {
			return "", errTooShort
		}
		rest, n, err := scanNanosecondFixed(s, digits)
		if err != nil {
			return "", err
		}
		if err := p.setNanosecond(n); err != nil {
			return "", err
		}
		return rest, nil

	case fxTimezoneName:
		i := 0
		for i < len(s) {
			r, sz := utf8.DecodeRuneInString(s[i:])
			if unicode.IsSpace(r) {
				break
			}
			i += sz
		}
		return s[i:], nil

	case fxTimezoneOffsetColon, fxTimezoneOffsetDoubleColon, fxTimezoneOffsetTripleColon, fxTimezoneOffset:
		rest, off, err := scanTimezoneOffset(strings.TrimLeftFunc(s, unicode.IsSpace), colonOrSpaceConsumer, false, false, true)
		if err != nil {
			return "", err
		}
		if err := p.setOffset(int64(off)); err != nil {
			return "", err
		}
		return rest, nil

	case fxTimezoneOffsetPermissive:
		rest, off, err := scanTimezoneOffset(strings.TrimLeftFunc(s, unicode.IsSpace), colonOrSpaceConsumer, true, true, true)
		if err != nil {
			return "", err
		}
		if err := p.setOffset(int64(off)); err != nil {
			return "", err
		}
		return rest, nil

	case fxRFC3339:
		return parseRFC3339Relaxed(p, s)
	}
	return "", errBadFormat
}

func colonOrSpaceConsumer(s string) (string, error) { return scanColonOrSpace(s), nil }

// rfc3339RelaxedDateItems and rfc3339RelaxedTimeItems mirror chrono's
// parse_rfc3339_relaxed DATE_ITEMS/TIME_ITEMS, used for the %+ specifier.
func rfc3339RelaxedDateItems() []item {
	return []item{
		numItemZero(numYear), spaceItem(""), litItem("-"),
		numItemZero(numMonth), spaceItem(""), litItem("-"),
		numItemZero(numDay),
	}
}

func rfc3339RelaxedTimeItems() []item {
	return []item{
		numItemZero(numHour), spaceItem(""), litItem(":"),
		numItemZero(numMinute), spaceItem(""), litItem(":"),
		numItemZero(numSecond), fixItem(fxNanosecond), spaceItem(""),
	}
}

// parseRFC3339Relaxed accepts a relaxed form of RFC 3339: it allows a space
// or 'T'/'t' as the date/time separator, spaces around components, and a
// literal "UTC" or "Z"/"z" in place of a numeric offset. It backs chrono's
// %+ / Fixed::RFC3339 specifier.
func parseRFC3339Relaxed(p *parsed, s string) (string, error) {
	s, err := parseItemsPrefix(p, s, rfc3339RelaxedDateItems())
	if err != nil {
		return "", err
	}

	if s == "" {
		return "", errTooShort
	}
	switch s[0] {
	case 't', 'T', ' ':
		s = s[1:]
	default:
		return "", errInvalid
	}

	s, err = parseItemsPrefix(p, s, rfc3339RelaxedTimeItems())
	if err != nil {
		return "", err
	}
	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	var offset int32
	if len(s) >= 3 && strings.EqualFold(s[:3], "UTC") {
		s = s[3:]
		offset = 0
	} else {
		s, offset, err = scanTimezoneOffset(s, colonOrSpaceConsumer, true, false, true)
		if err != nil {
			return "", err
		}
	}
	if err := p.setOffset(int64(offset)); err != nil {
		return "", err
	}
	return s, nil
}

// parseRFC2822 parses an RFC 2822 date-and-time string (the internet
// message format used by HTTP and email headers, e.g.
// "Tue, 1 Jul 2003 10:52:37 +0200"), mirroring chrono's parse_rfc2822.
func parseRFC2822(p *parsed, s string) (string, error) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	if rest, wd, err := scanShortWeekday(s); err == nil {
		if !strings.HasPrefix(rest, ",") {
			return "", errInvalid
		}
		s = rest[1:]
		if err := p.setWeekday(wd); err != nil {
			return "", err
		}
	}

	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	rest, day, err := scanNumber(s, 1, 2)
	if err != nil {
		return "", err
	}
	if err := p.setDay(day); err != nil {
		return "", err
	}
	s = rest

	s, err = scanSpace(s)
	if err != nil {
		return "", err
	}
	rest, month0, err := scanShortMonth0(s)
	if err != nil {
		return "", err
	}
	if err := p.setMonth(int64(month0) + 1); err != nil {
		return "", err
	}
	s = rest

	s, err = scanSpace(s)
	if err != nil {
		return "", err
	}

	prevLen := len(s)
	rest, year, err := scanNumber(s, 2, maxScanWidth)
	if err != nil {
		return "", err
	}
	yearLen := prevLen - len(rest)
	switch {
	case yearLen == 2 && year >= 0 && year <= 49:
		year += 2000
	case yearLen == 2 && year >= 50 && year <= 99:
		year += 1900
	case yearLen == 3:
		year += 1900
	}
	if err := p.setYear(year); err != nil {
		return "", err
	}
	s = rest

	s, err = scanSpace(s)
	if err != nil {
		return "", err
	}
	rest, hour, err := scanNumber(s, 2, 2)
	if err != nil {
		return "", err
	}
	if err := p.setHour(hour); err != nil {
		return "", err
	}
	s = rest

	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	s, err = scanChar(s, ':')
	if err != nil {
		return "", err
	}
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	rest, minute, err := scanNumber(s, 2, 2)
	if err != nil {
		return "", err
	}
	if err := p.setMinute(minute); err != nil {
		return "", err
	}
	s = rest

	if rest2, cerr := scanChar(strings.TrimLeftFunc(s, unicode.IsSpace), ':'); cerr == nil {
		rest2 = strings.TrimLeftFunc(rest2, unicode.IsSpace)
		rest3, sec, serr := scanNumber(rest2, 2, 2)
		if serr != nil {
			return "", serr
		}
		if err := p.setSecond(sec); err != nil {
			return "", err
		}
		s = rest3
	}

	s, err = scanSpace(s)
	if err != nil {
		return "", err
	}
	rest, off, err := scanTimezoneOffset2822(s)
	if err != nil {
		return "", err
	}
	if err := p.setOffset(int64(off)); err != nil {
		return "", err
	}
	s = rest

	for {
		rest, cerr := scanComment2822(s)
		if cerr != nil {
			break
		}
		s = rest
	}

	return s, nil
}
