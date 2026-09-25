// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import "time"

// opt is a small Option[T] substitute: a value together with whether it has
// been set. The zero value is "unset".
type opt[T comparable] struct {
	v  T
	ok bool
}

// set assigns val, or checks it is consistent with a previously set value.
// It mirrors chrono's set_if_consistent: setting the same field twice with
// two different values is an error.
func (o *opt[T]) set(val T) error {
	if o.ok && o.v != val {
		return errImpossible
	}
	o.v, o.ok = val, true
	return nil
}

func (o opt[T]) get() (T, bool) { return o.v, o.ok }

// parsed holds parsed fields of a date and time that can be checked for
// consistency and resolved into a concrete value. It mirrors chrono's
// format::Parsed and its resolution algorithm.
type parsed struct {
	year          opt[int32]
	yearDiv100    opt[int32]
	yearMod100    opt[int32]
	isoYear       opt[int32]
	isoYearMod100 opt[int32]
	quarter       opt[uint32]
	month         opt[uint32]
	weekFromSun   opt[uint32]
	weekFromMon   opt[uint32]
	isoWeek       opt[uint32]
	weekday       opt[time.Weekday]
	ordinal       opt[uint32]
	day           opt[uint32]
	hourDiv12     opt[uint32] // 0 = am, 1 = pm
	hourMod12     opt[uint32] // 0..11
	minute        opt[uint32]
	second        opt[uint32] // 0..60, 60 is a leap second
	nanosecond    opt[uint32]
	timestamp     opt[int64]
	offset        opt[int32] // seconds east of UTC
}

func (p *parsed) setYear(v int64) error {
	if v < minInt32 || v > maxInt32 {
		return errOutOfRange
	}
	return p.year.set(int32(v))
}

func (p *parsed) setYearDiv100(v int64) error {
	if v < 0 || v > maxInt32 {
		return errOutOfRange
	}
	return p.yearDiv100.set(int32(v))
}

func (p *parsed) setYearMod100(v int64) error {
	if v < 0 || v > 99 {
		return errOutOfRange
	}
	return p.yearMod100.set(int32(v))
}

func (p *parsed) setIsoYear(v int64) error {
	if v < minInt32 || v > maxInt32 {
		return errOutOfRange
	}
	return p.isoYear.set(int32(v))
}

func (p *parsed) setIsoYearMod100(v int64) error {
	if v < 0 || v > 99 {
		return errOutOfRange
	}
	return p.isoYearMod100.set(int32(v))
}

func (p *parsed) setQuarter(v int64) error {
	if v < 1 || v > 4 {
		return errOutOfRange
	}
	return p.quarter.set(uint32(v))
}

func (p *parsed) setMonth(v int64) error {
	if v < 1 || v > 12 {
		return errOutOfRange
	}
	return p.month.set(uint32(v))
}

func (p *parsed) setWeekFromSun(v int64) error {
	if v < 0 || v > 53 {
		return errOutOfRange
	}
	return p.weekFromSun.set(uint32(v))
}

func (p *parsed) setWeekFromMon(v int64) error {
	if v < 0 || v > 53 {
		return errOutOfRange
	}
	return p.weekFromMon.set(uint32(v))
}

func (p *parsed) setIsoWeek(v int64) error {
	if v < 1 || v > 53 {
		return errOutOfRange
	}
	return p.isoWeek.set(uint32(v))
}

func (p *parsed) setWeekday(v time.Weekday) error {
	return p.weekday.set(v)
}

func (p *parsed) setOrdinal(v int64) error {
	if v < 1 || v > 366 {
		return errOutOfRange
	}
	return p.ordinal.set(uint32(v))
}

func (p *parsed) setDay(v int64) error {
	if v < 1 || v > 31 {
		return errOutOfRange
	}
	return p.day.set(uint32(v))
}

func (p *parsed) setAmPm(pm bool) error {
	v := uint32(0)
	if pm {
		v = 1
	}
	return p.hourDiv12.set(v)
}

