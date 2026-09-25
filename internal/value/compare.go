// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package value

import (
	"bytes"
	"errors"
	"strings"
)

// ErrType reports values whose kinds cannot be compared.
var ErrType = errors.New("types between actual and expected are not consistent")

// Equal reports whether a and b are equal. Numbers compare by value across
// representations; lists and objects compare member by member, in order.
// Regex and HTTPResponse values are never equal to anything.
func Equal(a, b Value) bool {
	if isNumber(a) && isNumber(b) {
		return compareNumbers(a, b) == Same
	}
	switch a := a.(type) {
	case Null:
		_, ok := b.(Null)
		return ok
	case Unit:
		_, ok := b.(Unit)
		return ok
	case Bool:
		b, ok := b.(Bool)
		return ok && a == b
	case String:
		b, ok := b.(String)
		return ok && a == b
	case Bytes:
		b, ok := b.(Bytes)
		return ok && bytes.Equal(a, b)
	case Date:
		b, ok := b.(Date)
		return ok && a.UTC().Equal(b.UTC())
	case Nodeset:
		b, ok := b.(Nodeset)
		return ok && a == b
	case List:
		b, ok := b.(List)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !Equal(a[i], b[i]) {
				return false
			}
		}
		return true
	case Object:
		b, ok := b.(Object)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i].Key != b[i].Key || !Equal(a[i].Value, b[i].Value) {
				return false
			}
		}
		return true
	}
	return false
}

// Compare orders two strings (byte-wise), two numbers or two dates. Other
// combinations return ErrType. Comparing with NaN returns Unordered.
func Compare(a, b Value) (Ordering, error) {
	if isNumber(a) && isNumber(b) {
		return compareNumbers(a, b), nil
	}
	switch a := a.(type) {
	case String:
		if b, ok := b.(String); ok {
			return Ordering(strings.Compare(string(a), string(b))), nil
		}
	case Date:
		if b, ok := b.(Date); ok {
			return Ordering(a.UTC().Compare(b.UTC())), nil
		}
	}
	return Unordered, ErrType
}
