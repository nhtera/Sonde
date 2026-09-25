// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package value

import (
	"fmt"
	"strings"
	"time"
)

// displayDate formats t as `2000-01-01 12:00:00.123 UTC`: fractional
// seconds are omitted when zero and otherwise shown with 3, 6 or 9 digits,
// whichever is exact.
func displayDate(t time.Time) string {
	var b strings.Builder
	b.WriteString(FormatYear(t.Year()))
	fmt.Fprintf(&b, "-%02d-%02d %02d:%02d:%02d", int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	switch ns := t.Nanosecond(); {
	case ns == 0:
	case ns%1_000_000 == 0:
		fmt.Fprintf(&b, ".%03d", ns/1_000_000)
	case ns%1_000 == 0:
		fmt.Fprintf(&b, ".%06d", ns/1_000)
	default:
		fmt.Fprintf(&b, ".%09d", ns)
	}
	b.WriteString(" UTC")
	return b.String()
}

// FormatYear writes a year with at least four digits; years outside
// 0..9999 carry an explicit sign.
func FormatYear(y int) string {
	if y >= 0 && y <= 9999 {
		return fmt.Sprintf("%04d", y)
	}
	return fmt.Sprintf("%+05d", y)
}
