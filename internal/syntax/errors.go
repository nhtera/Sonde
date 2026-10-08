// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/styled"
)

// ErrorKind classifies a parse error.
type ErrorKind int

// Parse error kinds. Kinds with a parameter carry it in Error.Arg.
const (
	ErrDuplicateSection ErrorKind = iota
	ErrEscapeChar
	ErrExpecting // Arg: expected literal
	ErrFileContentType
	ErrFilename
	ErrGraphQLVariables
	ErrHexDigit
	ErrInvalidCookieAttribute
	ErrInvalidDurationUnit // Arg: unit
	ErrInvalidOption       // Arg: option name
	ErrJSONTrailingComma
	ErrJSONExpectingElement
	ErrJSONEmptyElement
	ErrJSONPathExpr
	ErrMethod // Arg: method name
	ErrMultiline
	ErrMultilineLanguageHint // Arg: hint
	ErrOddNumberOfHexDigits
	ErrPredicate
	ErrPredicateValue
	ErrRegexExpr // Arg: regex error
	ErrRequestSection
	ErrRequestSectionName // Arg: section name
	ErrResponseSection
	ErrResponseSectionName // Arg: section name
	ErrSpace
	ErrStatus
	ErrTemplateVariable
	ErrUnicode
	ErrURLIllegalCharacter // Arg: character
	ErrURLInvalidStart
	ErrVariable // Arg: message
	ErrVersion
	ErrXPathExpr
	ErrXML
	// Input guards.
	ErrInvalidUTF8
	ErrFileTooLarge
	ErrNestingTooDeep
	// Sonde extensions.
	ErrSondeOnly   // Arg: the construct, e.g. "section `[SondeMessages]`"
	ErrMessageStep // Arg: step name
	ErrStreamField // Arg: field name
	ErrMessageValue
	ErrGrpcField // Arg: field name
	ErrGrpcKey   // Arg: key
)

// Error is a parse error at a single position; parsing stops at the first one.
type Error struct {
	Pos  Pos
	Kind ErrorKind
	Arg  string

	// recoverable errors let combinators backtrack and try an alternative.
	recoverable bool
	// sonde is set when the file is a .sonde one: "did you mean" may then
	// suggest a Sonde-only name.
	sonde bool
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d:%d: %s: %s", e.Pos.Line, e.Pos.Col, e.Description(), e.Message())
}

// Description is the short title, e.g. "Parsing literal".
func (e *Error) Description() string {
	switch e.Kind {
	case ErrDuplicateSection, ErrRequestSection, ErrResponseSection:
		return "Parsing section"
	case ErrEscapeChar:
		return "Parsing escape character"
	case ErrExpecting:
		return "Parsing literal"
	case ErrFileContentType:
		return "Parsing file content type"
	case ErrFilename:
		return "Parsing filename"
	case ErrGraphQLVariables:
		return "Parsing GraphQL variables"
	case ErrHexDigit:
		return "Parsing hexadecimal number"
	case ErrInvalidCookieAttribute:
		return "Parsing cookie attribute"
	case ErrInvalidOption:
		return "Parsing option"
	case ErrInvalidDurationUnit:
		return "Parsing duration"
	case ErrJSONTrailingComma, ErrJSONExpectingElement, ErrJSONEmptyElement, ErrNestingTooDeep:
		return "Parsing JSON"
	case ErrJSONPathExpr:
		return "Parsing JSONPath expression"
	case ErrMethod:
		return "Parsing method"
	case ErrMultiline, ErrMultilineLanguageHint:
		return "Parsing multiline"
	case ErrOddNumberOfHexDigits:
		return "Parsing hex bytearray"
	case ErrPredicate:
		return "Parsing predicate"
	case ErrPredicateValue:
		return "Parsing predicate value"
	case ErrRegexExpr:
		return "Parsing regex"
	case ErrRequestSectionName:
		return "Parsing request section name"
	case ErrResponseSectionName:
		return "Parsing response section name"
	case ErrSpace:
		return "Parsing space"
	case ErrStatus:
		return "Parsing status code"
	case ErrTemplateVariable:
		return "Parsing template variable"
	case ErrUnicode:
		return "Parsing unicode literal"
	case ErrURLIllegalCharacter, ErrURLInvalidStart:
		return "Parsing URL"
	case ErrVariable:
		return "Parsing variable"
	case ErrVersion:
		return "Parsing version"
	case ErrXPathExpr:
		return "Parsing XPath expression"
	case ErrXML:
		return "Parsing XML"
	case ErrInvalidUTF8, ErrFileTooLarge:
		return "Reading file"
	case ErrSondeOnly:
		return "Parsing Sonde extension"
	case ErrMessageStep, ErrMessageValue:
		return "Parsing message step"
	case ErrStreamField:
		return "Parsing sondeStream field"
	case ErrGrpcField:
		return "Parsing sondeGrpc field"
	case ErrGrpcKey:
		return "Parsing SondeGrpc section"
	}
	return "Parsing"
}

