// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import "github.com/nhtera/sonde/internal/value"

// This file holds the JSONPath grammar (RFC 9535) as a tree of tagged
// structs: one Go type per grammar production, with a kind field selecting
// which of a production's alternatives applies. A tagged struct was chosen
// over one interface and type per alternative to keep the number of types
// small; eval.go dispatches on the kind fields.

// segment is a child ("[...]", ".name", ".*") or descendant ("..") segment.
type segmentKind int

const (
	segChild segmentKind = iota
	segDescendant
)

type segment struct {
	kind      segmentKind
	selectors []selector
}

// selector is one of the five selectors a bracketed or shorthand segment
// may hold.
type selectorKind int

const (
	selName selectorKind = iota
	selWildcard
	selIndex
	selSlice
	selFilter
)

type selector struct {
	kind selectorKind

	name  string // selName
	index int64  // selIndex

	sliceStart *int64 // selSlice
	sliceEnd   *int64
	sliceStep  int64

	filter logicalExpr // selFilter
}

// filterQuery is a query embedded in a filter selector or a function
// argument: absolute ("$...") or relative to the current node ("@...").
type filterQuery struct {
	absolute bool
	segments []segment
}

// logicalExpr is a filter-selector predicate.
type exprKind int

const (
	exprComparison exprKind = iota
	exprTest
	exprAnd
	exprOr
	exprNot
)

type logicalExpr struct {
	kind exprKind

	cmp *comparisonExpr // exprComparison

	testNot   bool                 // exprTest
	testQuery *filterQuery         // exprTest, when it wraps a filter-query
	testFn    *logicalTypeFunction // exprTest, when it wraps match()/search()

	operands []logicalExpr // exprAnd, exprOr

	notExpr *logicalExpr // exprNot
}

// comparisonOp is one of the six comparison operators. !=, <=, > and >=
// are evaluated in terms of == and <, per RFC 9535 §2.3.5.2.
type comparisonOp int

const (
	opEqual comparisonOp = iota
	opNotEqual
	opLessOrEqual
	opLess
	opGreaterOrEqual
	opGreater
)

type comparisonExpr struct {
	left, right comparand
	op          comparisonOp
}

// comparand is one side of a comparison: a literal, a singular query, or
// a function returning ValueType.
type comparandKind int

const (
	cmpLiteral comparandKind = iota
	cmpSingularQuery
	cmpFunction
)

type comparand struct {
	kind comparandKind
	lit  literal
	sq   singularQuery
	fn   *valueTypeFunction
}

// literal is a JSON primitive appearing in a filter expression.
type literalKind int

const (
	litNull literalKind = iota
	litBool
	litNumber
	litString
)

type numberKind int

const (
	numInt numberKind = iota
	numBig
	numFloat
)

type literal struct {
	kind literalKind
	b    bool
	s    string
	num  numberKind
	i    int64
	big  string
	f    float64
}

func (l literal) eval() value.Value {
	switch l.kind {
	case litNull:
		return value.Null{}
	case litBool:
		return value.Bool(l.b)
	case litString:
		return value.String(l.s)
	case litNumber:
		switch l.num {
		case numInt:
			return value.Int(l.i)
		case numBig:
			return value.BigInt(l.big)
		case numFloat:
			return value.Float(l.f)
		}
	}
	return value.Null{}
}

// singularQuery is a query guaranteed to select at most one node: a chain
// of name and index segments, absolute or relative to the current node.
type singularQuerySegment struct {
	isName bool
	name   string
	index  int64
}

type singularQuery struct {
	absolute bool
	segments []singularQuerySegment
}

// Function extensions (RFC 9535 §2.4): length, count and value return
// ValueType; match and search return LogicalType.
type valueTypeFnKind int

const (
	fnLength valueTypeFnKind = iota
	fnCount
	fnValue
)

type valueTypeFunction struct {
	kind      valueTypeFnKind
	lengthArg *valueTypeArgument // fnLength
	nodesArg  *nodesTypeArgument // fnCount, fnValue
}

type valueTypeArgKind int

const (
	vArgLiteral valueTypeArgKind = iota
	vArgSingularQuery
	vArgFunction
)

type valueTypeArgument struct {
	kind valueTypeArgKind
	lit  literal
	sq   singularQuery
	fn   *valueTypeFunction
}

type nodesTypeArgument struct {
	query filterQuery
}

type logicalTypeFnKind int

const (
	fnMatch logicalTypeFnKind = iota
	fnSearch
)

type logicalTypeFunction struct {
	kind   logicalTypeFnKind
	strArg valueTypeArgument
	patArg regexValueTypeArgument
}

// regexValueTypeArgument is an argument to match()/search() that must
// evaluate to a regular expression: a string literal compiled at parse
// time, or a query/function evaluated (and compiled) at eval time.
type regexArgKind int

const (
	rArgLiteral regexArgKind = iota
	rArgSingularQuery
	rArgFunction
)

type regexValueTypeArgument struct {
	kind regexArgKind
	lit  *value.Regex
	sq   singularQuery
	fn   *valueTypeFunction
}
