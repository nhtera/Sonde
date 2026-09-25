// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

// tryLiteral parses a JSON primitive: null, a boolean, a number, or a
// single- or double-quoted string. It returns (zero, false, nil) when the
// input does not start a literal at all, and a non-nil error when it
// starts one but is malformed (for example "1." with no digit after the
// point).
func tryLiteral(r *reader) (literal, bool, error) {
	if matchStr("null", r) {
		return literal{kind: litNull}, true, nil
	}
	if matchStr("true", r) {
		return literal{kind: litBool, b: true}, true, nil
	}
	if matchStr("false", r) {
		return literal{kind: litBool, b: false}, true, nil
	}
	if n, ok, err := tryNumber(r); err != nil {
		return literal{}, false, err
	} else if ok {
		return literal{kind: litNumber, num: n.kind, i: n.i, big: n.big, f: n.f}, true, nil
	}
	if s, ok, err := tryStringLiteral(r); err != nil {
		return literal{}, false, err
	} else if ok {
		return literal{kind: litString, s: s}, true, nil
	}
	return literal{}, false, nil
}
