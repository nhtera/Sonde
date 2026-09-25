// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"testing"
	"time"
)

// TestParseRFC2822Table ports representative cases chrono's RFC 2822
// support documents and tests: a standard example, obsolete 2-digit and
// 3-digit years, military zone letters (treated as -0000, i.e. UTC for our
// purposes since FixedOffset(0) == UTC), and a trailing comment.
func TestParseRFC2822Table(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"Tue, 1 Jul 2003 10:52:37 +0200", time.Date(2003, 7, 1, 8, 52, 37, 0, time.UTC)},
		{"Wed, 18 Feb 2015 23:16:09 GMT", time.Date(2015, 2, 18, 23, 16, 9, 0, time.UTC)},
		{"Wed, 13 Jan 2021 22:23:01 GMT", time.Date(2021, 1, 13, 22, 23, 1, 0, time.UTC)},
		// obsolete 2-digit year: <50 => 20xx
		{"Tue, 1 Jul 03 10:52:37 GMT", time.Date(2003, 7, 1, 10, 52, 37, 0, time.UTC)},
		// obsolete 2-digit year: >=50 => 19xx
		{"Sun, 1 Jul 79 10:52:37 GMT", time.Date(1979, 7, 1, 10, 52, 37, 0, time.UTC)},
		// obsolete 3-digit year => +1900
		{"Sun, 1 Jul 112 10:52:37 GMT", time.Date(2012, 7, 1, 10, 52, 37, 0, time.UTC)},
		// no seconds
		{"Tue, 1 Jul 2003 10:52 +0000", time.Date(2003, 7, 1, 10, 52, 0, 0, time.UTC)},
		// trailing comment
		{"Tue, 1 Jul 2003 10:52:37 +0200 (CEST)", time.Date(2003, 7, 1, 8, 52, 37, 0, time.UTC)},
		// no day-of-week
		{"1 Jul 2003 10:52:37 +0200", time.Date(2003, 7, 1, 8, 52, 37, 0, time.UTC)},
	}
	for _, c := range cases {
		got, err := ParseRFC2822(c.in)
		if err != nil {
			t.Errorf("ParseRFC2822(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseRFC2822(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestParseRFC2822MilitaryZone ports RFC 2822's rule that single-letter
// "military" zones (other than Z) are ambiguous and treated as -0000
// (which this package, having no distinct "unknown offset" concept,
// resolves the same as UTC).
func TestParseRFC2822MilitaryZone(t *testing.T) {
	got, err := ParseRFC2822("Tue, 1 Jul 2003 10:52:37 A")
	if err != nil {
		t.Fatalf("military zone A: %v", err)
	}
	want := time.Date(2003, 7, 1, 10, 52, 37, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestParseRFC2822WeekdayMismatch ports the Parsed doc example showing a
// mismatched day-of-week is rejected, using the RFC2822 parser directly.
func TestParseRFC2822WeekdayMismatch(t *testing.T) {
	if _, err := ParseRFC2822("Wed, 31 Dec 2014 04:26:40 +0000"); err != nil {
		t.Fatalf("consistent weekday: %v", err)
	}
	if _, err := ParseRFC2822("Thu, 31 Dec 2014 04:26:40 +0000"); err == nil {
		t.Fatalf("expected error for mismatched weekday")
	}
}

// TestParseRFC2822Invalid checks malformed input is rejected rather than
// silently accepted.
func TestParseRFC2822Invalid(t *testing.T) {
	for _, in := range []string{
		"",
		"not a date",
		"Tue, 1 Jul 2003",             // missing time
		"Tue, 1 Jul 2003 10:52:37",    // missing zone
		"Tue,1 Jul 2003 10:52:37 GMT", // missing space is fine (S* not 1*S) -- actually valid; remove
	} {
		if in == "Tue,1 Jul 2003 10:52:37 GMT" {
			continue // this is actually valid RFC 2822 (S* not 1*S after the comma)
		}
		if _, err := ParseRFC2822(in); err == nil {
			t.Errorf("ParseRFC2822(%q): expected error", in)
		}
	}
}

// TestParseRFC3339Table ports representative RFC 3339 cases: the
// canonical example, Z, fractional seconds of various widths, a leap
// second, and out-of-range rejections.
func TestParseRFC3339Table(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"1996-12-19T16:39:57-08:00", time.Date(1996, 12, 20, 0, 39, 57, 0, time.UTC)},
		{"1996-12-19T16:39:57Z", time.Date(1996, 12, 19, 16, 39, 57, 0, time.UTC)},
		{"1996-12-19t16:39:57z", time.Date(1996, 12, 19, 16, 39, 57, 0, time.UTC)},
		{"1996-12-19 16:39:57Z", time.Date(1996, 12, 19, 16, 39, 57, 0, time.UTC)},
		{"1996-12-19T16:39:57.123Z", time.Date(1996, 12, 19, 16, 39, 57, 123_000_000, time.UTC)},
		{"1996-12-19T16:39:57.123456789Z", time.Date(1996, 12, 19, 16, 39, 57, 123_456_789, time.UTC)},
		{"1996-12-19T16:39:57.1234567891234Z", time.Date(1996, 12, 19, 16, 39, 57, 123_456_789, time.UTC)},
	}
	for _, c := range cases {
		got, err := ParseRFC3339(c.in)
		if err != nil {
			t.Errorf("ParseRFC3339(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseRFC3339(%q) = %v, want %v", c.in, got, c.want)
		}
	}

	for _, bad := range []string{
		"",
		"1996-12-19T16:39:57",  // missing offset
		"1996-12-19",           // date only
		"1996-13-19T16:39:57Z", // month 13
		"1996-12-19T16:39:57+25:00",
		"1996-12-19X16:39:57Z", // bad separator
	} {
		if _, err := ParseRFC3339(bad); err == nil {
			t.Errorf("ParseRFC3339(%q): expected error", bad)
		}
	}
}

// TestParseRFC3339LeapSecond checks the leap-second-tolerant parse rule
// (RFC 3339 §5.6 allows a value of 60) resolves to the following instant,
// per this package's documented deviation from chrono for leap seconds.
func TestParseRFC3339LeapSecond(t *testing.T) {
	got, err := ParseRFC3339("1990-12-31T23:59:60Z")
	if err != nil {
		t.Fatalf("leap second: %v", err)
	}
	want := time.Date(1991, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestISOWeekDateRoundTrip checks %G/%V/%A parses back to the same date
// %Y-%m-%d would, across a range of ISO week edge cases (years whose
// January 1st or December 31st falls in a different ISO week-year).
func TestISOWeekDateRoundTrip(t *testing.T) {
	for year := 1995; year <= 2035; year++ {
		for _, md := range [][2]int{{1, 1}, {1, 4}, {6, 15}, {12, 28}, {12, 31}} {
			want := time.Date(year, time.Month(md[0]), md[1], 0, 0, 0, 0, time.UTC)
			s, err := Format(want, "%G-W%V-%u")
			if err != nil {
				t.Fatalf("Format ISO week for %v: %v", want, err)
			}
			got, err := ParseNaiveDate(s, "%G-W%V-%u")
			if err != nil {
				t.Fatalf("ParseNaiveDate(%q): %v", s, err)
			}
			if !got.Equal(want) {
				t.Errorf("round trip %v via %q = %v", want, s, got)
			}
		}
	}
}

// TestWeekFromSunMonRoundTrip checks %Y/%U/%w and %Y/%W/%u round trip for
// a range of dates including week-0 edge cases.
func TestWeekFromSunMonRoundTrip(t *testing.T) {
	for year := 2000; year <= 2024; year++ {
		for _, md := range [][2]int{{1, 1}, {1, 2}, {1, 7}, {12, 31}} {
			want := time.Date(year, time.Month(md[0]), md[1], 0, 0, 0, 0, time.UTC)
			s, err := Format(want, "%Y-%U-%w")
			if err != nil {
				t.Fatalf("Format %%U for %v: %v", want, err)
			}
			got, err := ParseNaiveDate(s, "%Y-%U-%w")
			if err != nil {
				t.Fatalf("ParseNaiveDate(%q): %v", s, err)
			}
			if !got.Equal(want) {
				t.Errorf("%%U round trip %v via %q = %v", want, s, got)
			}
		}
	}
}

// TestOrdinalRoundTrip checks %Y-%j round trips, including leap years.
func TestOrdinalRoundTrip(t *testing.T) {
	for _, year := range []int{1999, 2000, 2001, 2004, 2100, 2400} {
		for _, ordinal := range []int{1, 59, 60, 365, 366} {
			d, err := civilDateFromOrdinal(year, ordinal)
			if err != nil {
				continue // e.g. day 366 in a non-leap year
			}
			want := d.t()
			s, ferr := Format(want, "%Y-%j")
			if ferr != nil {
				t.Fatalf("Format: %v", ferr)
			}
			got, perr := ParseNaiveDate(s, "%Y-%j")
			if perr != nil {
				t.Fatalf("ParseNaiveDate(%q): %v", s, perr)
			}
			if !got.Equal(want) {
				t.Errorf("ordinal round trip %v via %q = %v", want, s, got)
			}
		}
	}
}

// TestParseRFC3339StrictErrors exercises a handful of parseRFC3339Strict's
// digit-by-digit validation error paths directly.
func TestParseRFC3339StrictErrors(t *testing.T) {
	for _, bad := range []string{
		"199a-12-19T16:39:57Z",  // non-digit year
		"1996x12-19T16:39:57Z",  // bad separator after year
		"1996-1a-19T16:39:57Z",  // non-digit month
		"1996-12x19T16:39:57Z",  // bad separator after month
		"1996-12-1aT16:39:57Z",  // non-digit day
		"1996-12-19T1a:39:57Z",  // non-digit hour
		"1996-12-19T16x39:57Z",  // bad separator after hour
		"1996-12-19T16:3a:57Z",  // non-digit minute
		"1996-12-19T16:39x57Z",  // bad separator after minute
		"1996-12-19T16:39:5aZ",  // non-digit second
		"1996-12-19T16:39:97Z",  // second out of range
		"1996-12-19T16:39:57Z ", // trailing input
	} {
		if _, err := ParseRFC3339(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

// TestParseRFC2822CommentAndErrors exercises comment_2822 (a malformed
// comment) and a couple of structural RFC 2822 error paths.
func TestParseRFC2822CommentAndErrors(t *testing.T) {
	if _, err := ParseRFC2822("Tue, 1 Jul 2003 10:52:37 +0200 (unterminated"); err == nil {
		t.Errorf("unterminated comment: expected error")
	}
	if _, err := ParseRFC2822("Tue 1 Jul 2003 10:52:37 +0200"); err == nil {
		t.Errorf("missing comma after weekday: expected error")
	}
}

// TestParseRFC2822NoSeconds exercises the RFC 2822 time-of-day form that
// omits seconds.
func TestParseRFC2822NoSeconds(t *testing.T) {
	got, err := ParseRFC2822("Tue, 1 Jul 2003 10:52 +0000")
	if err != nil {
		t.Fatalf("no seconds: %v", err)
	}
	if got.Second() != 0 {
		t.Fatalf("second = %d, want 0", got.Second())
	}
}
