// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// pad is the padding behavior of a numeric formatting item.
type pad uint8

const (
	padNone pad = iota
	padZero
	padSpace
)

// numKind identifies a numeric formatting item.
type numKind uint8

const (
	numYear numKind = iota
	numYearDiv100
	numYearMod100
	numIsoYear
	numIsoYearMod100
	numQuarter
	numMonth
	numDay
	numWeekFromSun
	numWeekFromMon
	numIsoWeek
	numNumDaysFromSun
	numWeekdayFromMon
	numOrdinal
	numHour
	numHour12
	numMinute
	numSecond
	numNanosecond
	numTimestamp
)

// fixKind identifies a fixed-format formatting item: one with its own
// formatting and parsing rules rather than a plain zero/space-padded number.
type fixKind uint8

const (
	fxShortMonthName fixKind = iota
	fxLongMonthName
	fxShortWeekdayName
	fxLongWeekdayName
	fxLowerAmPm
	fxUpperAmPm
	fxNanosecond
	fxNanosecond3
	fxNanosecond6
	fxNanosecond9
	fxTimezoneName
	fxTimezoneOffsetColon
	fxTimezoneOffsetDoubleColon
	fxTimezoneOffsetTripleColon
	fxTimezoneOffset
	fxRFC3339
	// fxTimezoneOffsetPermissive is parse-only (chrono's %#z / internal
	// TimezoneOffsetPermissive); formatting it is an error.
	fxTimezoneOffsetPermissive
	fxNanosecond3NoDot
	fxNanosecond6NoDot
	fxNanosecond9NoDot
)

type itemKind uint8

const (
	itLiteral itemKind = iota
	itSpace
	itNumeric
	itFixed
)

// item is a single formatting item, used for both formatting and parsing.
// It mirrors chrono's format::Item.
type item struct {
	kind itemKind
	text string // for itLiteral and itSpace
	num  numKind
	npad pad
	fx   fixKind
}

func litItem(s string) item         { return item{kind: itLiteral, text: s} }
func spaceItem(s string) item       { return item{kind: itSpace, text: s} }
func numItem(n numKind, p pad) item { return item{kind: itNumeric, num: n, npad: p} }
func fixItem(f fixKind) item        { return item{kind: itFixed, fx: f} }
func numItemNone(n numKind) item    { return numItem(n, padNone) }
func numItemZero(n numKind) item    { return numItem(n, padZero) }
func numItemSpace(n numKind) item   { return numItem(n, padSpace) }

// tFmtItems is chrono's T_FMT: "%H:%M:%S", used for both %T and %X (the
// English/POSIX locale has no distinct locale time format).
func tFmtItems() []item {
	return []item{numItemZero(numHour), litItem(":"), numItemZero(numMinute), litItem(":"), numItemZero(numSecond)}
}

// dFmtItems is chrono's D_FMT: "%m/%d/%y", used for both %D and %x.
func dFmtItems() []item {
	return []item{numItemZero(numMonth), litItem("/"), numItemZero(numDay), litItem("/"), numItemZero(numYearMod100)}
}

// dTFmtItems is chrono's D_T_FMT, used for %c: "%a %b %e %H:%M:%S %Y".
func dTFmtItems() []item {
	return []item{
		fixItem(fxShortWeekdayName), spaceItem(" "),
		fixItem(fxShortMonthName), spaceItem(" "),
		numItemSpace(numDay), spaceItem(" "),
		numItemZero(numHour), litItem(":"), numItemZero(numMinute), litItem(":"), numItemZero(numSecond),
		spaceItem(" "), numItemZero(numYear),
	}
}

// tFmtAmPmItems is chrono's T_FMT_AMPM, used for %r: "%I:%M:%S %p".
func tFmtAmPmItems() []item {
	return []item{
		numItemZero(numHour12), litItem(":"), numItemZero(numMinute), litItem(":"), numItemZero(numSecond),
		spaceItem(" "), fixItem(fxUpperAmPm),
	}
}