func (p *parsed) setHour12(v int64) error {
	if v < 1 || v > 12 {
		return errOutOfRange
	}
	if v == 12 {
		v = 0
	}
	return p.hourMod12.set(uint32(v))
}

func (p *parsed) setHour(v int64) error {
	var div, mod uint32
	switch {
	case v >= 0 && v <= 11:
		div, mod = 0, uint32(v)
	case v >= 12 && v <= 23:
		div, mod = 1, uint32(v)-12
	default:
		return errOutOfRange
	}
	if err := p.hourDiv12.set(div); err != nil {
		return err
	}
	return p.hourMod12.set(mod)
}

func (p *parsed) setMinute(v int64) error {
	if v < 0 || v > 59 {
		return errOutOfRange
	}
	return p.minute.set(uint32(v))
}

func (p *parsed) setSecond(v int64) error {
	if v < 0 || v > 60 {
		return errOutOfRange
	}
	return p.second.set(uint32(v))
}

func (p *parsed) setNanosecond(v int64) error {
	if v < 0 || v > 999_999_999 {
		return errOutOfRange
	}
	return p.nanosecond.set(uint32(v))
}

func (p *parsed) setTimestamp(v int64) error {
	return p.timestamp.set(v)
}

func (p *parsed) setOffset(v int64) error {
	if v < minInt32 || v > maxInt32 {
		return errOutOfRange
	}
	return p.offset.set(int32(v))
}

const (
	minInt32 = -1 << 31
	maxInt32 = 1<<31 - 1
)

// setWeekdayFromNumDaysFromSun sets weekday from a 0(Sun)..6(Sat) value, as
// used by %w.
func (p *parsed) setWeekdayFromNumDaysFromSun(v int64) error {
	if v < 0 || v > 6 {
		return errOutOfRange
	}
	return p.setWeekday(time.Weekday(v))
}

// setWeekdayFromNumberFromMonday sets weekday from a 1(Mon)..7(Sun) value,
// as used by %u.
func (p *parsed) setWeekdayFromNumberFromMonday(v int64) error {
	if v < 1 || v > 7 {
		return errOutOfRange
	}
	wd := time.Weekday(v % 7) // 7 -> Sunday (0)
	return p.setWeekday(wd)
}

// civilDate is a Gregorian calendar date, valid across the range this
// package supports.
type civilDate struct {
	year, month, day int
}

// t returns the civil date at midnight UTC.
func (d civilDate) t() time.Time {
	return time.Date(d.year, time.Month(d.month), d.day, 0, 0, 0, 0, time.UTC)
}

func (d civilDate) weekday() time.Weekday { return d.t().Weekday() }
func (d civilDate) ordinal() int          { return d.t().YearDay() }
func (d civilDate) quarter() int          { return (d.month-1)/3 + 1 }

func (d civilDate) isoYearWeek() (int, int) {
	y, w := d.t().ISOWeek()
	return y, w
}

// weeksFrom returns the week number of d, where week 1 starts at the first
// occurrence of `start` weekday in the year; days before that are week 0.
func (d civilDate) weeksFrom(start time.Weekday) int {
	jan1 := civilDate{d.year, 1, 1}.weekday()
	firstOffset := (int(start) - int(jan1) + 7) % 7
	dayIdx := d.ordinal() - 1
	if dayIdx < firstOffset {
		return 0
	}
	return (dayIdx-firstOffset)/7 + 1
}

// civilDateFromYMD builds a civil date, returning OutOfRange if the
// year/month/day combination does not exist (e.g. day 30 in February).
func civilDateFromYMD(year, month, day int) (civilDate, error) {
	if month < 1 || month > 12 || day < 1 {
		return civilDate{}, errOutOfRange
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return civilDate{}, errOutOfRange
	}
	return civilDate{year, month, day}, nil
}

