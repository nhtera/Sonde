// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package runerr defines the runtime error raised while evaluating a
// request file: templates, queries, filters, predicates and captures.
package runerr

import (
	"fmt"

	"github.com/nhtera/sonde/internal/syntax"
)

// Kind identifies a runtime error. The comment of each kind names the Error
// fields it uses.
type Kind int

// Runtime error kinds.
const (
	// AssertFailure: Actual, Expected, TypeMismatch.
	AssertFailure Kind = iota
	// ExpressionInvalidType: Actual (value repr), Expected (expected kind).
	ExpressionInvalidType
	// FileReadAccess: Value (path).
	FileReadAccess
	// UnauthorizedFileAccess: Value (path).
	UnauthorizedFileAccess
	// FilterDecode: Value (encoding).
	FilterDecode
	// FilterDateParsing: Value (date), Reason (format).
	FilterDateParsing
	// FilterInvalidEncoding: Value (encoding).
	FilterInvalidEncoding
	// FilterInvalidInputValue: Reason.
	FilterInvalidInputValue
	// FilterInvalidInputType: Actual, Expected (kind names).
	FilterInvalidInputType
	// FilterInvalidFormatSpecifier: Value (format).
	FilterInvalidFormatSpecifier
	// FilterMissingInput: no field.
	FilterMissingInput
	// HTTP is an error decoding the response: Value (description), Reason (message).
	HTTP
	// InvalidJSON: Value.
	InvalidJSON
	// InvalidRegex: no field.
	InvalidRegex
	// InvalidURL: Value (URL), Reason.
	InvalidURL
	// InvalidXPathEval: no field.
	InvalidXPathEval
	// NoQueryResult: no field.
	NoQueryResult
	// QueryHeaderNotFound: no field.
	QueryHeaderNotFound
	// QueryInvalidJSON: no field.
	QueryInvalidJSON
	// QueryInvalidJSONPath: Value (expression).
	QueryInvalidJSONPath
	// QueryInvalidXML: no field.
	QueryInvalidXML
	// UndefinedVariable: Value (name).
	UndefinedVariable
	// Unrenderable: Value (display of the value).
	Unrenderable
)

// Error is a runtime error at a source span. Assert is set when the error
// was raised while evaluating an assert, which makes it an assert failure
// rather than a runtime error for the exit code.
type Error struct {
	Span   syntax.Span
	Kind   Kind
	Assert bool

	Actual       string
	Expected     string
	TypeMismatch bool
	Value        string
	Reason       string
}

// New returns an error of kind k at span.
func New(span syntax.Span, k Kind, assert bool) *Error {
	return &Error{Span: span, Kind: k, Assert: assert}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Description(), e.Message())
}

// Description is the error title.
func (e *Error) Description() string {
	switch e.Kind {
	case AssertFailure:
		return "Assert failure"
	case ExpressionInvalidType:
		return "Invalid expression type"
	case FileReadAccess:
		return "File read access"
	case UnauthorizedFileAccess:
		return "Unauthorized file access"
	case FilterDecode, FilterDateParsing, FilterInvalidEncoding, FilterInvalidInputValue,
		FilterInvalidInputType, FilterInvalidFormatSpecifier, FilterMissingInput:
		return "Filter error"
	case HTTP:
		return e.Value
	case InvalidJSON, QueryInvalidJSON:
		return "Invalid JSON"
	case InvalidRegex:
		return "Invalid regex"
	case InvalidURL:
		return "Invalid URL"
	case InvalidXPathEval:
		return "Invalid XPath expression"
	case NoQueryResult:
		return "No query result"
	case QueryHeaderNotFound:
		return "Header not found"
	case QueryInvalidJSONPath:
		return "Invalid JSONPath"
	case QueryInvalidXML:
		return "Invalid XML"
	case UndefinedVariable:
		return "Undefined variable"
	case Unrenderable:
		return "Unrenderable expression"
	}
	return "Runtime error"
}

// Message is the explanation shown under the source line.
func (e *Error) Message() string {
	switch e.Kind {
	case AssertFailure:
		msg := "   actual:   " + e.Actual + "\n   expected: " + e.Expected
		if e.TypeMismatch {
			msg += "\n   >>> types between actual and expected are not consistent"
		}
		return msg
	case ExpressionInvalidType:
		return "expecting " + e.Expected + ", actual value is " + e.Actual
	case FileReadAccess:
		return "file " + e.Value + " can not be read"
	case UnauthorizedFileAccess:
		return "unauthorized access to file " + e.Value + ", check --file-root option"
	case FilterDecode:
		return "value can not be decoded with <" + e.Value + "> encoding"
	case FilterDateParsing:
		return "value <" + e.Value + "> could not be parsed with <" + e.Reason + "> format"
	case FilterInvalidEncoding:
		return "<" + e.Value + "> encoding is not supported"
	case FilterInvalidInputValue:
		return "invalid filter input: " + e.Reason
	case FilterInvalidInputType:
		return "invalid filter input type\n   actual:   " + e.Actual + "\n   expected: " + e.Expected
	case FilterInvalidFormatSpecifier:
		return "date format <" + e.Value + "> is not supported"
	case FilterMissingInput:
		return "missing value to apply filter"
	case HTTP:
		return e.Reason
	case InvalidJSON:
		return "actual value is <" + e.Value + ">"
	case InvalidRegex:
		return "regex expression is not valid"
	case InvalidURL:
		return "invalid URL <" + e.Value + "> (" + e.Reason + ")"
	case InvalidXPathEval:
		return "XPath expression is not valid"
	case NoQueryResult:
		return "query didn't return any result"
	case QueryHeaderNotFound:
		return "this header has not been found in the response"
	case QueryInvalidJSON:
		return "HTTP response is not a valid JSON"
	case QueryInvalidJSONPath:
		return "JSONPath expression '" + e.Value + "' is not valid"
	case QueryInvalidXML:
		return "HTTP response is not a valid XML"
	case UndefinedVariable:
		return "you must set the variable " + e.Value
	case Unrenderable:
		return "expression with value " + e.Value + " can not be rendered"
	}
	return e.Reason
}
