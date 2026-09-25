// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"math"
	"testing"
	"time"
)

// TestParsedSetFields ports the shape of chrono's
// format::parsed::test_parsed_set_fields: every set_* method rejects a
// second, different value as Impossible, accepts the same value again,
// and rejects out-of-range values.
func TestParsedSetFields(t *testing.T) {
	type setter struct {
		name     string
		ok       int64
		again    int64
		conflict int64
		bad      []int64
		call     func(*parsed, int64) error
	}
	setters := []setter{
		{"year", 1987, 1987, 1988, []int64{math.MinInt64, math.MaxInt64}, (*parsed).setYear},
		{"yearDiv100", 19, 19, 20, []int64{-1, math.MaxInt64}, (*parsed).setYearDiv100},
		{"yearMod100", 87, 87, 86, []int64{-1, 100}, (*parsed).setYearMod100},
		{"isoYear", 1987, 1987, 1988, []int64{math.MinInt64, math.MaxInt64}, (*parsed).setIsoYear},
		{"isoYearMod100", 87, 87, 86, []int64{-1, 100}, (*parsed).setIsoYearMod100},
		{"quarter", 2, 2, 3, []int64{0, 5}, (*parsed).setQuarter},
		{"month", 6, 6, 7, []int64{0, 13}, (*parsed).setMonth},
		{"weekFromSun", 10, 10, 11, []int64{-1, 54}, (*parsed).setWeekFromSun},
		{"weekFromMon", 10, 10, 11, []int64{-1, 54}, (*parsed).setWeekFromMon},
		{"isoWeek", 10, 10, 11, []int64{0, 54}, (*parsed).setIsoWeek},
		{"ordinal", 100, 100, 101, []int64{0, 367}, (*parsed).setOrdinal},
		{"day", 15, 15, 16, []int64{0, 32}, (*parsed).setDay},
		{"hour12", 5, 5, 6, []int64{0, 13}, (*parsed).setHour12},
		{"hour", 15, 15, 16, []int64{-1, 24}, (*parsed).setHour},
		{"minute", 30, 30, 31, []int64{-1, 60}, (*parsed).setMinute},
		{"second", 30, 30, 31, []int64{-1, 61}, (*parsed).setSecond},
		{"nanosecond", 500, 500, 501, []int64{-1, 1_000_000_000}, (*parsed).setNanosecond},
		{"timestamp", 100, 100, 101, nil, (*parsed).setTimestamp},
		{"offset", 3600, 3600, 7200, []int64{math.MinInt64, math.MaxInt64}, (*parsed).setOffset},
	}
	for _, s := range setters {
		t.Run(s.name, func(t *testing.T) {
			var p parsed
			if err := s.call(&p, s.ok); err != nil {
				t.Fatalf("first set(%d): %v", s.ok, err)
			}
			if err := s.call(&p, s.again); err != nil {
				t.Fatalf("re-set same value(%d): %v", s.again, err)
			}
			if err := s.call(&p, s.conflict); err == nil {
				t.Fatalf("conflicting set(%d): expected error", s.conflict)
			} else if e, ok := err.(*Error); !ok || e.Kind() != Impossible {
				t.Fatalf("conflicting set(%d): expected Impossible, got %v", s.conflict, err)
			}
			for _, bad := range s.bad {
				var p2 parsed
				if err := s.call(&p2, bad); err == nil {
					t.Fatalf("out-of-range set(%d): expected error", bad)
				} else if e, ok := err.(*Error); !ok || e.Kind() != OutOfRange {
					t.Fatalf("out-of-range set(%d): expected OutOfRange, got %v", bad, err)
				}
			}
		})
	}

	// setAmPm and the weekday setters have a different signature.
	var p parsed
	if err := p.setAmPm(false); err != nil {
		t.Fatalf("setAmPm(false): %v", err)
	}
	if err := p.setAmPm(true); err == nil {
		t.Fatalf("conflicting setAmPm: expected error")
	}

	var p2 parsed
	if err := p2.setWeekdayFromNumDaysFromSun(0); err != nil {
		t.Fatalf("setWeekdayFromNumDaysFromSun(0): %v", err)
	}
	if err := p2.setWeekdayFromNumDaysFromSun(1); err == nil {
		t.Fatalf("conflicting weekday: expected error")
	}
	if err := (&parsed{}).setWeekdayFromNumDaysFromSun(7); err == nil {
		t.Fatalf("setWeekdayFromNumDaysFromSun(7): expected OutOfRange")
	}

	var p3 parsed
	if err := p3.setWeekdayFromNumberFromMonday(7); err != nil { // Sunday
		t.Fatalf("setWeekdayFromNumberFromMonday(7): %v", err)
	}
	if v, _ := p3.weekday.get(); v != time.Sunday {
		t.Fatalf("weekday = %v, want Sunday", v)
	}
	if err := (&parsed{}).setWeekdayFromNumberFromMonday(0); err == nil {
		t.Fatalf("setWeekdayFromNumberFromMonday(0): expected OutOfRange")
	}
	if err := (&parsed{}).setWeekdayFromNumberFromMonday(8); err == nil {
		t.Fatalf("setWeekdayFromNumberFromMonday(8): expected OutOfRange")
	}
}

