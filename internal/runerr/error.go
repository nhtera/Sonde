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
	// AssertStatus: Actual (status code).
	AssertStatus
	// AssertVersion: Actual (HTTP version).
	AssertVersion
	// AssertHeaderValue: Actual (header value).
	AssertHeaderValue
	// AssertBodyValue: Actual (body text).
	AssertBodyValue
	// AssertBodyDiff: Reason (first diff hunk, one "+"/"-"/" " prefixed
	// line per body line), Value (line of the body start, decimal).
	AssertBodyDiff
	// InvalidOptionValue: Name, Value, Reason.
	InvalidOptionValue
	// NoFilterResult: no field.
	NoFilterResult
	// PossibleLoggedSecret: no field.
	PossibleLoggedSecret
	// UnsupportedSecretType: Actual (kind of the value).
	UnsupportedSecretType
	// FileWriteAccess: Value (path), Reason.
	FileWriteAccess
	// BinaryOutput: no field.
	BinaryOutput
	// ContractViolation: Reason, Name (instance path, optional), Value
	// (spec pointer, optional).
	ContractViolation
	// Stream is a failure of a streamed entry (a WebSocket step, a
	// sondeStream field): Value (description), Reason (message).
	Stream
	// GRPC is a failed gRPC call (a status other than OK that the entry
	// does not check, descriptors, messages): Value (description), Reason
	// (message).
	GRPC

	// KindCount is the number of kinds; new kinds go above it.
	KindCount
)

// Error is a runtime error at a source span. Assert is set when the error
// was raised while evaluating an assert, which makes it an assert failure
// rather than a runtime error for the exit code.
type Error struct {
	Span   syntax.Span
	Kind   Kind
	Assert bool

	Name         string
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
	case HTTP, Stream, GRPC:
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
	case AssertStatus:
		return "Assert status code"
	case AssertVersion:
		return "Assert HTTP version"
	case AssertHeaderValue:
		return "Assert header value"
	case AssertBodyValue, AssertBodyDiff:
		return "Assert body value"
	case InvalidOptionValue:
		return "Invalid option value"
	case NoFilterResult:
		return "Filter error"
	case PossibleLoggedSecret:
		return "Invalid redacted secret"
	case UnsupportedSecretType:
		return "Invalid secret type"
	case FileWriteAccess:
		return "File write access"
	case BinaryOutput:
		return "Binary output"
	case ContractViolation:
		return "Contract violation"
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
	case HTTP, Stream, GRPC:
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
	case AssertStatus, AssertVersion, AssertHeaderValue, AssertBodyValue:
		return "actual value is <" + e.Actual + ">"
	case AssertBodyDiff:
		return e.Reason
	case InvalidOptionValue:
		return "invalid " + e.Name + " option value <" + e.Value + ">: " + e.Reason
	case NoFilterResult:
		return "a filter didn't return any result"
	case PossibleLoggedSecret:
		return "redacted secret not authorized in verbose"
	case UnsupportedSecretType:
		return "secret must be string, actual value is <" + e.Actual + ">"
	case FileWriteAccess:
		return e.Value + " can not be written (" + e.Reason + ")"
	case ContractViolation:
		msg := e.Reason
		if e.Name != "" {
			msg += "\nat: " + e.Name
		}
		if e.Value != "" {
			msg += "\nspec: " + e.Value
		}
		return msg
	case BinaryOutput:
		return `binary output can mess up your terminal. Use "--output -" to tell sonde to output it to your terminal anyway, or consider "--output" to save to a file`
	}
	return e.Reason
}
