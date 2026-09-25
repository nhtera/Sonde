// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"strings"
	"testing"
	"time"
)

// TestFormatStrftimeDocs ports chrono's format::strftime test_strftime_docs
// table (src/format/strftime.rs), for a UTC instant so that offset-bearing
// specifiers use +00:00 (chrono's example used a +09:30 offset and a
// leap-second nanosecond; here that becomes 2001-07-08 00:34:59.026490708
// UTC, since Format always treats its input as a UTC instant and Go's
// time.Time cannot hold a leap second to begin with).
func TestFormatStrftimeDocs(t *testing.T) {
	dt := time.Date(2001, 7, 8, 0, 34, 59, 26_490_708, time.UTC)

	cases := []struct{ layout, want string }{
		// date specifiers
		{"%Y", "2001"}, {"%C", "20"}, {"%y", "01"}, {"%q", "3"},
		{"%m", "07"}, {"%b", "Jul"}, {"%B", "July"}, {"%h", "Jul"},
		{"%d", "08"}, {"%e", " 8"}, {"%a", "Sun"}, {"%A", "Sunday"},
		{"%w", "0"}, {"%u", "7"}, {"%U", "27"}, {"%W", "27"},
		{"%G", "2001"}, {"%g", "01"}, {"%V", "27"}, {"%j", "189"},
		{"%D", "07/08/01"}, {"%x", "07/08/01"}, {"%F", "2001-07-08"},
		{"%v", " 8-Jul-2001"},
		// time specifiers
		{"%H", "00"}, {"%k", " 0"}, {"%I", "12"}, {"%l", "12"},
		{"%P", "am"}, {"%p", "AM"}, {"%M", "34"}, {"%S", "59"},
		{"%f", "026490708"}, {"%.f", ".026490708"},
		{"%.3f", ".026"}, {"%.6f", ".026490"}, {"%.9f", ".026490708"},
		{"%3f", "026"}, {"%6f", "026490"}, {"%9f", "026490708"},
		{"%R", "00:34"}, {"%T", "00:34:59"}, {"%X", "00:34:59"},
		{"%r", "12:34:59 AM"},
		// timezone specifiers (Format is always UTC)
		{"%z", "+0000"}, {"%:z", "+00:00"}, {"%::z", "+00:00:00"}, {"%:::z", "+00"},
		{"%Z", "UTC"},
		// date & time specifiers
		{"%c", "Sun Jul  8 00:34:59 2001"},
		{"%+", "2001-07-08T00:34:59.026490708+00:00"},
		{"%s", "994552499"},
		// special specifiers
		{"%t", "\t"}, {"%n", "\n"}, {"%%", "%"},
	}
	for _, c := range cases {
		got, err := Format(dt, c.layout)
		if err != nil {
			t.Errorf("Format(%q) error: %v", c.layout, err)
			continue
		}
		if got != c.want {
			t.Errorf("Format(%q) = %q, want %q", c.layout, got, c.want)
		}
	}

	// exact fixed fraction with a rounder nanosecond, from the same test.
	rounder := time.Date(2001, 7, 8, 0, 34, 59, 26_490_000, time.UTC)
	got, err := Format(rounder, "%.f")
	if err != nil || got != ".026490" {
		t.Errorf("Format(%%.f) with 26490000ns = %q, %v, want .026490", got, err)
	}

	// complex format specifiers
	got, err = Format(dt, "  %Y%d%m%%%%%t%H%M%S\t")
	if err != nil || got != "  20010807%%\t003459\t" {
		t.Errorf("complex format = %q, %v", got, err)
	}
	got, err = Format(dt, "  %Y%d%m%%%%%t%H:%P:%M%S%:::z\t")
	if err != nil || got != "  20010807%%\t00:am:3459+00\t" {
		t.Errorf("complex format 2 = %q, %v", got, err)
	}
}

// TestFormatInvalidSpecifier ports the "%👻" invalid specifier case.
func TestFormatInvalidSpecifier(t *testing.T) {
	dt := time.Date(2001, 7, 8, 0, 0, 0, 0, time.UTC)
	if _, err := Format(dt, "%👻"); err == nil {
		t.Fatalf("expected error for %%👻")
	} else if e, ok := err.(*Error); !ok || e.Kind() != BadFormat {
		t.Fatalf("expected BadFormat, got %v", err)
	}
}

