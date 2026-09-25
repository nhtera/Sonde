// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

// ErrorKind categorizes why a format or parse operation failed. The set of
// kinds and their meaning mirrors chrono's ParseErrorKind.
type ErrorKind int

const (
	// OutOfRange means a field was set to a value outside its permitted
	// range, or a resolved date or time value has no valid representation.
	OutOfRange ErrorKind = iota
	// Impossible means there is no possible date and time value matching
	// the given set of fields (they are inconsistent with each other).
	Impossible
	// NotEnough means the given set of fields is not enough to resolve a
	// unique date and time value.
	NotEnough
	// Invalid means the input has an invalid character sequence for the
	// expected formatting item.
	Invalid
	// TooShort means the input ended prematurely.
	TooShort
	// TooLong means all formatting items were consumed but input remains.
	TooLong
	// BadFormat means the format string itself is invalid or contains an
	// unsupported specifier.
	BadFormat
)

func (k ErrorKind) String() string {
	switch k {
	case OutOfRange:
		return "input is out of range"
	case Impossible:
		return "no possible date and time matching input"
	case NotEnough:
		return "input is not enough for unique date and time"
	case Invalid:
		return "input contains invalid characters"
	case TooShort:
		return "premature end of input"
	case TooLong:
		return "trailing input"
	case BadFormat:
		return "bad or unsupported format string"
	default:
		return "unknown parse error"
	}
}

// Error reports why formatting or parsing a date/time value failed. Its
// Error() text matches chrono's ParseError Display output for the
// corresponding ErrorKind.
type Error struct {
	kind ErrorKind
}

// Kind returns the category of this error.
func (e *Error) Kind() ErrorKind { return e.kind }

func (e *Error) Error() string { return e.kind.String() }

func newError(kind ErrorKind) *Error { return &Error{kind: kind} }

var (
	errOutOfRange = newError(OutOfRange)
	errImpossible = newError(Impossible)
	errNotEnough  = newError(NotEnough)
	errInvalid    = newError(Invalid)
	errTooShort   = newError(TooShort)
	errTooLong    = newError(TooLong)
	errBadFormat  = newError(BadFormat)
)
