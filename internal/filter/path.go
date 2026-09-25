// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"github.com/nhtera/sonde/internal/jsonpath"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
	"github.com/nhtera/sonde/internal/xpath"
)

func (c call) jsonPath(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	doc, err := value.DecodeJSON(string(s))
	if err != nil {
		e := runerr.New(c.f.Span, runerr.FilterInvalidInputValue, false)
		e.Reason = "value is not a valid JSON"
		return nil, e
	}
	return EvalJSONPath(doc, c.f.Arg.(*syntax.Template), c.env)
}

// EvalJSONPath evaluates a JSONPath template on a document. No match gives
// nil, one match its value, several matches a list.
func EvalJSONPath(doc value.Value, expr *syntax.Template, env *template.Env) (value.Value, error) {
	src, err := env.Render(expr)
	if err != nil {
		return nil, err
	}
	q, err := jsonpath.Parse(src)
	if err != nil {
		e := runerr.New(expr.Span, runerr.QueryInvalidJSONPath, false)
		e.Value = src
		return nil, e
	}
	v, _ := jsonpath.Unwrap(q.Eval(doc))
	return v, nil
}

func (c call) xpath(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	// The lenient HTML parser also reads XML.
	doc, err := xpath.Parse(string(s), xpath.HTML)
	if err != nil {
		e := runerr.New(c.f.Span, runerr.FilterInvalidInputValue, false)
		e.Reason = "value is not a valid XML"
		return nil, e
	}
	return EvalXPath(doc, c.f.Arg.(*syntax.Template), c.env)
}

// EvalXPath evaluates an XPath template on a document.
func EvalXPath(doc *xpath.Document, expr *syntax.Template, env *template.Env) (value.Value, error) {
	src, err := env.Render(expr)
	if err != nil {
		return nil, err
	}
	v, err := doc.Eval(src)
	if err != nil {
		return nil, runerr.New(expr.Span, runerr.InvalidXPathEval, false)
	}
	return v, nil
}
