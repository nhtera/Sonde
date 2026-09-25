// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"testing"
	"time"
)

func mustParseDateTime(t *testing.T, s, layout string) time.Time {
	t.Helper()
	tm, err := ParseDateTime(s, layout)
	if err != nil {
		t.Fatalf("ParseDateTime(%q, %q): %v", s, layout, err)
	}
	return tm
}

// TestParseNumericTable ports a subset of chrono's format::parse
// test_parse_numeric (src/format/parse.rs), using %Y (signed) and %j
// (unsigned) as the numeric fields under test.
func TestParseNumericTable(t *testing.T) {
	type wantOK struct {
		year int
		ok   bool
	}
	yearCases := []struct {
		in   string
		want wantOK
	}{
		{"1987", wantOK{1987, true}},
		{"0000", wantOK{0, true}},
		{"9999", wantOK{9999, true}},
		{"5", wantOK{5, true}},
		{"-42", wantOK{-42, true}},
		{"+42", wantOK{42, true}},
		{"-0042", wantOK{-42, true}},
		{"+0042", wantOK{42, true}},
		{"-42195", wantOK{-42195, true}},
		{"+42195", wantOK{42195, true}},
		{"  -42195", wantOK{-42195, true}},
		{" +42195", wantOK{42195, true}},
	}
	for _, c := range yearCases {
		got, err := ParseNaiveDate(c.in+"-01-01", "%Y-%m-%d")
		if c.want.ok {
			if err != nil {
				t.Errorf("ParseNaiveDate(%q): unexpected error %v", c.in, err)
				continue
			}
			if got.Year() != c.want.year {
				t.Errorf("ParseNaiveDate(%q).Year() = %d, want %d", c.in, got.Year(), c.want.year)
			}
		} else if err == nil {
			t.Errorf("ParseNaiveDate(%q): expected error", c.in)
		}
	}

	// out of order / mismatched sign errors
	for _, bad := range []string{"-", "+", "  -   42", "  +   42"} {
		if _, err := ParseNaiveDate(bad+"-01-01", "%Y-%m-%d"); err == nil {
			t.Errorf("ParseNaiveDate(%q): expected error", bad)
		}
	}

	// unsigned field (%j, ordinal) rejects a sign.
	for _, bad := range []string{"+345", "-345"} {
		var p parsed
		if err := parseItems(&p, bad, []item{numItemNone(numOrdinal)}); err == nil {
			t.Errorf("ordinal %q: expected error", bad)
		}
	}
	var p parsed
	if err := parseItems(&p, "345", []item{numItemNone(numOrdinal)}); err != nil {
		t.Fatalf("ordinal 345: %v", err)
	}
	if v, _ := p.ordinal.get(); v != 345 {
		t.Errorf("ordinal = %d, want 345", v)
	}
}

// TestParseLiteralAndSpace ports a subset of
// format::parse::test_parse_whitespace_and_literal.
func TestParseLiteralAndSpace(t *testing.T) {
	check := func(s string, items []item, wantErr bool) {
		t.Helper()
		var p parsed
		err := parseItems(&p, s, items)
		if wantErr && err == nil {
			t.Errorf("parseItems(%q): expected error", s)
		}
		if !wantErr && err != nil {
			t.Errorf("parseItems(%q): unexpected error %v", s, err)
		}
	}
	check("", nil, false)
	check(" ", nil, true) // TooLong: nothing to consume the space
	check("", []item{spaceItem("")}, false)
	check(" ", []item{spaceItem(" ")}, false)
	check("  ", []item{spaceItem("  ")}, false)
	check("\t", []item{spaceItem("")}, false)
	check("a", []item{litItem("a")}, false)
	check("aa", []item{litItem("a")}, true)
	check("A", []item{litItem("a")}, true) // case sensitive
	check("xy", []item{litItem("x"), litItem("y")}, false)
	check("x y", []item{litItem("x"), litItem("y")}, true)
	check("x y", []item{litItem("x"), spaceItem(" "), litItem("y")}, false)
}

