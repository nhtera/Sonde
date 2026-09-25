// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// civilTime is the wall-clock time-of-day view of a time.Time, used for
// formatting.
type civilTime struct {
	hour, minute, second, nanosecond int
}

func civilTimeOf(t time.Time) civilTime {
	return civilTime{t.Hour(), t.Minute(), t.Second(), t.Nanosecond()}
}

// formatItems writes t (always taken as a UTC instant, per Format's
// contract) formatted with items to w.
func formatItems(w *strings.Builder, t time.Time, items []item) error {
	date := civilDate{t.Year(), int(t.Month()), t.Day()}
	tod := civilTimeOf(t)
	for _, it := range items {
		if err := formatOneItem(w, date, tod, t, it); err != nil {
			return err
		}
	}
	return nil
}

func formatOneItem(w *strings.Builder, date civilDate, tod civilTime, t time.Time, it item) error {
	switch it.kind {
	case itLiteral, itSpace:
		w.WriteString(it.text)
		return nil
	case itNumeric:
		return formatNumeric(w, date, tod, t, it.num, it.npad)
	case itFixed:
		return formatFixed(w, date, tod, it.fx)
	}
	return errBadFormat
}

func writeOne(w *strings.Builder, v uint8) { w.WriteByte('0' + v) }

func writeTwo(w *strings.Builder, v uint8, p pad) {
	ones := '0' + v%10
	switch {
	case v/10 == 0 && p == padNone:
	case v/10 == 0 && p == padSpace:
		w.WriteByte(' ')
	default:
		w.WriteByte('0' + v/10)
	}
	w.WriteByte(byte(ones))
}

// writeN formats v with at least n digits. alwaysSign forces an explicit
// sign even for non-negative values (used for out-of-range years).
func writeN(w *strings.Builder, n int, v int64, p pad, alwaysSign bool) {
	if alwaysSign {
		switch p {
		case padNone:
			fmt.Fprintf(w, "%+d", v)
		case padZero:
			fmt.Fprintf(w, "%+0"+strconv.Itoa(n+1)+"d", v)
		case padSpace:
			fmt.Fprintf(w, "%+"+strconv.Itoa(n+1)+"d", v)
		}
		return
	}
	switch p {
	case padNone:
		fmt.Fprintf(w, "%d", v)
	case padZero:
		fmt.Fprintf(w, "%0"+strconv.Itoa(n)+"d", v)
	case padSpace:
		fmt.Fprintf(w, "%"+strconv.Itoa(n)+"d", v)
	}
}

func writeHundreds(w *strings.Builder, n uint8) error {
	if n >= 100 {
		return errBadFormat
	}
	w.WriteByte('0' + n/10)
	w.WriteByte('0' + n%10)
	return nil
}

func writeYear(w *strings.Builder, year int32, p pad) {
	if year >= 1000 && year <= 9999 {
		_ = writeHundreds(w, uint8(year/100)) //nolint:gosec // G115: year is in [1000, 9999], so year/100 is in [10, 99]
		_ = writeHundreds(w, uint8(year%100)) //nolint:gosec // G115: year%100 is always in [0, 99]
		return
	}
	writeN(w, 4, int64(year), p, year < 0 || year >= 10000)
}