// civilDateFromOrdinal builds a civil date from a year and day-of-year.
func civilDateFromOrdinal(year, ordinal int) (civilDate, error) {
	if ordinal < 1 || ordinal > 366 {
		return civilDate{}, errOutOfRange
	}
	t := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, ordinal-1)
	if t.Year() != year {
		return civilDate{}, errOutOfRange
	}
	return civilDate{year, int(t.Month()), t.Day()}, nil
}

// resolveWeekDate resolves a (year, week, weekday) triple where week 1
// starts at the first occurrence of weekStart in the year, mirroring
// chrono's resolve_week_date.
func resolveWeekDate(year, week int, weekday, weekStart time.Weekday) (civilDate, error) {
	if week > 53 {
		return civilDate{}, errOutOfRange
	}
	firstDay, err := civilDateFromOrdinal(year, 1)
	if err != nil {
		return civilDate{}, errOutOfRange
	}
	firstWeekStart := 1 + daysSince(weekStart, firstDay.weekday())
	weekdayOffset := daysSince(weekday, weekStart)
	ordinal := firstWeekStart + (week-1)*7 + weekdayOffset
	if ordinal <= 0 {
		return civilDate{}, errImpossible
	}
	d, err := civilDateFromOrdinal(year, ordinal)
	if err != nil {
		return civilDate{}, errImpossible
	}
	return d, nil
}

// daysSince returns the number of days from `base` to reach `target`,
// going forward, in [0, 6].
func daysSince(target, base time.Weekday) int {
	return ((int(target)-int(base))%7 + 7) % 7
}

// civilDateFromISOYearWeekday resolves an ISO 8601 week date into a civil
// date, verifying the result actually falls in the requested ISO week.
func civilDateFromISOYearWeekday(isoYear, isoWeek int, weekday time.Weekday) (civilDate, error) {
	// ISO week 1 is the week containing the first Thursday of the year,
	// equivalently the week containing January 4th.
	jan4 := civilDate{isoYear, 1, 4}
	jan4ISOWeekday := daysFromMonday(jan4.weekday()) // 0=Mon..6=Sun
	weekOneMonday := jan4.t().AddDate(0, 0, -jan4ISOWeekday)
	isoWeekday := daysFromMonday(weekday) // 0=Mon..6=Sun
	t := weekOneMonday.AddDate(0, 0, (isoWeek-1)*7+isoWeekday)
	d := civilDate{t.Year(), int(t.Month()), t.Day()}
	gotYear, gotWeek := d.isoYearWeek()
	if gotYear != isoYear || gotWeek != isoWeek {
		return civilDate{}, errOutOfRange
	}
	return d, nil
}