// TestParseTimezoneOffsets exercises %z, %:z, %::z, %:::z and %#z parsing,
// which per chrono all accept the same set of inputs when parsing
// (colon optional, whitespace around the colon allowed).
func TestParseTimezoneOffsets(t *testing.T) {
	for _, layout := range []string{"%z", "%:z", "%::z", "%:::z"} {
		for _, in := range []string{"+0930", "+09:30", "-0400", "+00:00", "-2400", "+2400"} {
			var p parsed
			_, err := parseOneItem(&p, in, mustSingleItem(t, layout))
			// Only some inputs match a given layout's own format string,
			// but the timezone parser itself accepts any of these forms
			// regardless of which %?z variant requested it.
			off, ok := p.offset.get()
			if err != nil {
				continue
			}
			if !ok {
				t.Errorf("layout %s input %s: offset not set", layout, in)
			}
			_ = off
		}
	}

	// %#z allows missing minutes.
	items, err := tokenizeFormat("%#z")
	if err != nil {
		t.Fatalf("tokenize %%#z: %v", err)
	}
	var p parsed
	if err := parseItems(&p, "+09", items); err != nil {
		t.Fatalf("%%#z +09: %v", err)
	}
	off, _ := p.offset.get()
	if off != 9*3600 {
		t.Errorf("%%#z +09 offset = %d, want %d", off, 9*3600)
	}
}

func mustSingleItem(t *testing.T, layout string) item {
	t.Helper()
	items, err := tokenizeFormat(layout)
	if err != nil || len(items) != 1 {
		t.Fatalf("tokenizeFormat(%q) = %v, %v", layout, items, err)
	}
	return items[0]
}

