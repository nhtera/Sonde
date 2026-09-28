// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
)

// Pos is a position in a request file. Offset is a byte offset, counted
// after a leading byte order mark if the file has one; Line and Col are
// 1-based, Col counted in runes.
type Pos struct {
	Offset int
	Line   int
	Col    int
}

// Span is a half-open source range [Start, End).
type Span struct {
	Start Pos
	End   Pos
}

func spanOf(s syntax.Span) Span {
	return Span{Start: Pos(s.Start), End: Pos(s.End)}
}

// ErrorKind classifies an Error. New kinds may be added in minor
// versions: switch on it with a default case.
type ErrorKind string

// Error kinds.
const (
	// ErrorParse: the file does not parse (UnitResult.ParseError).
	ErrorParse ErrorKind = "parse"

	ErrorAssertFailure                ErrorKind = "assert-failure"
	ErrorAssertStatus                 ErrorKind = "assert-status"
	ErrorAssertVersion                ErrorKind = "assert-version"
	ErrorAssertHeaderValue            ErrorKind = "assert-header-value"
	ErrorAssertBodyValue              ErrorKind = "assert-body-value"
	ErrorAssertBodyDiff               ErrorKind = "assert-body-diff"
	ErrorContractViolation            ErrorKind = "contract-violation"
	ErrorExpressionInvalidType        ErrorKind = "expression-invalid-type"
	ErrorUndefinedVariable            ErrorKind = "undefined-variable"
	ErrorUnrenderable                 ErrorKind = "unrenderable"
	ErrorInvalidOptionValue           ErrorKind = "invalid-option-value"
	ErrorInvalidURL                   ErrorKind = "invalid-url"
	ErrorInvalidJSON                  ErrorKind = "invalid-json"
	ErrorInvalidRegex                 ErrorKind = "invalid-regex"
	ErrorInvalidXPathEval             ErrorKind = "invalid-xpath-eval"
	ErrorQueryInvalidJSON             ErrorKind = "query-invalid-json"
	ErrorQueryInvalidJSONPath         ErrorKind = "query-invalid-jsonpath"
	ErrorQueryInvalidXML              ErrorKind = "query-invalid-xml"
	ErrorQueryHeaderNotFound          ErrorKind = "query-header-not-found"
	ErrorNoQueryResult                ErrorKind = "no-query-result"
	ErrorNoFilterResult               ErrorKind = "no-filter-result"
	ErrorFilterDecode                 ErrorKind = "filter-decode"
	ErrorFilterDateParsing            ErrorKind = "filter-date-parsing"
	ErrorFilterInvalidEncoding        ErrorKind = "filter-invalid-encoding"
	ErrorFilterInvalidInputValue      ErrorKind = "filter-invalid-input-value"
	ErrorFilterInvalidInputType       ErrorKind = "filter-invalid-input-type"
	ErrorFilterInvalidFormatSpecifier ErrorKind = "filter-invalid-format-specifier"
	ErrorFilterMissingInput           ErrorKind = "filter-missing-input"
	// ErrorHTTP: a transport failure or a body that can't be decoded.
	ErrorHTTP                   ErrorKind = "http"
	ErrorFileReadAccess         ErrorKind = "file-read-access"
	ErrorFileWriteAccess        ErrorKind = "file-write-access"
	ErrorUnauthorizedFileAccess ErrorKind = "unauthorized-file-access"
	ErrorBinaryOutput           ErrorKind = "binary-output"
	ErrorPossibleLoggedSecret   ErrorKind = "possible-logged-secret"
	ErrorUnsupportedSecretType  ErrorKind = "unsupported-secret-type"
	// ErrorStream: a WebSocket step that failed, or a sondeStream field
	// that does not apply to the entry's protocol.
	ErrorStream ErrorKind = "stream"
	// ErrorGRPC: a gRPC call that failed: a status other than OK that the
	// entry does not check, descriptors that could not be loaded, or a
	// message that does not match its type.
	ErrorGRPC ErrorKind = "grpc"
)