// TestResolveWeekDateImpossible exercises resolve_week_date's rejection of
// a week/weekday combination that would land before the start of the
// year (ordinal <= 0).
func TestResolveWeekDateImpossible(t *testing.T) {
	// 2012-01-01 is a Sunday, so week 0 Sunday has no valid ordinal.
	if got := time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC).Weekday(); got != time.Sunday {
		t.Fatalf("test premise broken: 2012-01-01 is %v, not Sunday", got)
	}
	if _, err := ParseNaiveDate("2012-00-0", "%Y-%U-%w"); err == nil {
		t.Fatalf("expected error for week 0 starting before the year")
	} else if e, ok := err.(*Error); !ok || e.Kind() != Impossible {
		t.Fatalf("expected Impossible, got %v", err)
	}
}

// TestCivilDateFromISOYearWeekdayInvalid exercises the rejection of an ISO
// week that doesn't exist in the given ISO year (e.g. week 53 in a year
// that only has 52).
func TestCivilDateFromISOYearWeekdayInvalid(t *testing.T) {
	// ISO year 2016 has 52 weeks.
	if _, _, err := (func() (int, int, error) {
		d, err := civilDateFromISOYearWeekday(2016, 53, time.Monday)
		if err != nil {
			return 0, 0, err
		}
		y, w := d.isoYearWeek()
		return y, w, nil
	}()); err == nil {
		t.Fatalf("expected error for isoyear 2016 week 53")
	}
	if _, err := ParseNaiveDate("2016-W53-Mon", "%G-W%V-%A"); err == nil {
		t.Fatalf("expected error via ParseNaiveDate for isoyear 2016 week 53")
	}
}

// TestOrdinalOutOfRangeForYear checks day 366 is rejected for a
// non-leap year, exercising civilDateFromOrdinal's rollover detection.
func TestOrdinalOutOfRangeForYear(t *testing.T) {
	if _, err := ParseNaiveDate("2001-366", "%Y-%j"); err == nil {
		t.Fatalf("expected error for day 366 in a non-leap year")
	}
	if _, err := ParseNaiveDate("2000-366", "%Y-%j"); err != nil {
		t.Fatalf("day 366 in a leap year: %v", err)
	}
}

// TestToDatetimeOffsetOutOfRange exercises toDatetime's rejection of an
// offset whose magnitude is 24h or more.
func TestToDatetimeOffsetOutOfRange(t *testing.T) {
	if _, err := ParseDateTime("2001-01-01T00:00:00+9959", "%Y-%m-%dT%H:%M:%S%z"); err == nil {
		t.Fatalf("expected error for +9959 offset")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != OutOfRange {
		t.Fatalf("expected OutOfRange, got %v", err)
	}
}

// TestTimestampOnlyReconstruction exercises the fallback branch of
// to_naive_datetime_with_offset that reconstructs date/time fields purely
// from a %s timestamp.
func TestTimestampOnlyReconstruction(t *testing.T) {
	got, err := ParseNaiveDateTime("994552499", "%s")
	if err != nil {
		t.Fatalf("%%s alone: %v", err)
	}
	want := time.Date(2001, 7, 8, 0, 34, 59, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestTimestampLeapSecondReconciliation exercises the three branches of
// the timestamp-reconstruction leap-second special case: the timestamp
// landing exactly on the pre-leap second (59, no adjustment needed), on
// the following whole second (0, adjusted back), and anywhere else
// (rejected as Impossible).
func TestTimestampLeapSecondReconciliation(t *testing.T) {
	const layout = "%Y-%m-%d %H:%M:%S %s"

	got, err := ParseNaiveDateTime("2021-01-13 23:59:60 1610582399", layout)
	if err != nil {
		t.Fatalf("second=59 case: %v", err)
	}
	want := time.Date(2021, 1, 14, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("second=59 case: got %v, want %v", got, want)
	}

	got, err = ParseNaiveDateTime("2021-01-13 23:59:60 1610582400", layout)
	if err != nil {
		t.Fatalf("second=0 case: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("second=0 case: got %v, want %v", got, want)
	}

	if _, err := ParseNaiveDateTime("2021-01-13 23:59:60 1610582370", layout); err == nil {
		t.Fatalf("expected Impossible for an unrelated timestamp")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != Impossible {
		t.Fatalf("expected Impossible, got %v", err)
	}
}

// TestNaiveDateTimeMissingFields exercises the two "no timestamp to fall
// back on" error-propagation paths of to_naive_datetime_with_offset.
func TestNaiveDateTimeMissingFields(t *testing.T) {
	if _, err := ParseNaiveDateTime("", ""); err == nil {
		t.Fatalf("expected error for empty input/layout")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != NotEnough {
		t.Fatalf("expected NotEnough (date), got %v", err)
	}

	if _, err := ParseNaiveDateTime("2001-01-01", "%Y-%m-%d"); err == nil {
		t.Fatalf("expected error for a date with no time fields")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != NotEnough {
		t.Fatalf("expected NotEnough (time), got %v", err)
	}
}
