// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/value"
)

func evalValueTypeArgument(a valueTypeArgument, current, root value.Value) (value.Value, bool) {
	switch a.kind {
	case vArgLiteral:
		return a.lit.eval(), true
	case vArgSingularQuery:
		return evalSingularQuery(a.sq, current, root)
	case vArgFunction:
		return evalValueTypeFunction(*a.fn, current, root)
	}
	return nil, false
}

func evalNodesTypeArgument(a nodesTypeArgument, current, root value.Value) []value.Value {
	return a.query.eval(current, root)
}

func evalValueTypeFunction(f valueTypeFunction, current, root value.Value) (value.Value, bool) {
	switch f.kind {
	case fnLength:
		v, ok := evalValueTypeArgument(*f.lengthArg, current, root)
		if !ok {
			return nil, false
		}
		n, ok := lengthOf(v)
		if !ok {
			return nil, false
		}
		return value.Int(n), true
	case fnCount:
		nodes := evalNodesTypeArgument(*f.nodesArg, current, root)
		return value.Int(len(nodes)), true
	case fnValue:
		nodes := evalNodesTypeArgument(*f.nodesArg, current, root)
		if len(nodes) == 1 {
			return nodes[0], true
		}
		return nil, false
	}
	return nil, false
}

// lengthOf implements the length() function: the element count of an
// array or object, or the Unicode scalar value count of a string.
func lengthOf(v value.Value) (int64, bool) {
	switch vv := v.(type) {
	case value.List:
		return int64(len(vv)), true
	case value.Object:
		return int64(len(vv)), true
	case value.String:
		return int64(utf8.RuneCountInString(string(vv))), true
	}
	return 0, false
}

func evalRegexArgument(a regexValueTypeArgument, current, root value.Value) (value.Regex, bool) {
	switch a.kind {
	case rArgLiteral:
		return *a.lit, true
	case rArgSingularQuery:
		v, ok := evalSingularQuery(a.sq, current, root)
		if !ok {
			return value.Regex{}, false
		}
		return compileRegexArg(v)
	case rArgFunction:
		v, ok := evalValueTypeFunction(*a.fn, current, root)
		if !ok {
			return value.Regex{}, false
		}
		return compileRegexArg(v)
	}
	return value.Regex{}, false
}

func compileRegexArg(v value.Value) (value.Regex, bool) {
	s, ok := v.(value.String)
	if !ok {
		return value.Regex{}, false
	}
	re, err := value.NewRegex(string(s))
	if err != nil {
		return value.Regex{}, false
	}
	return re, true
}

func evalLogicalTypeFunction(f logicalTypeFunction, current, root value.Value) bool {
	sv, sok := evalValueTypeArgument(f.strArg, current, root)
	s, isStr := sv.(value.String)
	if !sok || !isStr {
		return false
	}
	re, ok := evalRegexArgument(f.patArg, current, root)
	if !ok {
		return false
	}
	switch f.kind {
	case fnMatch:
		// The whole string must match: anchor the source pattern and
		// recompile, rather than reusing the unanchored compiled regex.
		anchored, err := value.NewRegex("^" + re.Source + "$")
		if err != nil {
			return false
		}
		return anchored.Re.MatchString(string(s))
	case fnSearch:
		return re.Re.MatchString(string(s))
	}
	return false
}