var (
	validOptions = []string{
		"aws-sigv4", "cacert", "cert", "compressed", "connect-timeout", "connect-to", "delay",
		"digest", "header", "http1.0", "http1.1", "http2", "http2-prior-knowledge", "http3", "insecure",
		"ipv4", "ipv6",
		"key", "limit-rate", "location", "location-trusted", "max-redirs", "max-time",
		"negotiate", "netrc", "netrc-file", "netrc-optional", "no-proxy", "ntlm", "output",
		"path-as-is", "pinnedpubkey", "proxy", "repeat", "resolve", "retry", "retry-interval",
		"skip", "unix-socket", "user", "variable", "variables-file", "verbose", "verbosity",
		"very-verbose",
	}
	validMethods          = []string{"GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH"}
	validRequestSections  = []string{"Query", "Form", "Multipart", "Cookies", "Options"}
	validResponseSections = []string{"Captures", "Asserts"}
	validDurationUnits    = []string{"ms", "s"}

	// Sonde-only names, suggested only in .sonde files.
	sondeOptions         = []string{"sonde-stream-count", "sonde-stream-max-bytes", "sonde-stream-timeout"}
	sondeRequestSections = []string{"SondeMessages", "SondeGrpc"}
	validMessageSteps    = []string{"send", "receive", "close"}
	validStreamFields    = []string{"data", "event", "id", "retry", "type"}
	validGrpcFields      = []string{"status", "code", "message"}
	validGrpcKeys        = []string{"proto", "import-path", "protoset"}
)

// suggestions returns valid, plus the Sonde-only names in a .sonde file.
func (e *Error) suggestions(valid, sonde []string) []string {
	if !e.sonde {
		return valid
	}
	return append(append([]string(nil), valid...), sonde...)
}

// Message is the detail shown next to the caret, e.g. "expecting ':'".
func (e *Error) Message() string {
	switch e.Kind {
	case ErrDuplicateSection:
		return "the section is already defined"
	case ErrEscapeChar:
		return "the escaping sequence is not valid"
	case ErrExpecting:
		return "expecting '" + e.Arg + "'"
	case ErrFileContentType:
		return "expecting a content type"
	case ErrFilename:
		return "expecting a filename"
	case ErrGraphQLVariables:
		return "GraphQL variables is not a valid JSON object"
	case ErrHexDigit:
		return "expecting a valid hexadecimal number"
	case ErrInvalidCookieAttribute:
		return "the cookie attribute is not valid"
	case ErrInvalidDurationUnit:
		return "the duration unit is not valid. " + didYouMean(validDurationUnits, e.Arg, "Valid values are "+strings.Join(validDurationUnits, ", "))
	case ErrInvalidOption:
		valid := e.suggestions(validOptions, sondeOptions)
		return "the option name is not valid. " + didYouMean(valid, e.Arg, "Valid values are "+strings.Join(valid, ", "))
	case ErrJSONTrailingComma:
		return "trailing comma is not allowed"
	case ErrJSONEmptyElement:
		return "expecting an element; found empty element instead"
	case ErrJSONExpectingElement:
		return "expecting a boolean, number, string, array, object or null"
	case ErrJSONPathExpr:
		return "expecting a JSONPath expression"
	case ErrMethod:
		return "the HTTP method <" + e.Arg + "> is not valid. " + didYouMean(validMethods, e.Arg, "Valid values are "+strings.Join(validMethods, ", "))
	case ErrMultiline:
		return "the multiline is not valid"
	case ErrMultilineLanguageHint:
		return "Invalid language hint " + e.Arg
	case ErrOddNumberOfHexDigits:
		return "expecting an even number of hex digits"
	case ErrPredicate:
		return "expecting a predicate"
	case ErrPredicateValue:
		return "invalid predicate value"
	case ErrRegexExpr:
		return "invalid Regex expression: " + e.Arg
	case ErrRequestSection:
		return "this is not a valid section for a request"
	case ErrRequestSectionName:
		valid := e.suggestions(validRequestSections, sondeRequestSections)
		return "the section is not valid. " + didYouMean(valid, e.Arg, "Valid values are "+strings.Join(valid, ", "))
	case ErrResponseSection:
		return "this is not a valid section for a response"
	case ErrResponseSectionName:
		return "the section is not valid. " + didYouMean(validResponseSections, e.Arg, "Valid values are Captures or Asserts")
	case ErrSpace:
		return "expecting a space"
	case ErrStatus:
		return "HTTP status code is not valid"
	case ErrTemplateVariable:
		return "expecting a variable"
	case ErrUnicode:
		return "Invalid unicode literal"
	case ErrURLIllegalCharacter:
		return "illegal character <" + e.Arg + ">"
	case ErrURLInvalidStart:
		return "expecting http://, https:// or {{"
	case ErrVariable:
		return e.Arg
	case ErrVersion:
		return "HTTP version must be HTTP, HTTP/1.0, HTTP/1.1, HTTP/2 or HTTP/3"
	case ErrXPathExpr:
		return "expecting a XPath expression"
	case ErrXML:
		return "invalid XML"
	case ErrInvalidUTF8:
		return "the file is not valid UTF-8"
	case ErrFileTooLarge:
		return "the file is larger than " + e.Arg
	case ErrNestingTooDeep:
		return "maximum nesting depth of " + e.Arg + " exceeded"
	case ErrSondeOnly:
		return e.Arg + " requires a .sonde file"
	case ErrMessageStep:
		return "the step is not valid. " + didYouMean(validMessageSteps, e.Arg, "Valid values are send, receive, close")
	case ErrMessageValue:
		return "expecting a message: JSON, a `text` string, a ``` multiline string, XML, base64, hex or file"
	case ErrStreamField:
		return "the field is not valid. " + didYouMean(validStreamFields, e.Arg, "Valid values are "+strings.Join(validStreamFields, ", "))
	case ErrGrpcField:
		return "the field is not valid. " + didYouMean(validGrpcFields, e.Arg, "Valid values are "+strings.Join(validGrpcFields, ", "))
	case ErrGrpcKey:
		return "the key is not valid. " + didYouMean(validGrpcKeys, e.Arg, "Valid values are "+strings.Join(validGrpcKeys, ", "))
	}
	return "invalid input"
}