// toNaiveDate resolves the date fields, checking every given field is
// consistent with every other, mirroring chrono's Parsed::to_naive_date.
func (p *parsed) toNaiveDate() (civilDate, error) {
	givenYear, err := resolveYear(p.year, p.yearDiv100, p.yearMod100)
	if err != nil {
		return civilDate{}, err
	}
	givenIsoYear, err := resolveYear(p.isoYear, opt[int32]{}, p.isoYearMod100)
	if err != nil {
		return civilDate{}, err
	}

	verifyYMD := func(d civilDate) bool {
		year := int32(d.year) //nolint:gosec // G115: d.year is within this package's supported year range (see doc.go)
		var yearDiv100, yearMod100 opt[int32]
		if year >= 0 {
			yearDiv100 = opt[int32]{v: year / 100, ok: true}
			yearMod100 = opt[int32]{v: year % 100, ok: true}
		}
		if y, ok := p.year.get(); ok && y != year {
			return false
		}
		if v, ok := p.yearDiv100.get(); ok {
			want, hasWant := yearDiv100.get()
			if !hasWant || v != want {
				return false
			}
		}
		if v, ok := p.yearMod100.get(); ok {
			want, hasWant := yearMod100.get()
			if !hasWant || v != want {
				return false
			}
		}
		if m, ok := p.month.get(); ok && m != uint32(d.month) { //nolint:gosec // G115: d.month is always in [1, 12]
			return false
		}
		if dd, ok := p.day.get(); ok && dd != uint32(d.day) { //nolint:gosec // G115: d.day is always in [1, 31]
			return false
		}
		return true
	}

	verifyISOWeekDate := func(d civilDate) bool {
		isoYear, isoWeek := d.isoYearWeek()
		wd := d.weekday()
		var isoYearMod100 opt[int32]
		if isoYear >= 0 {
			isoYearMod100 = opt[int32]{v: int32(isoYear % 100), ok: true}
		}
		if y, ok := p.isoYear.get(); ok && y != int32(isoYear) { //nolint:gosec // G115: isoYear is within this package's supported year range (see doc.go)
			return false
		}
		if v, ok := p.isoYearMod100.get(); ok {
			want, hasWant := isoYearMod100.get()
			if !hasWant || v != want {
				return false
			}
		}
		if w, ok := p.isoWeek.get(); ok && w != uint32(isoWeek) { //nolint:gosec // G115: isoWeek is always in [1, 53]
			return false
		}
		if wdParsed, ok := p.weekday.get(); ok && wdParsed != wd {
			return false
		}
		return true
	}

	verifyOrdinal := func(d civilDate) bool {
		ordinal := d.ordinal()
		weekFromSun := d.weeksFrom(time.Sunday)
		weekFromMon := d.weeksFrom(time.Monday)
		if o, ok := p.ordinal.get(); ok && o != uint32(ordinal) { //nolint:gosec // G115: ordinal() is always in [1, 366]
			return false
		}
		if w, ok := p.weekFromSun.get(); ok && int(w) != weekFromSun {
			return false
		}
		if w, ok := p.weekFromMon.get(); ok && int(w) != weekFromMon {
			return false
		}
		return true
	}

	var verified bool
	var date civilDate

	switch {
	case givenYear != nil && func() bool { _, ok1 := p.month.get(); _, ok2 := p.day.get(); return ok1 && ok2 }():
		month, _ := p.month.get()
		day, _ := p.day.get()
		d, derr := civilDateFromYMD(int(*givenYear), int(month), int(day))
		if derr != nil {
			return civilDate{}, errOutOfRange
		}
		date = d
		verified = verifyISOWeekDate(d) && verifyOrdinal(d)

	case givenYear != nil && func() bool { _, ok := p.ordinal.get(); return ok }():
		ordinal, _ := p.ordinal.get()
		d, derr := civilDateFromOrdinal(int(*givenYear), int(ordinal))
		if derr != nil {
			return civilDate{}, errOutOfRange
		}
		date = d
		verified = verifyYMD(d) && verifyISOWeekDate(d) && verifyOrdinal(d)

	case givenYear != nil && func() bool { _, ok1 := p.weekFromSun.get(); _, ok2 := p.weekday.get(); return ok1 && ok2 }():
		week, _ := p.weekFromSun.get()
		wd, _ := p.weekday.get()
		d, derr := resolveWeekDate(int(*givenYear), int(week), wd, time.Sunday)
		if derr != nil {
			return civilDate{}, derr
		}
		date = d
		verified = verifyYMD(d) && verifyISOWeekDate(d) && verifyOrdinal(d)

	case givenYear != nil && func() bool { _, ok1 := p.weekFromMon.get(); _, ok2 := p.weekday.get(); return ok1 && ok2 }():
		week, _ := p.weekFromMon.get()
		wd, _ := p.weekday.get()
		d, derr := resolveWeekDate(int(*givenYear), int(week), wd, time.Monday)
		if derr != nil {
			return civilDate{}, derr
		}
		date = d
		verified = verifyYMD(d) && verifyISOWeekDate(d) && verifyOrdinal(d)

	case givenIsoYear != nil && func() bool { _, ok1 := p.isoWeek.get(); _, ok2 := p.weekday.get(); return ok1 && ok2 }():
		isoWeek, _ := p.isoWeek.get()
		wd, _ := p.weekday.get()
		d, derr := civilDateFromISOYearWeekday(int(*givenIsoYear), int(isoWeek), wd)
		if derr != nil {
			return civilDate{}, errOutOfRange
		}
		date = d
		verified = verifyYMD(d) && verifyOrdinal(d)

	default:
		return civilDate{}, errNotEnough
	}

	if !verified {
		return civilDate{}, errImpossible
	}
	if q, ok := p.quarter.get(); ok && int(q) != date.quarter() {
		return civilDate{}, errImpossible
	}
	return date, nil
}