// TestFormatDateTable ports chrono's format::formatting::test_date_format.
func TestFormatDateTable(t *testing.T) {
	d := time.Date(2012, 3, 4, 0, 0, 0, 0, time.UTC)
	check := func(layout, want string) {
		t.Helper()
		got, err := Format(d, layout)
		if err != nil {
			t.Errorf("Format(%q) error: %v", layout, err)
			return
		}
		if got != want {
			t.Errorf("Format(%q) = %q, want %q", layout, got, want)
		}
	}
	check("%Y,%C,%y,%G,%g", "2012,20,12,2012,12")
	check("%m,%b,%h,%B", "03,Mar,Mar,March")
	check("%q", "1")
	check("%d,%e", "04, 4")
	check("%U,%W,%V", "10,09,09")
	check("%a,%A,%w,%u", "Sun,Sunday,0,7")
	check("%j", "064")
	check("%D,%x", "03/04/12,03/04/12")
	check("%F", "2012-03-04")
	check("%v", " 4-Mar-2012")
	check("%t%n%%%n%t", "\t\n%\n\t")

	nonFourDigit := []struct {
		year int
		want string
	}{
		{12345, "+12345"}, {1234, "1234"}, {123, "0123"}, {12, "0012"}, {1, "0001"},
		{0, "0000"}, {-1, "-0001"}, {-12, "-0012"}, {-123, "-0123"}, {-1234, "-1234"},
		{-12345, "-12345"},
	}
	for _, c := range nonFourDigit {
		got, err := Format(time.Date(c.year, 1, 1, 0, 0, 0, 0, time.UTC), "%Y")
		if err != nil || got != c.want {
			t.Errorf("year %d: Format(%%Y) = %q, %v, want %q", c.year, got, err, c.want)
		}
	}

	check2 := func(year, month, day int, layout, want string) {
		t.Helper()
		got, err := Format(time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), layout)
		if err != nil || got != want {
			t.Errorf("%d-%d-%d Format(%q) = %q, %v, want %q", year, month, day, layout, got, err, want)
		}
	}
	check2(2007, 12, 31, "%G,%g,%U,%W,%V", "2008,08,52,53,01")
	check2(2010, 1, 3, "%G,%g,%U,%W,%V", "2009,09,01,00,53")
}

// TestFormatTimeTable ports chrono's format::formatting::test_time_format.
func TestFormatTimeTable(t *testing.T) {
	base := time.Date(2000, 1, 1, 3, 5, 7, 0, time.UTC)
	check := func(tm time.Time, layout, want string) {
		t.Helper()
		got, err := Format(tm, layout)
		if err != nil || got != want {
			t.Errorf("Format(%q) = %q, %v, want %q", layout, got, err, want)
		}
	}
	check(base.Add(98_765_432), "%H,%k,%I,%l,%P,%p", "03, 3,03, 3,am,AM")
	check(base, "%M", "05")
	check(base.Add(98_765_432), "%S,%f,%.f", "07,098765432,.098765432")
	check(base.Add(98_765_432), "%.3f,%.6f,%.9f", ".098,.098765,.098765432")
	check(base, "%R", "03:05")
	check(base, "%T,%X", "03:05:07,03:05:07")
	check(base, "%r", "03:05:07 AM")
	check(base, "%t%n%%%n%t", "\t\n%\n\t")

	t2 := time.Date(2000, 1, 1, 3, 5, 7, 432_100_000, time.UTC)
	check(t2, "%S,%f,%.f", "07,432100000,.432100")
	check(t2, "%.3f,%.6f,%.9f", ".432,.432100,.432100000")

	t3 := time.Date(2000, 1, 1, 3, 5, 7, 210_000_000, time.UTC)
	check(t3, "%S,%f,%.f", "07,210000000,.210")
	check(t3, "%.3f,%.6f,%.9f", ".210,.210000,.210000000")

	check(base, "%S,%f,%.f", "07,000000000,")
	check(base, "%.3f,%.6f,%.9f", ".000,.000000,.000000000")

	check(time.Date(2000, 1, 1, 13, 57, 9, 0, time.UTC), "%r", "01:57:09 PM")
}

// TestFormatDateTimeTable ports chrono's format::formatting::test_datetime_format.
func TestFormatDateTimeTable(t *testing.T) {
	dt := time.Date(2010, 9, 8, 7, 6, 54, 321_000_000, time.UTC)
	check := func(tm time.Time, layout, want string) {
		t.Helper()
		got, err := Format(tm, layout)
		if err != nil || got != want {
			t.Errorf("Format(%q) = %q, %v, want %q", layout, got, err, want)
		}
	}
	check(dt, "%c", "Wed Sep  8 07:06:54 2010")
	check(dt, "%s", "1283929614")
	check(dt, "%t%n%%%n%t", "\t\n%\n\t")

	dt2 := time.Date(2012, 6, 30, 23, 59, 59, 0, time.UTC)
	check(dt2, "%c", "Sat Jun 30 23:59:59 2012")
	check(dt2, "%s", "1341100799")
}