func formatNumeric(w *strings.Builder, date civilDate, tod civilTime, t time.Time, n numKind, p pad) error {
	switch n {
	case numYear:
		writeYear(w, int32(date.year), p) //nolint:gosec // G115: years beyond int32 are outside this package's supported range (see doc.go)
	case numYearDiv100:
		// Deliberately mirrors chrono's own `div_euclid(100) as u8`, which
		// wraps for negative centuries (years before 1 BCE): see
		// TestFormatNegativeYearCentury.
		writeTwo(w, uint8(floorDiv(date.year, 100)), p) //nolint:gosec // G115: intentionally reproduces chrono's wrapping cast
	case numYearMod100:
		writeTwo(w, uint8(floorMod(date.year, 100)), p) //nolint:gosec // G115: floorMod(_, 100) is always in [0, 100)
	case numIsoYear:
		isoYear, _ := date.isoYearWeek()
		writeYear(w, int32(isoYear), p) //nolint:gosec // G115: years beyond int32 are outside this package's supported range (see doc.go)
	case numIsoYearMod100:
		isoYear, _ := date.isoYearWeek()
		writeTwo(w, uint8(floorMod(isoYear, 100)), p) //nolint:gosec // G115: floorMod(_, 100) is always in [0, 100)
	case numQuarter:
		writeOne(w, uint8(date.quarter())) //nolint:gosec // G115: quarter() is always in [1, 4]
	case numMonth:
		writeTwo(w, uint8(date.month), p) //nolint:gosec // G115: date.month is always in [1, 12]
	case numDay:
		writeTwo(w, uint8(date.day), p) //nolint:gosec // G115: date.day is always in [1, 31]
	case numWeekFromSun:
		writeTwo(w, uint8(date.weeksFrom(time.Sunday)), p) //nolint:gosec // G115: weeksFrom is always in [0, 53]
	case numWeekFromMon:
		writeTwo(w, uint8(date.weeksFrom(time.Monday)), p) //nolint:gosec // G115: weeksFrom is always in [0, 53]
	case numIsoWeek:
		_, isoWeek := date.isoYearWeek()
		writeTwo(w, uint8(isoWeek), p) //nolint:gosec // G115: isoWeek is always in [1, 53]
	case numNumDaysFromSun:
		writeOne(w, uint8(date.weekday())) //nolint:gosec // G115: time.Weekday is always in [0, 6]
	case numWeekdayFromMon:
		wd := date.weekday()
		numberFromMonday := (int(wd)+6)%7 + 1
		writeOne(w, uint8(numberFromMonday)) //nolint:gosec // G115: numberFromMonday is always in [1, 7]
	case numOrdinal:
		writeN(w, 3, int64(date.ordinal()), p, false)
	case numHour:
		writeTwo(w, uint8(tod.hour), p) //nolint:gosec // G115: time.Time.Hour() is always in [0, 23]
	case numHour12:
		h := tod.hour % 12
		if h == 0 {
			h = 12
		}
		writeTwo(w, uint8(h), p) //nolint:gosec // G115: h is always in [1, 12]
	case numMinute:
		writeTwo(w, uint8(tod.minute), p) //nolint:gosec // G115: time.Time.Minute() is always in [0, 59]
	case numSecond:
		writeTwo(w, uint8(tod.second), p) //nolint:gosec // G115: time.Time.Second() is always in [0, 59]
	case numNanosecond:
		writeN(w, 9, int64(tod.nanosecond), p, false)
	case numTimestamp:
		writeN(w, 9, t.Unix(), p, false)
	default:
		return errBadFormat
	}
	return nil
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int) int {
	m := a % b
	if m != 0 && ((a < 0) != (b < 0)) {
		m += b
	}
	return m
}

func formatFixed(w *strings.Builder, date civilDate, tod civilTime, fx fixKind) error {
	switch fx {
	case fxShortMonthName:
		w.WriteString(shortMonths()[date.month-1])
	case fxLongMonthName:
		w.WriteString(longMonths()[date.month-1])
	case fxShortWeekdayName:
		w.WriteString(shortWeekdays()[int(date.weekday())])
	case fxLongWeekdayName:
		w.WriteString(longWeekdays()[int(date.weekday())])
	case fxLowerAmPm:
		w.WriteString(strings.ToLower(amPmName(tod)))
	case fxUpperAmPm:
		w.WriteString(amPmName(tod))
	case fxNanosecond:
		writeFractionalNanosecond(w, tod.nanosecond)
	case fxNanosecond3:
		w.WriteString(decimalPoint)
		fmt.Fprintf(w, "%03d", tod.nanosecond/1_000_000%1000)
	case fxNanosecond6:
		w.WriteString(decimalPoint)
		fmt.Fprintf(w, "%06d", tod.nanosecond/1_000%1_000_000)
	case fxNanosecond9:
		w.WriteString(decimalPoint)
		fmt.Fprintf(w, "%09d", tod.nanosecond%1_000_000_000)
	case fxNanosecond3NoDot:
		fmt.Fprintf(w, "%03d", tod.nanosecond/1_000_000%1000)
	case fxNanosecond6NoDot:
		fmt.Fprintf(w, "%06d", tod.nanosecond/1_000%1_000_000)
	case fxNanosecond9NoDot:
		fmt.Fprintf(w, "%09d", tod.nanosecond%1_000_000_000)
	case fxTimezoneName:
		// Format() always treats its input as a UTC instant, matching
		// chrono's DateTime<Utc> whose Offset Display is "UTC".
		w.WriteString("UTC")
	case fxTimezoneOffset, fxTimezoneOffsetColon, fxTimezoneOffsetDoubleColon, fxTimezoneOffsetTripleColon:
		return formatOffsetItem(w, fx)
	case fxRFC3339:
		writeRFC3339(w, date, tod)
	case fxTimezoneOffsetPermissive:
		// Parse-only, per chrono's Fixed::Internal(TimezoneOffsetPermissive).
		return errBadFormat
	default:
		return errBadFormat
	}
	return nil
}

func amPmName(tod civilTime) string {
	names := amPmNames()
	if tod.hour >= 12 {
		return names[1]
	}
	return names[0]
}

func writeFractionalNanosecond(w *strings.Builder, nano int) {
	if nano == 0 {
		return
	}
	w.WriteString(decimalPoint)
	switch {
	case nano%1_000_000 == 0:
		fmt.Fprintf(w, "%03d", nano/1_000_000)
	case nano%1_000 == 0:
		fmt.Fprintf(w, "%06d", nano/1_000)
	default:
		fmt.Fprintf(w, "%09d", nano)
	}
}