// tokenizeFormat parses a strftime-like format string into formatting
// items, mirroring chrono's StrftimeItems (non-lenient, non-locale).
//
// It fails fast on the first invalid or unsupported specifier: chrono's
// formatter and parser both abort as soon as they encounter such an item,
// so producing items past that point would never be observed.
func tokenizeFormat(format string) ([]item, error) {
	var out []item
	remainder := format
	for len(remainder) > 0 {
		items, rest, err := parseNextItem(remainder)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		remainder = rest
	}
	return out, nil
}

func nextRune(s string) (rune, string, bool) {
	if s == "" {
		return 0, s, false
	}
	r, size := utf8.DecodeRuneInString(s)
	return r, s[size:], true
}

func parseNextItem(remainder string) ([]item, string, error) {
	r, rest, ok := nextRune(remainder)
	if !ok {
		return nil, remainder, nil
	}

	if r == '%' {
		return parseSpecifier(rest)
	}

	if unicode.IsSpace(r) {
		i := 0
		for i < len(remainder) {
			rr, sz := utf8.DecodeRuneInString(remainder[i:])
			if !unicode.IsSpace(rr) {
				break
			}
			i += sz
		}
		return []item{spaceItem(remainder[:i])}, remainder[i:], nil
	}

	i := 0
	for i < len(remainder) {
		rr, sz := utf8.DecodeRuneInString(remainder[i:])
		if unicode.IsSpace(rr) || rr == '%' {
			break
		}
		i += sz
	}
	return []item{litItem(remainder[:i])}, remainder[i:], nil
}

