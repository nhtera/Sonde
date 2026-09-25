// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package datefmt reproduces the date/time formatting and parsing
// semantics of the Rust chrono crate (v0.4.44, built with
// default-features = false, features = ["clock"], i.e. without its
// locale support), for strftime-style format strings as used by
// chrono's DateTime::format, DateTime::parse_from_str,
// NaiveDateTime::parse_from_str, NaiveDate::parse_from_str,
// DateTime::parse_from_rfc3339 and DateTime::parse_from_rfc2822.
//
// Every value formatted or produced by parsing is a plain time.Time
// representing an absolute instant; there is no notion of a "naive" (no
// offset) value or of a location other than UTC. Format always treats its
// input as a UTC instant. ParseNaiveDateTime and ParseNaiveDate interpret
// their input as already being in UTC (chrono's "naive" types carry no
// offset at all; this package represents that as UTC). ParseDateTime,
// ParseRFC3339 and ParseRFC2822 require an offset in the input and return
// the corresponding UTC instant.
//
// # Supported specifiers
//
// All specifiers documented for chrono's format::strftime module are
// supported: date specifiers (%Y %C %y %q %m %b %B %h %d %e %a %A %w %u
// %U %W %G %g %V %j %D %x %F %v), time specifiers (%H %k %I %l %P %p %M
// %S %f %.f %.3f %.6f %.9f %3f %6f %9f %R %T %X %r), timezone specifiers
// (%Z %z %:z %::z %:::z %#z, parse-only), the combined %c and %+
// specifiers, %s, and the literal specifiers %t %n %%. Padding modifiers
// (%-?, %_?, %0?) are supported on numeric specifiers. Month, weekday and
// am/pm names are always the fixed English ones chrono uses without its
// "unstable-locales" feature.
//
// # Deviations from chrono
//
// Two deliberate deviations exist, both a consequence of Go's time.Time
// having no way to represent a leap second (a wall-clock time of exactly
// :60 whose instant is nonetheless treated by chrono as no later than
// :59.999999999):
//
//   - Format never observes a leap-second instant, because no time.Time
//     value can hold one to begin with.
//   - Parsing a literal ":60" second is accepted (as chrono does), but the
//     resulting instant is the following second (e.g. 12:00:60 becomes
//     12:01:00 plus any parsed fractional seconds), rather than chrono's
//     cosmetic-only leap second that leaves the instant at :59. This
//     preserves any parsed fractional-second precision and produces a
//     valid, orderable instant; the timestamp field ("%s"), when also
//     present, is still cross-checked against chrono's (non-rolled-over)
//     value for consistency.
//
// Years are supported at least in the exact range 0..9999. Chrono itself
// allows years from -262144 to 262143; this package additionally accepts
// years outside 0..9999 (down to what a Go time.Time/int can represent)
// without panicking, but does not guarantee exact parity with chrono for
// years outside chrono's own documented range or outside what time.Time
// can hold.
package datefmt