type offsetFormat struct {
	precision offsetPrecision
	colon     bool
	allowZulu bool
	padding   pad
}

type offsetPrecision int

const (
	offPrecHours offsetPrecision = iota
	offPrecMinutes
	offPrecSeconds
)

// formatOffset writes offsetSeconds (east of UTC) per f. Format() always
// uses offsetSeconds == 0 (UTC), but this is kept general to match
// chrono's OffsetFormat exactly.
func formatOffset(w *strings.Builder, f offsetFormat, offsetSeconds int32) {
	if f.allowZulu && offsetSeconds == 0 {
		w.WriteByte('Z')
		return
	}
	sign := byte('+')
	off := offsetSeconds
	if off < 0 {
		sign = '-'
		off = -off
	}

	var hours, mins, secs uint8
	precision := f.precision
	// formatOffset is only ever called with offsets from a valid UTC offset
	// (this package's own callers pass 0, and Parsed rejects anything
	// |offset| >= 24h before it reaches here), so hours/mins/secs always
	// fit a uint8; mirrors chrono's own unchecked `as u8` in OffsetFormat.
	switch f.precision {
	case offPrecHours:
		hours = uint8(off / 3600) //nolint:gosec // G115: see comment above
	case offPrecMinutes:
		minutes := (off + 30) / 60
		mins = uint8(minutes % 60)  //nolint:gosec // G115: see comment above
		hours = uint8(minutes / 60) //nolint:gosec // G115: see comment above
	case offPrecSeconds:
		minutes := off / 60
		secs = uint8(off % 60)      //nolint:gosec // G115: see comment above
		mins = uint8(minutes % 60)  //nolint:gosec // G115: see comment above
		hours = uint8(minutes / 60) //nolint:gosec // G115: see comment above
	}

	if hours < 10 {
		if f.padding == padSpace {
			w.WriteByte(' ')
		}
		w.WriteByte(sign)
		if f.padding == padZero {
			w.WriteByte('0')
		}
		w.WriteByte('0' + hours)
	} else {
		w.WriteByte(sign)
		_ = writeHundreds(w, hours)
	}
	if precision == offPrecMinutes || precision == offPrecSeconds {
		if f.colon {
			w.WriteByte(':')
		}
		_ = writeHundreds(w, mins)
	}
	if precision == offPrecSeconds {
		if f.colon {
			w.WriteByte(':')
		}
		_ = writeHundreds(w, secs)
	}
}

func formatOffsetItem(w *strings.Builder, fx fixKind) error {
	var f offsetFormat
	f.padding = padZero
	switch fx {
	case fxTimezoneOffset:
		f = offsetFormat{precision: offPrecMinutes, colon: false, allowZulu: false, padding: padZero}
	case fxTimezoneOffsetColon:
		f = offsetFormat{precision: offPrecMinutes, colon: true, allowZulu: false, padding: padZero}
	case fxTimezoneOffsetDoubleColon:
		f = offsetFormat{precision: offPrecSeconds, colon: true, allowZulu: false, padding: padZero}
	case fxTimezoneOffsetTripleColon:
		f = offsetFormat{precision: offPrecHours, colon: false, allowZulu: false, padding: padZero}
	default:
		return errBadFormat
	}
	formatOffset(w, f, 0)
	return nil
}

// writeRFC3339 writes date/tod like "%Y-%m-%dT%H:%M:%S%.fZ" (chrono's %+
// specifier, always with a zero UTC offset since Format() treats its input
// as a UTC instant).
func writeRFC3339(w *strings.Builder, date civilDate, tod civilTime) {
	year := date.year
	if year >= 0 && year <= 9999 {
		_ = writeHundreds(w, uint8(year/100)) //nolint:gosec // G115: year is in [0, 9999], so year/100 is in [0, 99]
		_ = writeHundreds(w, uint8(year%100)) //nolint:gosec // G115: year%100 is always in [0, 99]
	} else {
		fmt.Fprintf(w, "%+05d", year)
	}
	w.WriteByte('-')
	_ = writeHundreds(w, uint8(date.month)) //nolint:gosec // G115: date.month is always in [1, 12]
	w.WriteByte('-')
	_ = writeHundreds(w, uint8(date.day)) //nolint:gosec // G115: date.day is always in [1, 31]
	w.WriteByte('T')
	_ = writeHundreds(w, uint8(tod.hour)) //nolint:gosec // G115: time.Time.Hour() is always in [0, 23]
	w.WriteByte(':')
	_ = writeHundreds(w, uint8(tod.minute)) //nolint:gosec // G115: time.Time.Minute() is always in [0, 59]
	w.WriteByte(':')
	_ = writeHundreds(w, uint8(tod.second)) //nolint:gosec // G115: time.Time.Second() is always in [0, 59]
	writeFractionalNanosecond(w, tod.nanosecond)
	formatOffset(w, offsetFormat{precision: offPrecMinutes, colon: true, allowZulu: false, padding: padZero}, 0)
}
