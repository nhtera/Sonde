// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package credential guesses which values hold credentials: those the
// user did not declare secret but that should not be shown or kept.
package credential

import (
	"regexp"
	"unicode/utf8"
)

// tokenName is a name that suggests a credential.
var tokenName = regexp.MustCompile(`(?i)token|secret|key|password|passwd|^pass$|pwd|session|auth|cookie|jwt|bearer`)

// jwtShape is a JWT-like value: three base64url parts.
var jwtShape = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$`)

// Likely reports whether a value named name looks like a credential: by
// its name (token, secret, session…) or its shape (a JWT).
func Likely(name, value string) bool {
	return tokenName.MatchString(name) || len(value) >= 16 && jwtShape.MatchString(value)
}

// MinMasked is the shortest value masked wherever it appears as text: a
// shorter one would mask every string that contains it ("7" in any
// number), so it is masked only where its place is known.
const MinMasked = 8

// Maskable reports whether value is long enough to mask as text.
func Maskable(value string) bool { return utf8.RuneCountInString(value) >= MinMasked }
