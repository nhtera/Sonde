// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

// tryComparisonExpr tries to parse a comparison expression. It returns
// (zero, false, nil) when the input is not a comparison at all (for
// example a bare test expression such as "@.b"), so the caller can try
// another alternative.
func tryComparisonExpr(r *reader) (comparisonExpr, bool, error) {
	save := r.cursor()

	left, ok, err := tryComparable(r)
	if err != nil {
		return comparisonExpr{}, false, err
	}
	if !ok {
		return comparisonExpr{}, false, nil
	}

	skipWhitespace(r)
	op, ok := tryComparisonOp(r)
	if !ok {
		r.seek(save)
		return comparisonExpr{}, false, nil
	}

	skipWhitespace(r)
	right, err := parseComparable(r)
	if err != nil {
		return comparisonExpr{}, false, err
	}

	return comparisonExpr{left: left, right: right, op: op}, true, nil
}

func parseComparable(r *reader) (comparand, error) {
	c, ok, err := tryComparable(r)
	if err != nil {
		return comparand{}, err
	}
	if !ok {
		return comparand{}, newParseError(r.cursor(), "expecting comparable")
	}
	return c, nil
}

func tryComparable(r *reader) (comparand, bool, error) {
	if lit, ok, err := tryLiteral(r); err != nil {
		return comparand{}, false, err
	} else if ok {
		return comparand{kind: cmpLiteral, lit: lit}, true, nil
	}
	if sq, ok, err := trySingularQuery(r); err != nil {
		return comparand{}, false, err
	} else if ok {
		return comparand{kind: cmpSingularQuery, sq: sq}, true, nil
	}
	if fn, ok, err := tryValueTypeFunction(r); err != nil {
		return comparand{}, false, err
	} else if ok {
		return comparand{kind: cmpFunction, fn: &fn}, true, nil
	}
	return comparand{}, false, nil
}

func tryComparisonOp(r *reader) (comparisonOp, bool) {
	switch {
	case matchStr("==", r):
		return opEqual, true
	case matchStr("!=", r):
		return opNotEqual, true
	case matchStr("<=", r):
		return opLessOrEqual, true
	case matchStr("<", r):
		return opLess, true
	case matchStr(">=", r):
		return opGreaterOrEqual, true
	case matchStr(">", r):
		return opGreater, true
	}
	return 0, false
}