// resolveYear reconstructs a full year from the full-year, div-100 and
// mod-100 fields, checking they are consistent with each other, mirroring
// chrono's resolve_year.
func resolveYear(y, q, r opt[int32]) (*int32, error) {
	yv, yok := y.get()
	qv, qok := q.get()
	rv, rok := r.get()

	switch {
	case !qok && !rok:
		if !yok {
			return nil, nil
		}
		v := yv
		return &v, nil

	case yok && (!rok || (rv >= 0 && rv <= 99)):
		if yv < 0 {
			return nil, errImpossible
		}
		impliedDiv100 := yv / 100
		impliedMod100 := yv % 100
		wantQ := impliedDiv100
		if qok {
			wantQ = qv
		}
		wantR := impliedMod100
		if rok {
			wantR = rv
		}
		if wantQ == impliedDiv100 && wantR == impliedMod100 {
			v := yv
			return &v, nil
		}
		return nil, errImpossible

	case !yok && qok && rok && rv >= 0 && rv <= 99:
		if qv < 0 {
			return nil, errImpossible
		}
		full := int64(qv)*100 + int64(rv)
		if full < minInt32 || full > maxInt32 {
			return nil, errOutOfRange
		}
		v := int32(full)
		return &v, nil

	case !yok && !qok && rok && rv >= 0 && rv <= 99:
		v := rv
		if v < 70 {
			v += 2000
		} else {
			v += 1900
		}
		return &v, nil

	case !yok && qok && !rok:
		return nil, errNotEnough

	default:
		return nil, errOutOfRange
	}
}

// toNaiveTime resolves the time-of-day fields, mirroring chrono's
// Parsed::to_naive_time. The returned nanosecond may reach 1,999,999,999
// to represent a leap second (second 60).
func (p *parsed) toNaiveTime() (hour, minute, second int, nanosecond int64, err error) {
	div, divOK := p.hourDiv12.get()
	if !divOK {
		return 0, 0, 0, 0, errNotEnough
	}
	if div > 1 {
		return 0, 0, 0, 0, errOutOfRange
	}
	mod, modOK := p.hourMod12.get()
	if !modOK {
		return 0, 0, 0, 0, errNotEnough
	}
	if mod > 11 {
		return 0, 0, 0, 0, errOutOfRange
	}
	hour = int(div*12 + mod)

	m, ok := p.minute.get()
	if !ok {
		return 0, 0, 0, 0, errNotEnough
	}
	if m > 59 {
		return 0, 0, 0, 0, errOutOfRange
	}
	minute = int(m)

	secVal, secSet := p.second.get()
	var nano int64
	switch {
	case !secSet:
		second = 0
	case secVal <= 59:
		second = int(secVal)
	case secVal == 60:
		second = 59
		nano = 1_000_000_000
	default:
		return 0, 0, 0, 0, errOutOfRange
	}

	if ns, ok := p.nanosecond.get(); ok {
		if !secSet {
			return 0, 0, 0, 0, errNotEnough
		}
		if ns > 999_999_999 {
			return 0, 0, 0, 0, errOutOfRange
		}
		nano += int64(ns)
	}

	return hour, minute, second, nano, nil
}