// Render formats the error as a source snippet with a caret (the CLI adds
// the "error: " prefix):
//
//	Parsing literal
//	  --> file.hurl:2:14
//	   |
//	 2 | base64, aaaaa?
//	   |              ^ expecting ';'
//	   |
func (e *Error) Render(filename string, src []byte) string {
	return e.render(filename, src).String(false)
}

// RenderColor is Render with ANSI colors: a blue gutter and the message
// in red.
func (e *Error) RenderColor(filename string, src []byte) string {
	return e.render(filename, src).String(true)
}

func (e *Error) render(filename string, src []byte) styled.Text {
	lines := SourceLines(string(src))
	width := max(len(strconv.Itoa(len(lines))), 2)
	spaces := strings.Repeat(" ", width)
	gutter := styled.Bold | styled.Blue

	line := ""
	if e.Pos.Line-1 < len(lines) && e.Pos.Line >= 1 {
		line = lines[e.Pos.Line-1]
	}

	var t styled.Text
	t.Push(e.Description(), styled.Bold)
	t.Push("\n"+spaces, styled.Plain)
	t.Push("-->", gutter)
	t.Push(fmt.Sprintf(" %s:%d:%d\n", filename, e.Pos.Line, e.Pos.Col), styled.Plain)
	t.Push(spaces+" |", gutter)
	t.Push("\n", styled.Plain)
	t.Push(fmt.Sprintf("%*d |", width, e.Pos.Line), gutter)
	t.Push(" "+strings.ReplaceAll(line, "\t", "    ")+"\n", styled.Plain)
	t.Push(spaces+" |", gutter)
	t.Push(carets(line, e.Pos.Col)+e.Message(), styled.Bold|styled.Red)
	t.Push("\n", styled.Plain)
	t.Push(spaces+" |", gutter)
	return t
}

// carets indents a single caret under column col; each tab before the column
// is displayed as four spaces.
func carets(line string, col int) string {
	tabs := 0
	i := 0
	for _, c := range line {
		if i >= col-1 {
			break
		}
		if c == '\t' {
			tabs++
		}
		i++
	}
	return strings.Repeat(" ", col+tabs*3) + "^ "
}

// SourceLines splits source text into lines: "\n" separators, a trailing
// "\r" removed, no final empty line.
func SourceLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

func didYouMean(valid []string, actual, fallback string) string {
	for _, v := range valid {
		if levenshtein(strings.ToLower(v), strings.ToLower(actual)) < 2 {
			return "Did you mean " + v + "?"
		}
	}
	return fallback
}

func levenshtein(a, b string) int {
	s1, s2 := []rune(a), []rune(b)
	column := make([]int, len(s1)+1)
	for i := range column {
		column[i] = i
	}
	for x := 1; x <= len(s2); x++ {
		column[0] = x
		lastDiag := x - 1
		for y := 1; y <= len(s1); y++ {
			oldDiag := column[y]
			cost := 1
			if s1[y-1] == s2[x-1] {
				cost = 0
			}
			column[y] = min(column[y]+1, column[y-1]+1, lastDiag+cost)
			lastDiag = oldDiag
		}
	}
	return column[len(s1)]
}