// errorKinds maps every runtime error kind to its public kind.
var errorKinds = [runerr.KindCount]ErrorKind{
	runerr.AssertFailure:                ErrorAssertFailure,
	runerr.ExpressionInvalidType:        ErrorExpressionInvalidType,
	runerr.FileReadAccess:               ErrorFileReadAccess,
	runerr.UnauthorizedFileAccess:       ErrorUnauthorizedFileAccess,
	runerr.FilterDecode:                 ErrorFilterDecode,
	runerr.FilterDateParsing:            ErrorFilterDateParsing,
	runerr.FilterInvalidEncoding:        ErrorFilterInvalidEncoding,
	runerr.FilterInvalidInputValue:      ErrorFilterInvalidInputValue,
	runerr.FilterInvalidInputType:       ErrorFilterInvalidInputType,
	runerr.FilterInvalidFormatSpecifier: ErrorFilterInvalidFormatSpecifier,
	runerr.FilterMissingInput:           ErrorFilterMissingInput,
	runerr.HTTP:                         ErrorHTTP,
	runerr.InvalidJSON:                  ErrorInvalidJSON,
	runerr.InvalidRegex:                 ErrorInvalidRegex,
	runerr.InvalidURL:                   ErrorInvalidURL,
	runerr.InvalidXPathEval:             ErrorInvalidXPathEval,
	runerr.NoQueryResult:                ErrorNoQueryResult,
	runerr.QueryHeaderNotFound:          ErrorQueryHeaderNotFound,
	runerr.QueryInvalidJSON:             ErrorQueryInvalidJSON,
	runerr.QueryInvalidJSONPath:         ErrorQueryInvalidJSONPath,
	runerr.QueryInvalidXML:              ErrorQueryInvalidXML,
	runerr.UndefinedVariable:            ErrorUndefinedVariable,
	runerr.Unrenderable:                 ErrorUnrenderable,
	runerr.AssertStatus:                 ErrorAssertStatus,
	runerr.AssertVersion:                ErrorAssertVersion,
	runerr.AssertHeaderValue:            ErrorAssertHeaderValue,
	runerr.AssertBodyValue:              ErrorAssertBodyValue,
	runerr.AssertBodyDiff:               ErrorAssertBodyDiff,
	runerr.InvalidOptionValue:           ErrorInvalidOptionValue,
	runerr.NoFilterResult:               ErrorNoFilterResult,
	runerr.PossibleLoggedSecret:         ErrorPossibleLoggedSecret,
	runerr.UnsupportedSecretType:        ErrorUnsupportedSecretType,
	runerr.FileWriteAccess:              ErrorFileWriteAccess,
	runerr.BinaryOutput:                 ErrorBinaryOutput,
	runerr.ContractViolation:            ErrorContractViolation,
	runerr.Stream:                       ErrorStream,
	runerr.GRPC:                         ErrorGRPC,
}

// Error is an error of a run: a parse error (kind ErrorParse) or an error
// of one entry. It carries the file it belongs to, so Render needs no
// arguments. Message texts are for people and may change in any version.
// Errors come from the engine: the zero Error holds nothing and every
// method returns its zero result.
type Error struct {
	// Exactly one of run and parse is set.
	run   *runerr.Error
	parse *syntax.Error
	// file and src locate the error for Render; entry is the request line
	// of its entry (0 for a parse error).
	file  string
	src   []byte
	entry int
}

// Error returns the error on one line: "<description>: <message>" (a
// parse error starts with "<line>:<col>: ").
func (e *Error) Error() string {
	if e.parse != nil {
		return e.parse.Error()
	}
	if e.run == nil {
		return ""
	}
	return e.run.Error()
}

// Kind classifies the error.
func (e *Error) Kind() ErrorKind {
	if e.parse != nil {
		return ErrorParse
	}
	if e.run == nil {
		return ""
	}
	return errorKinds[e.run.Kind]
}

// Assert reports whether the error is an assert failure (the sonde CLI
// exits with 4) rather than a runtime error (exit 3). It is false for a
// parse error.
func (e *Error) Assert() bool { return e.run != nil && e.run.Assert }

// Span locates the error in its file. A parse error is a position: Start
// and End are equal.
func (e *Error) Span() Span {
	if e.parse != nil {
		return Span{Start: Pos(e.parse.Pos), End: Pos(e.parse.Pos)}
	}
	if e.run == nil {
		return Span{}
	}
	return spanOf(e.run.Span)
}

// Description is the error's title, e.g. "Assert status code".
func (e *Error) Description() string {
	if e.parse != nil {
		return e.parse.Description()
	}
	if e.run == nil {
		return ""
	}
	return e.run.Description()
}

// Message is the explanation shown under the source line.
func (e *Error) Message() string {
	if e.parse != nil {
		return e.parse.Message()
	}
	if e.run == nil {
		return ""
	}
	return e.run.Message()
}

// Actual is the actual value of a failed assert or of a value with an
// unexpected type, as displayed; "" when the error has none.
func (e *Error) Actual() string {
	if e.run == nil {
		return ""
	}
	return e.run.Actual
}

// Expected is the expected value of a failed assert or type, as
// displayed; "" when the error has none.
func (e *Error) Expected() string {
	if e.run == nil {
		return ""
	}
	return e.run.Expected
}

// Render formats the error the way the sonde CLI prints it: the
// description, the location, the source lines involved and the message.
func (e *Error) Render() string {
	if e.parse != nil {
		return e.parse.Render(e.file, trimBOM(e.src))
	}
	if e.run == nil {
		return ""
	}
	return e.run.Render(e.file, string(e.src), e.entry)
}

// RenderColor is Render with ANSI colors.
func (e *Error) RenderColor() string {
	if e.parse != nil {
		return e.parse.RenderColor(e.file, trimBOM(e.src))
	}
	if e.run == nil {
		return ""
	}
	return e.run.RenderColor(e.file, string(e.src), e.entry)
}

// trimBOM drops a leading UTF-8 byte order mark, which the parser skips:
// positions are relative to the text after it.
func trimBOM(src []byte) []byte {
	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		return src[3:]
	}
	return src
}

// finish locates the errors of an entry attempt in its file, once the
// attempt is complete (its Line may move while it runs).
func (u *unit) finish(res *EntryResult) {
	for _, e := range res.Errors {
		e.file, e.src, e.entry = u.name, u.src, res.Line
	}
	for _, a := range res.Asserts {
		if a.Err != nil {
			a.Err.file, a.Err.src, a.Err.entry = u.name, u.src, res.Line
		}
	}
}