// TestFormatPaddingModifiers exercises the %-?, %_?, %0? padding overrides.
func TestFormatPaddingModifiers(t *testing.T) {
	d := time.Date(2001, 1, 9, 0, 0, 0, 0, time.UTC)
	check := func(layout, want string) {
		t.Helper()
		got, err := Format(d, layout)
		if err != nil || got != want {
			t.Errorf("Format(%q) = %q, %v, want %q", layout, got, err, want)
		}
	}
	check("%j", "009")
	check("%-j", "9")
	check("%0j", "009")
	check("%_j", "  9")
	check("%e", " 9")
	check("%-e", "9")
	check("%0e", "09")
	check("%_e", " 9")
}

// TestFormatPaddingOnCompositeIsBadFormat ports the tokenizer rule that a
// padding modifier is invalid on a specifier that expands to more than one
// item.
func TestFormatPaddingOnCompositeIsBadFormat(t *testing.T) {
	d := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, layout := range []string{"%-Z", "%0Z", "%_Z", "%.Z", "%:Z", "%.j", "%:j", "%.e", "%:e", "%#m"} {
		if _, err := Format(d, layout); err == nil {
			t.Errorf("Format(%q): expected error", layout)
		}
	}
}

// TestOffsetFormatTable ports chrono's format::formatting::
// test_offset_formatting, exercising formatOffset directly since Format's
// public API always formats a zero (UTC) offset.
func TestOffsetFormatTable(t *testing.T) {
	// +03:45, -03:30, +11:00, -11:00:22, +02:34:26, -12:34:30, +00:00
	offsets := []int32{13_500, -12_600, 39_600, -39_622, 9266, -45270, 0}

	render := func(f offsetFormat, off int32) string {
		var b strings.Builder
		formatOffset(&b, f, off)
		return b.String()
	}

	checkAll := func(precision offsetPrecision, expected [12][7]string) {
		check := func(colon bool, padding pad, allowZulu bool, want [7]string) {
			t.Helper()
			f := offsetFormat{precision: precision, colon: colon, allowZulu: allowZulu, padding: padding}
			for i, off := range offsets {
				got := render(f, off)
				if got != want[i] {
					t.Errorf("precision=%v colon=%v pad=%v zulu=%v off=%d: got %q, want %q",
						precision, colon, padding, allowZulu, off, got, want[i])
				}
			}
		}
		check(true, padZero, false, expected[0])
		check(true, padZero, true, expected[1])
		check(true, padSpace, false, expected[2])
		check(true, padSpace, true, expected[3])
		check(true, padNone, false, expected[4])
		check(true, padNone, true, expected[5])
		check(false, padZero, false, expected[6])
		check(false, padZero, true, expected[7])
		check(false, padSpace, false, expected[8])
		check(false, padSpace, true, expected[9])
		check(false, padNone, false, expected[10])
		check(false, padNone, true, expected[11])
	}

	checkAll(offPrecHours, [12][7]string{
		{"+03", "-03", "+11", "-11", "+02", "-12", "+00"},
		{"+03", "-03", "+11", "-11", "+02", "-12", "Z"},
		{" +3", " -3", "+11", "-11", " +2", "-12", " +0"},
		{" +3", " -3", "+11", "-11", " +2", "-12", "Z"},
		{"+3", "-3", "+11", "-11", "+2", "-12", "+0"},
		{"+3", "-3", "+11", "-11", "+2", "-12", "Z"},
		{"+03", "-03", "+11", "-11", "+02", "-12", "+00"},
		{"+03", "-03", "+11", "-11", "+02", "-12", "Z"},
		{" +3", " -3", "+11", "-11", " +2", "-12", " +0"},
		{" +3", " -3", "+11", "-11", " +2", "-12", "Z"},
		{"+3", "-3", "+11", "-11", "+2", "-12", "+0"},
		{"+3", "-3", "+11", "-11", "+2", "-12", "Z"},
	})
	checkAll(offPrecMinutes, [12][7]string{
		{"+03:45", "-03:30", "+11:00", "-11:00", "+02:34", "-12:35", "+00:00"},
		{"+03:45", "-03:30", "+11:00", "-11:00", "+02:34", "-12:35", "Z"},
		{" +3:45", " -3:30", "+11:00", "-11:00", " +2:34", "-12:35", " +0:00"},
		{" +3:45", " -3:30", "+11:00", "-11:00", " +2:34", "-12:35", "Z"},
		{"+3:45", "-3:30", "+11:00", "-11:00", "+2:34", "-12:35", "+0:00"},
		{"+3:45", "-3:30", "+11:00", "-11:00", "+2:34", "-12:35", "Z"},
		{"+0345", "-0330", "+1100", "-1100", "+0234", "-1235", "+0000"},
		{"+0345", "-0330", "+1100", "-1100", "+0234", "-1235", "Z"},
		{" +345", " -330", "+1100", "-1100", " +234", "-1235", " +000"},
		{" +345", " -330", "+1100", "-1100", " +234", "-1235", "Z"},
		{"+345", "-330", "+1100", "-1100", "+234", "-1235", "+000"},
		{"+345", "-330", "+1100", "-1100", "+234", "-1235", "Z"},
	})
	checkAll(offPrecSeconds, [12][7]string{
		{"+03:45:00", "-03:30:00", "+11:00:00", "-11:00:22", "+02:34:26", "-12:34:30", "+00:00:00"},
		{"+03:45:00", "-03:30:00", "+11:00:00", "-11:00:22", "+02:34:26", "-12:34:30", "Z"},
		{" +3:45:00", " -3:30:00", "+11:00:00", "-11:00:22", " +2:34:26", "-12:34:30", " +0:00:00"},
		{" +3:45:00", " -3:30:00", "+11:00:00", "-11:00:22", " +2:34:26", "-12:34:30", "Z"},
		{"+3:45:00", "-3:30:00", "+11:00:00", "-11:00:22", "+2:34:26", "-12:34:30", "+0:00:00"},
		{"+3:45:00", "-3:30:00", "+11:00:00", "-11:00:22", "+2:34:26", "-12:34:30", "Z"},
		{"+034500", "-033000", "+110000", "-110022", "+023426", "-123430", "+000000"},
		{"+034500", "-033000", "+110000", "-110022", "+023426", "-123430", "Z"},
		{" +34500", " -33000", "+110000", "-110022", " +23426", "-123430", " +00000"},
		{" +34500", " -33000", "+110000", "-110022", " +23426", "-123430", "Z"},
		{"+34500", "-33000", "+110000", "-110022", "+23426", "-123430", "+00000"},
		{"+34500", "-33000", "+110000", "-110022", "+23426", "-123430", "Z"},
	})
}