// parseSpecifier parses one "%..." specifier, given the string right after
// the '%'. It returns the items it expands to and the remaining string.
func parseSpecifier(rest string) ([]item, string, error) {
	spec, rest, ok := nextRune(rest)
	if !ok {
		return nil, "", errBadFormat
	}

	var padOverride *pad
	switch spec {
	case '-':
		p := padNone
		padOverride = &p
	case '0':
		p := padZero
		padOverride = &p
	case '_':
		p := padSpace
		padOverride = &p
	}
	isAlternate := spec == '#'
	if padOverride != nil || isAlternate {
		spec2, rest2, ok2 := nextRune(rest)
		if !ok2 {
			return nil, "", errBadFormat
		}
		spec, rest = spec2, rest2
	}
	if isAlternate && spec != 'z' {
		return nil, "", errBadFormat
	}

	var items []item
	switch spec {
	case 'A':
		items = []item{fixItem(fxLongWeekdayName)}
	case 'B':
		items = []item{fixItem(fxLongMonthName)}
	case 'C':
		items = []item{numItemZero(numYearDiv100)}
	case 'D':
		items = []item{numItemZero(numMonth), litItem("/"), numItemZero(numDay), litItem("/"), numItemZero(numYearMod100)}
	case 'F':
		items = []item{numItemZero(numYear), litItem("-"), numItemZero(numMonth), litItem("-"), numItemZero(numDay)}
	case 'G':
		items = []item{numItemZero(numIsoYear)}
	case 'H':
		items = []item{numItemZero(numHour)}
	case 'I':
		items = []item{numItemZero(numHour12)}
	case 'M':
		items = []item{numItemZero(numMinute)}
	case 'P':
		items = []item{fixItem(fxLowerAmPm)}
	case 'R':
		items = []item{numItemZero(numHour), litItem(":"), numItemZero(numMinute)}
	case 'S':
		items = []item{numItemZero(numSecond)}
	case 'T':
		items = tFmtItems()
	case 'U':
		items = []item{numItemZero(numWeekFromSun)}
	case 'V':
		items = []item{numItemZero(numIsoWeek)}
	case 'W':
		items = []item{numItemZero(numWeekFromMon)}
	case 'X':
		items = tFmtItems()
	case 'Y':
		items = []item{numItemZero(numYear)}
	case 'Z':
		items = []item{fixItem(fxTimezoneName)}
	case 'a':
		items = []item{fixItem(fxShortWeekdayName)}
	case 'b', 'h':
		items = []item{fixItem(fxShortMonthName)}
	case 'c':
		items = dTFmtItems()
	case 'd':
		items = []item{numItemZero(numDay)}
	case 'e':
		items = []item{numItemSpace(numDay)}
	case 'f':
		items = []item{numItemZero(numNanosecond)}
	case 'g':
		items = []item{numItemZero(numIsoYearMod100)}
	case 'j':
		items = []item{numItemZero(numOrdinal)}
	case 'k':
		items = []item{numItemSpace(numHour)}
	case 'l':
		items = []item{numItemSpace(numHour12)}
	case 'm':
		items = []item{numItemZero(numMonth)}
	case 'n':
		items = []item{spaceItem("\n")}
	case 'p':
		items = []item{fixItem(fxUpperAmPm)}
	case 'q':
		items = []item{numItemNone(numQuarter)}
	case 'r':
		items = tFmtAmPmItems()
	case 's':
		items = []item{numItemNone(numTimestamp)}
	case 't':
		items = []item{spaceItem("\t")}
	case 'u':
		items = []item{numItemNone(numWeekdayFromMon)}
	case 'v':
		items = []item{numItemSpace(numDay), litItem("-"), fixItem(fxShortMonthName), litItem("-"), numItemZero(numYear)}
	case 'w':
		items = []item{numItemNone(numNumDaysFromSun)}
	case 'x':
		items = dFmtItems()
	case 'y':
		items = []item{numItemZero(numYearMod100)}
	case 'z':
		if isAlternate {
			items = []item{fixItem(fxTimezoneOffsetPermissive)}
		} else {
			items = []item{fixItem(fxTimezoneOffset)}
		}
	case '+':
		items = []item{fixItem(fxRFC3339)}
	case ':':
		switch {
		case strings.HasPrefix(rest, "::z"):
			rest = rest[3:]
			items = []item{fixItem(fxTimezoneOffsetTripleColon)}
		case strings.HasPrefix(rest, ":z"):
			rest = rest[2:]
			items = []item{fixItem(fxTimezoneOffsetDoubleColon)}
		case strings.HasPrefix(rest, "z"):
			rest = rest[1:]
			items = []item{fixItem(fxTimezoneOffsetColon)}
		default:
			return nil, "", errBadFormat
		}
	case '.':
		r2, rest2, ok2 := nextRune(rest)
		if !ok2 {
			return nil, "", errBadFormat
		}
		switch r2 {
		case '3', '6', '9':
			r3, rest3, ok3 := nextRune(rest2)
			if !ok3 || r3 != 'f' {
				return nil, "", errBadFormat
			}
			rest = rest3
			switch r2 {
			case '3':
				items = []item{fixItem(fxNanosecond3)}
			case '6':
				items = []item{fixItem(fxNanosecond6)}
			case '9':
				items = []item{fixItem(fxNanosecond9)}
			}
		case 'f':
			items = []item{fixItem(fxNanosecond)}
			rest = rest2
		default:
			return nil, "", errBadFormat
		}
	case '3', '6', '9':
		r2, rest2, ok2 := nextRune(rest)
		if !ok2 || r2 != 'f' {
			return nil, "", errBadFormat
		}
		rest = rest2
		switch spec {
		case '3':
			items = []item{fixItem(fxNanosecond3NoDot)}
		case '6':
			items = []item{fixItem(fxNanosecond6NoDot)}
		case '9':
			items = []item{fixItem(fxNanosecond9NoDot)}
		}
	case '%':
		items = []item{litItem("%")}
	default:
		return nil, "", errBadFormat
	}

	if padOverride != nil {
		if len(items) != 1 || items[0].kind != itNumeric {
			return nil, "", errBadFormat
		}
		items[0].npad = *padOverride
	}
	return items, rest, nil
}
