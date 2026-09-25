// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runerr

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

func span(line, col, endCol int) syntax.Span {
	return syntax.Span{Start: syntax.Pos{Line: line, Col: col}, End: syntax.Pos{Line: line, Col: endCol}}
}

func TestMessages(t *testing.T) {
	tests := []struct {
		e    *Error
		want string
	}{
		{&Error{Kind: AssertFailure, Actual: "integer <1>", Expected: "integer <2>"}, "Assert failure:    actual:   integer <1>\n   expected: integer <2>"},
		{&Error{Kind: AssertFailure, Actual: "a", Expected: "b", TypeMismatch: true}, "Assert failure:    actual:   a\n   expected: b\n   >>> types between actual and expected are not consistent"},
		{&Error{Kind: ExpressionInvalidType, Actual: "float <1.0>", Expected: "integer"}, "Invalid expression type: expecting integer, actual value is float <1.0>"},
		{&Error{Kind: FileReadAccess, Value: "a.bin"}, "File read access: file a.bin can not be read"},
		{&Error{Kind: UnauthorizedFileAccess, Value: "../a"}, "Unauthorized file access: unauthorized access to file ../a, check --file-root option"},
		{&Error{Kind: FilterDecode, Value: "utf-8"}, "Filter error: value can not be decoded with <utf-8> encoding"},
		{&Error{Kind: FilterDateParsing, Value: "x", Reason: "%Y"}, "Filter error: value <x> could not be parsed with <%Y> format"},
		{&Error{Kind: FilterInvalidEncoding, Value: "x"}, "Filter error: <x> encoding is not supported"},
		{&Error{Kind: FilterInvalidInputValue, Reason: "list is empty"}, "Filter error: invalid filter input: list is empty"},
		{&Error{Kind: FilterInvalidInputType, Actual: "string", Expected: "list"}, "Filter error: invalid filter input type\n   actual:   string\n   expected: list"},
		{&Error{Kind: FilterInvalidFormatSpecifier, Value: "%Q"}, "Filter error: date format <%Q> is not supported"},
		{&Error{Kind: FilterMissingInput}, "Filter error: missing value to apply filter"},
		{&Error{Kind: HTTP, Value: "Invalid charset", Reason: "the charset 'x' is not valid"}, "Invalid charset: the charset 'x' is not valid"},
		{&Error{Kind: InvalidJSON, Value: "abc"}, "Invalid JSON: actual value is <abc>"},
		{&Error{Kind: InvalidRegex}, "Invalid regex: regex expression is not valid"},
		{&Error{Kind: InvalidURL, Value: "x", Reason: "empty host"}, "Invalid URL: invalid URL <x> (empty host)"},
		{&Error{Kind: InvalidXPathEval}, "Invalid XPath expression: XPath expression is not valid"},
		{&Error{Kind: NoQueryResult}, "No query result: query didn't return any result"},
		{&Error{Kind: QueryHeaderNotFound}, "Header not found: this header has not been found in the response"},
		{&Error{Kind: QueryInvalidJSON}, "Invalid JSON: HTTP response is not a valid JSON"},
		{&Error{Kind: QueryInvalidJSONPath, Value: "$["}, "Invalid JSONPath: JSONPath expression '$[' is not valid"},
		{&Error{Kind: QueryInvalidXML}, "Invalid XML: HTTP response is not a valid XML"},
		{&Error{Kind: UndefinedVariable, Value: "x"}, "Undefined variable: you must set the variable x"},
		{&Error{Kind: Unrenderable, Value: "[1]"}, "Unrenderable expression: expression with value [1] can not be rendered"},
		{&Error{Kind: Kind(999), Reason: "r"}, "Runtime error: r"},
	}
	for _, tt := range tests {
		if got := tt.e.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
	if e := New(span(1, 2, 3), NoQueryResult, true); !e.Assert || e.Span.Start.Col != 2 {
		t.Errorf("New = %#v", e)
	}
}

const src = "GET http://a\nHTTP 200\n[Asserts]\njsonpath \"$.b\" count == 1\n\tjsonpath \"$.a\"\ttoInt == 2\n"

func TestRender(t *testing.T) {
	tests := []struct {
		name  string
		e     *Error
		entry int
		want  string
	}{
		{"assert failure", &Error{Span: span(4, 0, 0), Kind: AssertFailure, Actual: "none", Expected: "integer <1>"}, 1, `Assert failure
  --> t.hurl:4:0
   |
   | GET http://a
   | ...
 4 | jsonpath "$.b" count == 1
   |   actual:   none
   |   expected: integer <1>
   |`},
		{"carets", &Error{Span: span(4, 16, 21), Kind: FilterMissingInput}, 1, `Filter error
  --> t.hurl:4:16
   |
   | GET http://a
   | ...
 4 | jsonpath "$.b" count == 1
   |                ^^^^^ missing value to apply filter
   |`},
		{"multiline message and tabs", &Error{Span: span(5, 17, 22), Kind: FilterInvalidInputType, Actual: "string", Expected: "list"}, 4,
			"Filter error\n  --> t.hurl:5:17\n   |\n   | jsonpath \"$.b\" count == 1\n 5 |     jsonpath \"$.a\"    toInt == 2\n" +
				"   |                       ^^^^^ invalid filter input type\n   |                                actual:   string\n" +
				"   |                                expected: list\n   |"},
		{"no entry line", &Error{Span: span(1, 5, 5), Kind: UndefinedVariable, Value: "x"}, 0, `Undefined variable
  --> t.hurl:1:5
   |
 1 | GET http://a
   |     ^ you must set the variable x
   |`},
		{"entry on error line", &Error{Span: span(1, 5, 13), Kind: InvalidURL, Value: "x", Reason: "y"}, 1, `Invalid URL
  --> t.hurl:1:5
   |
 1 | GET http://a
   |     ^^^^^^^^ invalid URL <x> (y)
   |`},
		{"line out of range", &Error{Span: span(9, 1, 1), Kind: InvalidRegex}, 0,
			"Invalid regex\n  --> t.hurl:9:1\n   |\n 9 | \n   | ^ regex expression is not valid\n   |"},
	}
	for _, tt := range tests {
		if got := tt.e.Render("t.hurl", src, tt.entry); got != tt.want {
			t.Errorf("%s:\n%s\nwant:\n%s", tt.name, got, tt.want)
		}
	}
	long := strings.Repeat("\n", 120) + "x\n"
	got := (&Error{Span: span(121, 1, 2), Kind: InvalidRegex}).Render("t.hurl", long, 0)
	if !strings.HasPrefix(got, "Invalid regex\n   --> t.hurl:121:1\n    |\n121 | x\n") {
		t.Errorf("wide gutter:\n%s", got)
	}
}