// TestWriteHundredsOutOfRange exercises writeHundreds' own range guard.
func TestWriteHundredsOutOfRange(t *testing.T) {
	var b strings.Builder
	if err := writeHundreds(&b, 100); err == nil {
		t.Fatalf("expected error for writeHundreds(100)")
	}
}

// TestFormatFixedRFC3339OutOfRangeYear checks %+ formats years outside
// 0..9999 with an explicit sign, mirroring chrono's write_rfc3339.
func TestFormatFixedRFC3339OutOfRangeYear(t *testing.T) {
	got, err := Format(time.Date(12345, 6, 15, 1, 2, 3, 0, time.UTC), "%+")
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if !strings.HasPrefix(got, "+12345-06-15T01:02:03") {
		t.Fatalf("got %q", got)
	}

	got, err = Format(time.Date(-1, 6, 15, 1, 2, 3, 0, time.UTC), "%+")
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if !strings.HasPrefix(got, "-0001-06-15T01:02:03") {
		t.Fatalf("got %q", got)
	}
}

// TestFormatNegativeYearCentury exercises %C/%y (floorDiv/floorMod) for a
// negative year, where chrono's div_euclid/rem_euclid differ from plain
// truncating division.
func TestFormatNegativeYearCentury(t *testing.T) {
	got, err := Format(time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC), "%C,%y")
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	// year -1 floor-divided by 100 is -1 (%y = 99, matching chrono's
	// documented floor-division rule), but chrono's own %C formatter casts
	// that quotient to an unsigned byte (`div_euclid(100) as u8`), which
	// wraps -1 to 255 and renders as "I5". This is a faithfully preserved
	// chrono quirk, not a bug in this port: floorDiv(-1, 100) == -1, but
	// uint8(-1) wraps the same way Rust's `as u8` does.
	if got != "I5,99" {
		t.Fatalf("got %q, want I5,99 (chrono's %%C u8-wrap quirk for negative centuries)", got)
	}
}

// TestFormatOutOfRangeYearPaddingVariants exercises writeN's always-sign
// branches for padSpace and padNone, not just the default padZero used by
// plain %Y.
func TestFormatOutOfRangeYearPaddingVariants(t *testing.T) {
	d := time.Date(12345, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := Format(d, "%-Y")
	if err != nil || got != "+12345" {
		t.Errorf("%%-Y = %q, %v, want +12345", got, err)
	}
	got, err = Format(d, "%_Y")
	if err != nil || got != "+12345" {
		t.Errorf("%%_Y = %q, %v, want +12345", got, err)
	}
}

// A padded timestamp is at least nine characters wide.
func TestFormatPaddedTimestamp(t *testing.T) {
	ts := time.Unix(5, 0).UTC()
	for layout, want := range map[string]string{"%s": "5", "%0s": "000000005", "%_s": "        5", "%-s": "5"} {
		got, err := Format(ts, layout)
		if err != nil || got != want {
			t.Errorf("Format(%q) = %q, %v; want %q", layout, got, err, want)
		}
	}
}
