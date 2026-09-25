// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import "github.com/nhtera/sonde/internal/value"

// tryValueTypeFunction parses a call to one of the three functions
// returning ValueType: length(), count(), value().
func tryValueTypeFunction(r *reader) (valueTypeFunction, bool, error) {
	switch {
	case matchStr("length", r):
		arg, err := callArg(r, parseValueTypeArgument)
		if err != nil {
			return valueTypeFunction{}, false, err
		}
		return valueTypeFunction{kind: fnLength, lengthArg: &arg}, true, nil
	case matchStr("value", r):
		arg, err := callArg(r, parseNodesTypeArgument)
		if err != nil {
			return valueTypeFunction{}, false, err
		}
		return valueTypeFunction{kind: fnValue, nodesArg: &arg}, true, nil
	case matchStr("count", r):
		arg, err := callArg(r, parseNodesTypeArgument)
		if err != nil {
			return valueTypeFunction{}, false, err
		}
		return valueTypeFunction{kind: fnCount, nodesArg: &arg}, true, nil
	}
	return valueTypeFunction{}, false, nil
}

// tryLogicalTypeFunction parses a call to match() or search(), the two
// functions returning LogicalType.
func tryLogicalTypeFunction(r *reader) (logicalTypeFunction, bool, error) {
	switch {
	case matchStr("match", r):
		a1, a2, err := call2Arg(r, parseValueTypeArgument, parseRegexValueTypeArgument)
		if err != nil {
			return logicalTypeFunction{}, false, err
		}
		return logicalTypeFunction{kind: fnMatch, strArg: a1, patArg: a2}, true, nil
	case matchStr("search", r):
		a1, a2, err := call2Arg(r, parseValueTypeArgument, parseRegexValueTypeArgument)
		if err != nil {
			return logicalTypeFunction{}, false, err
		}
		return logicalTypeFunction{kind: fnSearch, strArg: a1, patArg: a2}, true, nil
	}
	return logicalTypeFunction{}, false, nil
}

// callArg parses "(" arg ")" for a one-argument function call.
func callArg[T any](r *reader, parseArg func(*reader) (T, error)) (T, error) {
	var zero T
	if err := expectStr("(", r); err != nil {
		return zero, err
	}
	skipWhitespace(r)
	arg, err := parseArg(r)
	if err != nil {
		return zero, err
	}
	skipWhitespace(r)
	if err := expectStr(")", r); err != nil {
		return zero, err
	}
	return arg, nil
}

// call2Arg parses "(" arg1 "," arg2 ")" for a two-argument function call.
func call2Arg[A, B any](r *reader, parseA func(*reader) (A, error), parseB func(*reader) (B, error)) (A, B, error) {
	var zeroA A
	var zeroB B
	if err := expectStr("(", r); err != nil {
		return zeroA, zeroB, err
	}
	skipWhitespace(r)
	a, err := parseA(r)
	if err != nil {
		return zeroA, zeroB, err
	}
	skipWhitespace(r)
	if err := expectStr(",", r); err != nil {
		return zeroA, zeroB, err
	}
	skipWhitespace(r)
	b, err := parseB(r)
	if err != nil {
		return zeroA, zeroB, err
	}
	skipWhitespace(r)
	if err := expectStr(")", r); err != nil {
		return zeroA, zeroB, err
	}
	return a, b, nil
}

func parseValueTypeArgument(r *reader) (valueTypeArgument, error) {
	if lit, ok, err := tryLiteral(r); err != nil {
		return valueTypeArgument{}, err
	} else if ok {
		return valueTypeArgument{kind: vArgLiteral, lit: lit}, nil
	}
	if sq, ok, err := trySingularQuery(r); err != nil {
		return valueTypeArgument{}, err
	} else if ok {
		return valueTypeArgument{kind: vArgSingularQuery, sq: sq}, nil
	}
	if fn, ok, err := tryValueTypeFunction(r); err != nil {
		return valueTypeArgument{}, err
	} else if ok {
		return valueTypeArgument{kind: vArgFunction, fn: &fn}, nil
	}
	return valueTypeArgument{}, newParseError(r.cursor(), "expecting a ValueType argument")
}

func parseRegexValueTypeArgument(r *reader) (regexValueTypeArgument, error) {
	if re, ok, err := tryRegexLiteral(r); err != nil {
		return regexValueTypeArgument{}, err
	} else if ok {
		return regexValueTypeArgument{kind: rArgLiteral, lit: re}, nil
	}
	if sq, ok, err := trySingularQuery(r); err != nil {
		return regexValueTypeArgument{}, err
	} else if ok {
		return regexValueTypeArgument{kind: rArgSingularQuery, sq: sq}, nil
	}
	if fn, ok, err := tryValueTypeFunction(r); err != nil {
		return regexValueTypeArgument{}, err
	} else if ok {
		return regexValueTypeArgument{kind: rArgFunction, fn: &fn}, nil
	}
	return regexValueTypeArgument{}, newParseError(r.cursor(), "expecting a RegexValueType argument")
}

// tryRegexLiteral parses a quoted string and compiles it as a regular
// expression right away, so an invalid pattern is a parse error rather
// than a silent no-match at evaluation time.
func tryRegexLiteral(r *reader) (*value.Regex, bool, error) {
	savedPos := r.cursor()
	s, ok, err := tryStringLiteral(r)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	re, cerr := value.NewRegex(s)
	if cerr != nil {
		return nil, false, newParseError(savedPos, "expecting a valid regex")
	}
	return &re, true, nil
}

func parseNodesTypeArgument(r *reader) (nodesTypeArgument, error) {
	fq, ok, err := tryFilterQuery(r)
	if err != nil {
		return nodesTypeArgument{}, err
	}
	if !ok {
		return nodesTypeArgument{}, newParseError(r.cursor(), "expecting a NodesType argument")
	}
	return nodesTypeArgument{query: fq}, nil
}