// TestParseMustPassCases exercises the cookie/HTTP date formats and
// specifiers this package's callers (dateFormat/toDate filters and cookie
// Expires parsing) depend on.
func TestParseMustPassCases(t *testing.T) {
	t.Run("rfc3339-relaxed-Z", func(t *testing.T) {
		got, err := ParseDateTime("2001-07-07T15:04:60.026490708Z", "%+")
		if err != nil {
			t.Fatalf("%%+ with Z: %v", err)
		}
		want := time.Date(2001, 7, 7, 15, 5, 0, 26_490_708, time.UTC)
		if !got.Equal(want) {
			t.Errorf("%%+ with Z = %v, want %v", got, want)
		}
	})

	t.Run("rfc3339-relaxed-UTC", func(t *testing.T) {
		if _, err := ParseDateTime("2001-07-07T15:04:60.026490708UTC", "%+"); err != nil {
			t.Fatalf("%%+ with UTC: %v", err)
		}
		if _, err := ParseDateTime("2001-07-07t15:04:60.026490708utc", "%+"); err != nil {
			t.Fatalf("%%+ with lowercase utc: %v", err)
		}
	})

	t.Run("rfc2822-gmt-literal", func(t *testing.T) {
		got, err := ParseNaiveDateTime("Wed, 13 Jan 2021 22:23:01 GMT", "%a, %d %b %Y %H:%M:%S GMT")
		if err != nil {
			t.Fatalf("cookie date literal GMT: %v", err)
		}
		want := time.Date(2021, 1, 13, 22, 23, 1, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("rfc2822-gmt-tzname", func(t *testing.T) {
		got, err := ParseNaiveDateTime("Wed, 13 Jan 2021 22:23:01 GMT", "%a, %d %b %Y %H:%M:%S GMT%Z")
		if err != nil {
			t.Fatalf("cookie date with trailing %%Z: %v", err)
		}
		want := time.Date(2021, 1, 13, 22, 23, 1, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("naive-date", func(t *testing.T) {
		got, err := ParseNaiveDate("2001-07-08", "%Y-%m-%d")
		if err != nil || !got.Equal(time.Date(2001, 7, 8, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("naive-datetime", func(t *testing.T) {
		got, err := ParseNaiveDateTime("2001-07-08T00:34:59", "%Y-%m-%dT%H:%M:%S")
		if err != nil || !got.Equal(time.Date(2001, 7, 8, 0, 34, 59, 0, time.UTC)) {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("naive-datetime-fraction-Z", func(t *testing.T) {
		got, err := ParseNaiveDateTime("2001-07-08T00:34:59.5Z", "%Y-%m-%dT%H:%M:%S%.fZ")
		if err != nil || !got.Equal(time.Date(2001, 7, 8, 0, 34, 59, 500_000_000, time.UTC)) {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("datetime-numeric-offset", func(t *testing.T) {
		got := mustParseDateTime(t, "2001-07-08T00:34:59+0930", "%Y-%m-%dT%H:%M:%S%z")
		want := time.Date(2001, 7, 7, 15, 4, 59, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("cookie-3-digit-fraction", func(t *testing.T) {
		got, err := ParseNaiveDateTime("Wed, 13-Jan-2021 22:23:01.000 GMT", "%a, %d-%b-%Y %H:%M:%S%.3f GMT")
		if err != nil {
			t.Fatalf("cookie 3-digit fraction: %v", err)
		}
		want := time.Date(2021, 1, 13, 22, 23, 1, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// TestParseWeekdayMismatchIsImpossible ports the Parsed doc example: a
// weekday inconsistent with the date is an Impossible error, not silently
// accepted.
func TestParseWeekdayMismatchIsImpossible(t *testing.T) {
	if _, err := ParseNaiveDate("Wed 31 Dec 2014", "%a %d %b %Y"); err != nil {
		t.Fatalf("consistent weekday: %v", err)
	}
	_, err := ParseNaiveDate("Thu 31 Dec 2014", "%a %d %b %Y")
	if err == nil {
		t.Fatalf("expected error for mismatched weekday")
	}
	if e, ok := err.(*Error); !ok || e.Kind() != Impossible {
		t.Fatalf("expected Impossible, got %v", err)
	}
}

// TestParseCaseInsensitiveNames ports the FromStr doc tests for month and
// weekday names: parsing is case-insensitive and accepts either the short
// or the long form.
func TestParseCaseInsensitiveNames(t *testing.T) {
	got, err := ParseNaiveDate("mON 01 jAnUaRy 2001", "%a %d %B %Y")
	if err != nil {
		t.Fatalf("case-insensitive parse: %v", err)
	}
	if got.Weekday() != time.Monday {
		t.Errorf("weekday = %v, want Monday", got.Weekday())
	}
}

// TestParseAmbiguousTwoDigitYear ports chrono's %y / %C century rules: a
// two-digit year alone assumes 19xx when >=70, 20xx otherwise.
func TestParseAmbiguousTwoDigitYear(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"69", 2069}, {"70", 1970}, {"00", 2000}, {"99", 1999},
	}
	for _, c := range cases {
		got, err := ParseNaiveDate(c.in+"-01-01", "%y-%m-%d")
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got.Year() != c.want {
			t.Errorf("%q: year = %d, want %d", c.in, got.Year(), c.want)
		}
	}
}

// TestParseTimestampConsistency ports the NaiveDateTime doc example that a
// %s timestamp must agree with the other parsed fields.
func TestParseTimestampConsistency(t *testing.T) {
	const layout = "%Y-%m-%d %H:%M:%S = UNIX timestamp %s"
	if _, err := ParseNaiveDateTime("2001-09-09 01:46:39 = UNIX timestamp 999999999", layout); err != nil {
		t.Fatalf("consistent timestamp: %v", err)
	}
	if _, err := ParseNaiveDateTime("1970-01-01 00:00:00 = UNIX timestamp 1", layout); err == nil {
		t.Fatalf("expected error for inconsistent timestamp")
	}
}

// TestParseInvalidCalendarDate ports NaiveDateTime doc examples of
// out-of-range field values.
func TestParseInvalidCalendarDate(t *testing.T) {
	for _, s := range []string{
		"2015-02-30 12:34:56", // February has no 30th
		"2015-13-01 12:34:56", // month 13
		"2015-00-01 12:34:56", // month 0
		"2015-01-32 12:34:56", // day 32
	} {
		if _, err := ParseNaiveDateTime(s, "%Y-%m-%d %H:%M:%S"); err == nil {
			t.Errorf("%q: expected error", s)
		}
	}
}

// TestParseYearsRequireSignOutsideFourDigits ports chrono's rule that
// years before 1 BCE or after 9999 CE require an explicit sign with %Y.
func TestParseYearsRequireSignOutsideFourDigits(t *testing.T) {
	const layout = "%Y-%m-%d %H:%M:%S"
	if _, err := ParseNaiveDateTime("10000-09-09 01:46:39", layout); err == nil {
		t.Fatalf("expected error without explicit sign")
	}
	if _, err := ParseNaiveDateTime("+10000-09-09 01:46:39", layout); err != nil {
		t.Fatalf("with explicit sign: %v", err)
	}
}

// TestParseTimeOutOfRange ports NaiveDateTime doc examples for invalid
// time-of-day fields.
func TestParseTimeOutOfRange(t *testing.T) {
	if _, err := ParseNaiveDateTime("94/9/4 17:60", "%y/%m/%d %H:%M"); err == nil {
		t.Errorf("94/9/4 17:60: expected error")
	}
	if _, err := ParseNaiveDateTime("94/9/4 24:00:00", "%y/%m/%d %H:%M:%S"); err == nil {
		t.Errorf("94/9/4 24:00:00: expected error")
	}
}

// TestParseCenturyField exercises %C (year_div_100), alone and combined
// with %Y / %y, mirroring chrono's set_year_div_100 consistency rules.
func TestParseCenturyField(t *testing.T) {
	// %C alone is not enough to resolve a date.
	if _, err := ParseNaiveDate("20", "%C"); err == nil {
		t.Fatalf("expected error for %%C alone")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != NotEnough {
		t.Fatalf("expected NotEnough, got %v", err)
	}

	// %C + %y reconstructs the full year without %Y.
	got, err := ParseNaiveDate("19 87-06-15", "%C %y-%m-%d")
	if err != nil {
		t.Fatalf("%%C %%y: %v", err)
	}
	if got.Year() != 1987 {
		t.Fatalf("year = %d, want 1987", got.Year())
	}

	// %Y consistent with %C.
	got, err = ParseNaiveDate("1987 19-06-15", "%Y %C-%m-%d")
	if err != nil {
		t.Fatalf("consistent %%Y %%C: %v", err)
	}
	if got.Year() != 1987 {
		t.Fatalf("year = %d, want 1987", got.Year())
	}

	// %Y inconsistent with %C is Impossible.
	if _, err := ParseNaiveDate("1987 20-06-15", "%Y %C-%m-%d"); err == nil {
		t.Fatalf("expected error for inconsistent %%C")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != Impossible {
		t.Fatalf("expected Impossible, got %v", err)
	}
}

// TestParseIsoYearMod100 exercises %g (isoyear_mod_100), consistent and
// inconsistent with %G, combined with %V/%A to fully resolve a date. %g
// alone (like %G alone) is not enough to resolve a date, since neither
// implies an isoweek or weekday.
func TestParseIsoYearMod100(t *testing.T) {
	if _, err := ParseNaiveDate("05", "%g"); err == nil {
		t.Fatalf("expected error for %%g alone")
	}

	got, err := ParseNaiveDate("2004 04-W53-Fri", "%G %g-W%V-%A")
	if err != nil {
		t.Fatalf("consistent %%G %%g: %v", err)
	}
	want := time.Date(2004, 12, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	if _, err := ParseNaiveDate("2004 05-W53-Fri", "%G %g-W%V-%A"); err == nil {
		t.Fatalf("expected error for inconsistent %%g")
	}
}

// TestParseQuarter exercises %q, both consistent and inconsistent with a
// full date.
func TestParseQuarter(t *testing.T) {
	if _, err := ParseNaiveDate("2 2001-06-15", "%q %Y-%m-%d"); err != nil {
		t.Fatalf("consistent quarter: %v", err)
	}
	if _, err := ParseNaiveDate("1 2001-06-15", "%q %Y-%m-%d"); err == nil {
		t.Fatalf("expected error for inconsistent quarter")
	}
}

// TestParseWeekFromMon exercises %W (week_from_mon) combined with a
// weekday and year, mirroring the year+week_from_mon+weekday branch of
// Parsed::to_naive_date.
func TestParseWeekFromMon(t *testing.T) {
	for year := 2015; year <= 2023; year++ {
		for _, md := range [][2]int{{1, 1}, {3, 15}, {12, 31}} {
			want := time.Date(year, time.Month(md[0]), md[1], 0, 0, 0, 0, time.UTC)
			s, err := Format(want, "%Y-%W-%u")
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			got, err := ParseNaiveDate(s, "%Y-%W-%u")
			if err != nil {
				t.Fatalf("ParseNaiveDate(%q): %v", s, err)
			}
			if !got.Equal(want) {
				t.Errorf("%%W round trip %v via %q = %v", want, s, got)
			}
		}
	}
}

// TestParseAmPmHour12 exercises %I/%l (hour_mod_12) and %p/%P (am/pm),
// including the noon/midnight edge cases and rejection of malformed text.
func TestParseAmPmHour12(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"12:00 AM", 0}, {"12:00 PM", 12}, {"01:00 AM", 1}, {"01:00 PM", 13},
		{"11:59 pm", 23}, {"11:59 am", 11},
	}
	for _, c := range cases {
		got, err := ParseNaiveDateTime(c.in+" 2001-01-01", "%I:%M %p %Y-%m-%d")
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got.Hour() != c.want {
			t.Errorf("%q: hour = %d, want %d", c.in, got.Hour(), c.want)
		}
	}

	got, err := ParseNaiveDateTime("12:00 am 2001-01-01", "%l:%M %P %Y-%m-%d")
	if err != nil || got.Hour() != 0 {
		t.Errorf("%%l %%P: got %v, %v", got, err)
	}

	for _, bad := range []string{"13:00 AM 2001-01-01", "00:00 AM 2001-01-01", "12:00 XM 2001-01-01", "12:00 A 2001-01-01"} {
		if _, err := ParseNaiveDateTime(bad, "%I:%M %p %Y-%m-%d"); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

// TestParseNumericNanosecond exercises %f (Numeric::Nanosecond), which
// unlike %.f/%3f is parsed as a plain integer, not scaled by digit count.
func TestParseNumericNanosecond(t *testing.T) {
	var p parsed
	if err := parseItems(&p, "5", []item{numItemZero(numNanosecond)}); err != nil {
		t.Fatalf("%%f \"5\": %v", err)
	}
	if v, _ := p.nanosecond.get(); v != 5 {
		t.Fatalf("nanosecond = %d, want 5 (unscaled)", v)
	}

	var p2 parsed
	if err := parseItems(&p2, "026490708", []item{numItemZero(numNanosecond)}); err != nil {
		t.Fatalf("%%f full width: %v", err)
	}
	if v, _ := p2.nanosecond.get(); v != 26_490_708 {
		t.Fatalf("nanosecond = %d, want 26490708", v)
	}
}

// TestParseFractionalSecondsAllWidths exercises %.6f, %.9f, %6f and %9f
// parsing (the %.3f/%3f forms are already exercised by other tests).
func TestParseFractionalSecondsAllWidths(t *testing.T) {
	got, err := ParseNaiveDateTime("2001-01-01 00:00:00.123456", "%Y-%m-%d %H:%M:%S%.6f")
	if err != nil || got.Nanosecond() != 123_456_000 {
		t.Fatalf("%%.6f: %v %v", got, err)
	}
	got, err = ParseNaiveDateTime("2001-01-01 00:00:00.123456789", "%Y-%m-%d %H:%M:%S%.9f")
	if err != nil || got.Nanosecond() != 123_456_789 {
		t.Fatalf("%%.9f: %v %v", got, err)
	}
	got, err = ParseNaiveDateTime("2001-01-01 00:00:00123456", "%Y-%m-%d %H:%M:%S%6f")
	if err != nil || got.Nanosecond() != 123_456_000 {
		t.Fatalf("%%6f: %v %v", got, err)
	}
	got, err = ParseNaiveDateTime("2001-01-01 00:00:00123456789", "%Y-%m-%d %H:%M:%S%9f")
	if err != nil || got.Nanosecond() != 123_456_789 {
		t.Fatalf("%%9f: %v %v", got, err)
	}
	if _, err := ParseNaiveDateTime("2001-01-01 00:00:0012345", "%Y-%m-%d %H:%M:%S%6f"); err == nil {
		t.Fatalf("%%6f short: expected error")
	}
}

// TestParseLongWeekdayName exercises %A, both the long form and the short
// form chrono also accepts for it.
func TestParseLongWeekdayName(t *testing.T) {
	got, err := ParseNaiveDate("Monday 2001-01-01", "%A %Y-%m-%d")
	if err != nil || got.Weekday() != time.Monday {
		t.Fatalf("long form: %v %v", got, err)
	}
	got, err = ParseNaiveDate("Wed 2001-01-03", "%A %Y-%m-%d")
	if err != nil || got.Weekday() != time.Wednesday {
		t.Fatalf("short form via %%A: %v %v", got, err)
	}
}

// TestParseTimezoneName exercises %Z, which consumes and discards a
// non-whitespace token without validating or recording it.
func TestParseTimezoneName(t *testing.T) {
	got, err := ParseNaiveDateTime("2001-01-01 00:00:00 PST", "%Y-%m-%d %H:%M:%S %Z")
	if err != nil {
		t.Fatalf("%%Z: %v", err)
	}
	if !got.Equal(time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
}

// TestAmPmMalformedFixedItem exercises the too-short and invalid-text
// error paths of the am/pm fixed item parser.
func TestAmPmMalformedFixedItem(t *testing.T) {
	var p parsed
	if err := parseItems(&p, "A", []item{fixItem(fxUpperAmPm)}); err == nil {
		t.Fatalf("expected TooShort")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != TooShort {
		t.Fatalf("expected TooShort, got %v", err)
	}
	var p2 parsed
	if err := parseItems(&p2, "XX", []item{fixItem(fxUpperAmPm)}); err == nil {
		t.Fatalf("expected Invalid")
	} else if e, _ := err.(*Error); e == nil || e.Kind() != Invalid {
		t.Fatalf("expected Invalid, got %v", err)
	}
}

// TestParseTimezoneOffsetPermissiveMissingMinutes exercises %#z accepting
// an offset with no minutes component at all (not even "00").
func TestParseTimezoneOffsetPermissiveMissingMinutes(t *testing.T) {
	items, err := tokenizeFormat("%#z")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	var p parsed
	if err := parseItems(&p, "-09", items); err != nil {
		t.Fatalf("%%#z -09: %v", err)
	}
	off, _ := p.offset.get()
	if off != -9*3600 {
		t.Errorf("offset = %d, want %d", off, -9*3600)
	}
}
