// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"math"
	"strconv"

	"github.com/nhtera/sonde/internal/value"
)

func evalLogicalExpr(e logicalExpr, current, root value.Value) bool {
	switch e.kind {
	case exprComparison:
		return evalComparison(*e.cmp, current, root)
	case exprTest:
		var v bool
		if e.testQuery != nil {
			v = len(e.testQuery.eval(current, root)) > 0
		} else {
			v = evalLogicalTypeFunction(*e.testFn, current, root)
		}
		if e.testNot {
			return !v
		}
		return v
	case exprAnd:
		for _, op := range e.operands {
			if !evalLogicalExpr(op, current, root) {
				return false
			}
		}
		return true
	case exprOr:
		for _, op := range e.operands {
			if evalLogicalExpr(op, current, root) {
				return true
			}
		}
		return false
	case exprNot:
		return !evalLogicalExpr(*e.notExpr, current, root)
	}
	return false
}

func evalComparison(e comparisonExpr, current, root value.Value) bool {
	left, lok := evalComparable(e.left, current, root)
	right, rok := evalComparable(e.right, current, root)
	switch e.op {
	case opEqual:
		return isEqualOpt(left, lok, right, rok)
	case opNotEqual:
		return !isEqualOpt(left, lok, right, rok)
	case opLess:
		return isLessOpt(left, lok, right, rok)
	case opLessOrEqual:
		return isLessOpt(left, lok, right, rok) || isEqualOpt(left, lok, right, rok)
	case opGreater:
		return isLessOpt(right, rok, left, lok)
	case opGreaterOrEqual:
		return isLessOpt(right, rok, left, lok) || isEqualOpt(left, lok, right, rok)
	}
	return false
}

func evalComparable(c comparand, current, root value.Value) (value.Value, bool) {
	switch c.kind {
	case cmpLiteral:
		return c.lit.eval(), true
	case cmpSingularQuery:
		return evalSingularQuery(c.sq, current, root)
	case cmpFunction:
		return evalValueTypeFunction(*c.fn, current, root)
	}
	return nil, false
}

// isEqualOpt implements RFC 9535's "Nothing" semantics: two absent
// operands (an empty singular query, or a function that produced
// nothing) compare equal to each other and to nothing else. Two numbers
// compare equal within a small epsilon (see numberEquals) rather than
// exactly, so that e.g. 110 and 1.1e2 compare equal despite the latter
// not being exactly representable as a float64.
func isEqualOpt(left value.Value, lok bool, right value.Value, rok bool) bool {
	if !lok && !rok {
		return true
	}
	if !lok || !rok {
		return false
	}
	if isNumberValue(left) && isNumberValue(right) {
		lf, lok2 := numberAsFloat64(left)
		rf, rok2 := numberAsFloat64(right)
		return lok2 && rok2 && numberEquals(lf, rf)
	}
	return value.Equal(left, right)
}

// isLessOpt implements "<": only two strings or two numbers offer an
// order; anything else (including an absent operand) is never less.
// Numbers are compared as float64, within the same epsilon isEqualOpt
// uses, so that a value is never simultaneously "less than" and "equal
// to" another.
func isLessOpt(left value.Value, lok bool, right value.Value, rok bool) bool {
	if !lok || !rok {
		return false
	}
	if l, ok := left.(value.String); ok {
		r, ok := right.(value.String)
		return ok && string(l) < string(r)
	}
	if !isNumberValue(left) || !isNumberValue(right) {
		return false
	}
	lf, lok2 := numberAsFloat64(left)
	rf, rok2 := numberAsFloat64(right)
	return lok2 && rok2 && numberLess(lf, rf)
}

func isNumberValue(v value.Value) bool {
	switch v.(type) {
	case value.Int, value.BigInt, value.Float:
		return true
	}
	return false
}

// numberAsFloat64 widens any numeric value to a float64.
func numberAsFloat64(v value.Value) (float64, bool) {
	switch vv := v.(type) {
	case value.Int:
		return float64(vv), true
	case value.Float:
		return float64(vv), true
	case value.BigInt:
		f, err := strconv.ParseFloat(string(vv), 64)
		return f, err == nil
	}
	return 0, false
}

// numberEqualsEpsilon absorbs the rounding a number picks up going
// through a float64, most visibly for a decimal-fraction-with-exponent
// literal such as 1.1e2.
const numberEqualsEpsilon = 1e-12

func numberEquals(a, b float64) bool {
	return math.Abs(a-b) < numberEqualsEpsilon
}

func numberLess(a, b float64) bool {
	return (b - a) > numberEqualsEpsilon
}

func evalSingularQuery(q singularQuery, current, root value.Value) (value.Value, bool) {
	v := current
	if q.absolute {
		v = root
	}
	for _, seg := range q.segments {
		if seg.isName {
			obj, ok := v.(value.Object)
			if !ok {
				return nil, false
			}
			v, ok = obj.Get(seg.name)
			if !ok {
				return nil, false
			}
			continue
		}
		lst, ok := v.(value.List)
		if !ok {
			return nil, false
		}
		v, ok = indexInto(lst, seg.index)
		if !ok {
			return nil, false
		}
	}
	return v, true
}
