// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package predicate evaluates assert predicates (`==`, `contains`,
// `isInteger`, …, optionally negated with `not`).
package predicate

import (
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

// result is the outcome of a predicate function before `not` applies.
type result struct {
	success      bool
	typeMismatch bool
	actual       string
	expected     string
}

// Eval evaluates p against the actual value (nil when the query returned
// nothing). It returns nil when the assert holds, a *runerr.Error of kind
// AssertFailure when it does not, or another *runerr.Error when the
// expected value cannot be evaluated.
func Eval(p *syntax.Predicate, actual value.Value, env *template.Env) error {
	r, err := evalFunc(p.Func, actual, env)
	if err != nil {
		return err
	}
	fail := func(expected string, mismatch bool) error {
		// Column 0: the failure is reported on the line, without carets.
		line := p.Func.Span.Start.Line
		span := syntax.Span{Start: syntax.Pos{Line: line}, End: syntax.Pos{Line: line}}
		e := runerr.New(span, runerr.AssertFailure, true)
		e.Actual, e.Expected, e.TypeMismatch = r.actual, expected, mismatch
		return e
	}
	// The expected value reads "not ..." when the predicate is negated
	// once: by `not`, or by `!=` (which negates equality), not both.
	expected := r.expected
	if p.Not != (p.Func.Kind == syntax.PredicateNotEqual) {
		expected = "not " + expected
	}
	switch {
	case r.typeMismatch:
		return fail(expected, true)
	case p.Not == r.success:
		return fail(expected, false)
	}
	return nil
}

func evalFunc(f *syntax.PredicateFunc, actual value.Value, env *template.Env) (result, error) {
	if actual == nil {
		expected, err := expectedNoValue(f, env)
		return result{actual: "none", expected: expected}, err
	}
	var expected value.Value
	if f.Value != nil {
		var err error
		if expected, err = Value(f.Value, f.Span, env); err != nil {
			return result{}, err
		}
	}
	switch f.Kind {
	case syntax.PredicateEqual:
		return result{success: value.Equal(actual, expected), actual: value.Repr(actual), expected: value.Repr(expected)}, nil
	case syntax.PredicateNotEqual:
		return result{success: !value.Equal(actual, expected), actual: value.Repr(actual), expected: value.Repr(expected)}, nil
	case syntax.PredicateGreater:
		return compare(actual, expected, "greater than ", func(o value.Ordering) bool { return o == value.Greater }), nil
	case syntax.PredicateGreaterOrEqual:
		return compare(actual, expected, "greater or equal than ", func(o value.Ordering) bool { return o == value.Greater || o == value.Same }), nil
	case syntax.PredicateLess:
		return compare(actual, expected, "less than ", func(o value.Ordering) bool { return o == value.Less }), nil
	case syntax.PredicateLessOrEqual:
		return compare(actual, expected, "less or equal than ", func(o value.Ordering) bool { return o == value.Less || o == value.Same }), nil
	case syntax.PredicateStartWith:
		ok, typed := startsWith(actual, expected)
		return result{success: ok, typeMismatch: !typed, actual: value.Repr(actual), expected: "starts with " + value.Repr(expected)}, nil
	case syntax.PredicateEndWith:
		ok, typed := endsWith(actual, expected)
		return result{success: ok, typeMismatch: !typed, actual: value.Repr(actual), expected: "ends with " + value.Repr(expected)}, nil
	case syntax.PredicateContain:
		ok, typed := contains(actual, expected)
		return result{success: ok, typeMismatch: !typed, actual: value.Repr(actual), expected: "contains " + value.Repr(expected)}, nil
	case syntax.PredicateInclude:
		list, typed := actual.(value.List)
		return result{success: typed && includes(list, expected), typeMismatch: !typed, actual: value.Repr(actual), expected: "includes " + value.Repr(expected)}, nil
	case syntax.PredicateMatch:
		return match(f, actual, expected)
	case syntax.PredicateExist:
		n, isNodeset := actual.(value.Nodeset)
		return result{success: !isNodeset || n > 0, actual: value.Repr(actual), expected: "something"}, nil
	case syntax.PredicateIsEmpty:
		n, ok := count(actual)
		if !ok {
			return result{typeMismatch: true, actual: value.Repr(actual), expected: "count equals to 0"}, nil
		}
		return result{success: n == 0, actual: "count equals to " + itoa(n), expected: "count equals to 0"}, nil
	case syntax.PredicateIsIsoDate:
		return stringCheck(actual, "string with format YYYY-MM-DDTHH:mm:ss.sssZ", isISODate), nil
	case syntax.PredicateIsIPv4:
		return stringCheck(actual, "string in IPv4 format", isIPv4), nil
	case syntax.PredicateIsIPv6:
		return stringCheck(actual, "string in IPv6 format", isIPv6), nil
	case syntax.PredicateIsUUID:
		return stringCheck(actual, "string in UUID format", isUUID), nil
	}
	name, ok := kindCheck(f.Kind, actual)
	return result{success: ok, actual: value.Repr(actual), expected: name}, nil
}

// Value evaluates a predicate value; span locates errors on a literal.
func Value(v syntax.PredicateValue, span syntax.Span, env *template.Env) (value.Value, error) {
	switch v := v.(type) {
	case *syntax.Template:
		s, err := env.Render(v)
		return value.String(s), err
	case *syntax.MultilineString:
		s, err := env.RenderMultiline(v)
		return value.String(s), err
	case *syntax.Boolean:
		return value.Bool(v.Value), nil
	case *syntax.Null:
		return value.Null{}, nil
	case *syntax.Number:
		switch v.Kind {
		case syntax.NumberFloat:
			return value.Float(v.Float), nil
		case syntax.NumberBigInteger:
			return value.BigInt(v.Source), nil
		}
		return value.Int(v.Int), nil
	case *syntax.FileRef:
		b, err := env.File(v.Filename)
		return value.Bytes(b), err
	case *syntax.Hex:
		return value.Bytes(v.Value), nil
	case *syntax.Base64:
		return value.Bytes(v.Value), nil
	case *syntax.Placeholder:
		return env.Eval(v.Expr)
	case *syntax.Regex:
		return env.Regex(v, span)
	}
	return value.Null{}, nil
}

func compare(actual, expected value.Value, prefix string, ok func(value.Ordering) bool) result {
	r := result{actual: value.Repr(actual), expected: prefix + value.Repr(expected)}
	o, err := value.Compare(actual, expected)
	if err != nil {
		r.typeMismatch = true
		return r
	}
	r.success = ok(o)
	return r
}

func match(f *syntax.PredicateFunc, actual, expected value.Value) (result, error) {
	r := result{actual: value.Repr(actual), expected: "matches regex <" + value.Display(expected) + ">"}
	var re value.Regex
	switch e := expected.(type) {
	case value.Regex:
		re = e
	case value.String:
		var err error
		if re, err = value.NewRegex(string(e)); err != nil {
			return result{}, runerr.New(f.Span, runerr.InvalidRegex, false)
		}
	default:
		r.typeMismatch = true
		return r, nil
	}
	s, ok := actual.(value.String)
	if !ok {
		r.typeMismatch = true
		return r, nil
	}
	r.success = re.Re.MatchString(string(s))
	return r, nil
}

// stringCheck applies a format check to a string; other kinds are a type
// mismatch. The actual value is shown without its kind.
func stringCheck(actual value.Value, expected string, check func(string) bool) result {
	s, ok := actual.(value.String)
	if !ok {
		return result{typeMismatch: true, actual: value.Repr(actual), expected: "string"}
	}
	return result{success: check(string(s)), actual: string(s), expected: expected}
}

// expectedNoValue describes the expectation when the query returned nothing.
func expectedNoValue(f *syntax.PredicateFunc, env *template.Env) (string, error) {
	var expected value.Value
	if f.Value != nil && f.Kind != syntax.PredicateMatch {
		var err error
		if expected, err = Value(f.Value, f.Span, env); err != nil {
			return "", err
		}
	}
	switch f.Kind {
	case syntax.PredicateEqual, syntax.PredicateNotEqual:
		return value.Expected(expected), nil
	case syntax.PredicateGreater:
		return "greater than <" + value.Expected(expected) + ">", nil
	case syntax.PredicateGreaterOrEqual:
		return "greater than or equals to <" + value.Expected(expected) + ">", nil
	case syntax.PredicateLess:
		return "less than <" + value.Expected(expected) + ">", nil
	case syntax.PredicateLessOrEqual:
		return "less than or equals to <" + value.Expected(expected) + ">", nil
	case syntax.PredicateStartWith:
		return "starts with " + value.Expected(expected), nil
	case syntax.PredicateEndWith:
		return "ends with " + value.Expected(expected), nil
	case syntax.PredicateContain:
		return "contains " + value.Expected(expected), nil
	case syntax.PredicateInclude:
		return "include " + value.Expected(expected), nil
	case syntax.PredicateMatch:
		return matchPattern(f, env)
	case syntax.PredicateExist:
		return "something", nil
	case syntax.PredicateIsEmpty:
		return "empty", nil
	case syntax.PredicateIsIsoDate:
		return "date", nil
	case syntax.PredicateIsIPv4:
		return "ipv4", nil
	case syntax.PredicateIsIPv6:
		return "ipv6", nil
	case syntax.PredicateIsUUID:
		return "uuid", nil
	}
	name, _ := kindCheck(f.Kind, value.Null{})
	return name, nil
}

// matchPattern renders the pattern of a match predicate as text.
func matchPattern(f *syntax.PredicateFunc, env *template.Env) (string, error) {
	var pattern string
	switch v := f.Value.(type) {
	case *syntax.Regex:
		pattern = v.Pattern
	case *syntax.Template:
		s, err := env.Render(v)
		if err != nil {
			return "", err
		}
		pattern = s
	default:
		x, err := Value(v, f.Span, env)
		if err != nil {
			return "", err
		}
		pattern = value.Display(x)
	}
	return "matches regex <" + pattern + ">", nil
}