// toNaiveDateTimeWithOffset resolves the date and time fields (assuming
// the given offset in seconds east of UTC for reconciling a timestamp
// field) into a wall-clock time.Time interpreted as UTC, mirroring
// chrono's Parsed::to_naive_datetime_with_offset.
func (p *parsed) toNaiveDateTimeWithOffset(offset int32) (time.Time, error) {
	date, dateErr := p.toNaiveDate()
	hour, minute, second, nano, timeErr := p.toNaiveTime()

	if dateErr == nil && timeErr == nil {
		// base is the naive wall-clock instant with `second` capped at 59
		// (any leap second is carried entirely in `nano`, which may reach
		// 1,999,999,999). chrono's own timestamp() for a leap-second value
		// is computed from the pre-leap whole seconds only, ignoring the
		// leap; base.Unix() matches that so the timestamp consistency
		// check below agrees with chrono. The returned instant, however,
		// rolls the leap second's extra nanoseconds forward into the next
		// second/minute (see the package doc for why).
		base := time.Date(date.year, time.Month(date.month), date.day, hour, minute, second, 0, time.UTC)
		dt := base.Add(time.Duration(nano))
		if ts, ok := p.timestamp.get(); ok {
			gotTS := base.Unix() - int64(offset)
			leapOK := nano >= 1_000_000_000 && ts == gotTS+1
			if ts != gotTS && !leapOK {
				return time.Time{}, errImpossible
			}
		}
		return dt, nil
	}

	ts, hasTS := p.timestamp.get()
	if !hasTS {
		if dateErr != nil {
			return time.Time{}, dateErr
		}
		return time.Time{}, timeErr
	}

	if kindOf(dateErr) == OutOfRange || kindOf(timeErr) == OutOfRange {
		return time.Time{}, errOutOfRange
	}
	if kindOf(dateErr) == Impossible || kindOf(timeErr) == Impossible {
		return time.Time{}, errImpossible
	}

	sum := ts + int64(offset)
	dt := time.Unix(sum, 0).UTC()

	filled := *p
	if secVal, ok := p.second.get(); ok && secVal == 60 {
		switch dt.Second() {
		case 59:
			// nothing to do
		case 0:
			dt = dt.Add(-time.Second)
		default:
			return time.Time{}, errImpossible
		}
	} else if err := filled.setSecond(int64(dt.Second())); err != nil {
		return time.Time{}, err
	}
	if err := filled.setYear(int64(dt.Year())); err != nil {
		return time.Time{}, err
	}
	if err := filled.setOrdinal(int64(dt.YearDay())); err != nil {
		return time.Time{}, err
	}
	if err := filled.setHour(int64(dt.Hour())); err != nil {
		return time.Time{}, err
	}
	if err := filled.setMinute(int64(dt.Minute())); err != nil {
		return time.Time{}, err
	}

	fdate, err := filled.toNaiveDate()
	if err != nil {
		return time.Time{}, err
	}
	fhour, fminute, fsecond, fnano, err := filled.toNaiveTime()
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(fdate.year, time.Month(fdate.month), fdate.day, fhour, fminute, fsecond, 0, time.UTC).
		Add(time.Duration(fnano)), nil
}

func kindOf(err error) ErrorKind {
	if e, ok := err.(*Error); ok {
		return e.kind
	}
	return -1
}

// toDatetime resolves the date, time and offset fields into an absolute
// instant, mirroring chrono's Parsed::to_datetime. The returned time.Time
// is normalized to UTC.
func (p *parsed) toDatetime() (time.Time, error) {
	offVal, offOK := p.offset.get()
	_, tsOK := p.timestamp.get()
	var offset int32
	switch {
	case offOK:
		offset = offVal
	case tsOK:
		offset = 0
	default:
		return time.Time{}, errNotEnough
	}

	naive, err := p.toNaiveDateTimeWithOffset(offset)
	if err != nil {
		return time.Time{}, err
	}
	if offset <= -86400 || offset >= 86400 {
		return time.Time{}, errOutOfRange
	}
	return naive.Add(-time.Duration(offset) * time.Second).UTC(), nil
}
