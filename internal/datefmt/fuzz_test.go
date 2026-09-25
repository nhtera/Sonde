// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

import (
	"testing"
	"time"
)

// FuzzFormat checks Format never panics for any layout and any instant
// representable by four int64/int32 seeds.
func FuzzFormat(f *testing.F) {
	seeds := []string{
		"", "%Y-%m-%d", "%+", "%s", "%c", "%r", "%v", "%D", "%x", "%X",
		"%a, %d %b %Y %H:%M:%S %z", "%-j%_e%0H", "%#z", "%.3f", "%3f",
		"%👻", "%", "%%", "%:::z", "100%%", "\t \n literal",
	}
	for _, s := range seeds {
		f.Add(s, int64(0), int32(0))
		f.Add(s, int64(1_700_000_000), int32(123_456_789))
	}
	f.Fuzz(func(t *testing.T, layout string, unixSec int64, nanos int32) {
		n := int64(nanos)
		if n < 0 {
			n = -n
		}
		n %= 1_000_000_000
		tm := time.Unix(unixSec, n).UTC()

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Format(%q) panicked: %v", layout, r)
			}
		}()
		_, _ = Format(tm, layout)
	})
}

// FuzzParse checks the four string-input parse entry points never panic
// for any (input, layout) pair, or any RFC 3339 / RFC 2822 input.
func FuzzParse(f *testing.F) {
	layoutSeeds := []string{
		"%Y-%m-%d", "%Y-%m-%dT%H:%M:%S%z", "%Y-%m-%dT%H:%M:%S%.fZ",
		"%a, %d %b %Y %H:%M:%S GMT", "%+", "%s", "%G-W%V-%u", "%y/%m/%d %H:%M:%S",
		"%-j%_e%0H", "%#z",
	}
	inputSeeds := []string{
		"", "2001-07-08", "2001-07-08T00:34:59+09:30", "2001-07-08T00:34:59.5Z",
		"Wed, 13 Jan 2021 22:23:01 GMT", "not a date", "9999999999999999999",
		"+10000-09-09 01:46:39", "2015-02-30 12:34:56", "🤠", "-", "+", "Z",
	}
	for _, l := range layoutSeeds {
		for _, in := range inputSeeds {
			f.Add(in, l)
		}
	}
	f.Fuzz(func(t *testing.T, s, layout string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("parse(%q, %q) panicked: %v", s, layout, r)
			}
		}()
		_, _ = ParseDateTime(s, layout)
		_, _ = ParseNaiveDateTime(s, layout)
		_, _ = ParseNaiveDate(s, layout)
		_, _ = ParseRFC3339(s)
		_, _ = ParseRFC2822(s)
	})
}
