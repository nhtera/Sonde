// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package filter applies filters (`count`, `jsonpath`, `toInt`, …) to the
// value of a query.
package filter

import (
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
)

// Options change how filters evaluate.
type Options struct {
	// InAssert marks errors raised while evaluating an assert.
	InAssert bool
	// NoJSONPathCoercion keeps the result of a jsonpath filter a list of
	// matches, even for no match or a single one.
	NoJSONPathCoercion bool
}

// Apply applies filters in order to v. A nil result means the last filter
// produced no value; a filter that receives no value fails. inAssert marks
// errors raised while evaluating an assert.
func Apply(items []*syntax.FilterItem, v value.Value, env *template.Env, inAssert bool) (value.Value, error) {
	return ApplyOptions(items, v, env, Options{InAssert: inAssert})
}

// ApplyOptions is Apply with every option.
func ApplyOptions(items []*syntax.FilterItem, v value.Value, env *template.Env, opts Options) (value.Value, error) {
	for _, it := range items {
		if v == nil {
			return nil, runerr.New(it.Filter.Span, runerr.FilterMissingInput, opts.InAssert)
		}
		var err error
		if v, err = EvalOptions(it.Filter, v, env, opts); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// Eval applies one filter to v (non-nil).
func Eval(f *syntax.Filter, v value.Value, env *template.Env, inAssert bool) (value.Value, error) {
	return EvalOptions(f, v, env, Options{InAssert: inAssert})
}

// EvalOptions is Eval with every option.
func EvalOptions(f *syntax.Filter, v value.Value, env *template.Env, opts Options) (value.Value, error) {
	c := call{f: f, env: env, assert: opts.InAssert, noCoercion: opts.NoJSONPathCoercion}
	switch f.Kind {
	case syntax.FilterBase64Decode:
		return c.base64Decode(v)
	case syntax.FilterBase64Encode:
		return c.base64Encode(v)
	case syntax.FilterBase64URLSafeDecode:
		return c.base64URLSafeDecode(v)
	case syntax.FilterBase64URLSafeEncode:
		return c.base64URLSafeEncode(v)
	case syntax.FilterCharsetDecode, syntax.FilterDecode:
		return c.charsetDecode(v)
	case syntax.FilterCount:
		return c.count(v)
	case syntax.FilterDaysAfterNow:
		return c.daysFromNow(v, true)
	case syntax.FilterDaysBeforeNow:
		return c.daysFromNow(v, false)
	case syntax.FilterFirst:
		return c.firstOrLast(v, true)
	case syntax.FilterLast:
		return c.firstOrLast(v, false)
	case syntax.FilterFormat, syntax.FilterDateFormat:
		return c.dateFormat(v)
	case syntax.FilterHTMLEscape:
		return c.htmlEscape(v)
	case syntax.FilterHTMLUnescape:
		return c.htmlUnescape(v)
	case syntax.FilterJSONPath:
		return c.jsonPath(v)
	case syntax.FilterLocation:
		return c.location(v)
	case syntax.FilterNth:
		return c.nth(v)
	case syntax.FilterRegex:
		return c.regex(v)
	case syntax.FilterReplace:
		return c.replace(v)
	case syntax.FilterReplaceRegex:
		return c.replaceRegex(v)
	case syntax.FilterSplit:
		return c.split(v)
	case syntax.FilterToDate:
		return c.toDate(v)
	case syntax.FilterToFloat:
		return c.toFloat(v)
	case syntax.FilterToHex:
		return c.toHex(v)
	case syntax.FilterToInt:
		return c.toInt(v)
	case syntax.FilterToString:
		return c.toString(v)
	case syntax.FilterURLDecode:
		return c.urlDecode(v)
	case syntax.FilterURLEncode:
		return c.urlEncode(v)
	case syntax.FilterURLQueryParam:
		return c.urlQueryParam(v)
	case syntax.FilterUTF8Decode:
		return c.utf8Decode(v)
	case syntax.FilterUTF8Encode:
		return c.utf8Encode(v)
	case syntax.FilterXPath:
		return c.xpath(v)
	}
	return nil, c.invalidValue("unknown filter " + f.Kind.String())
}

// call is one filter application.
type call struct {
	f          *syntax.Filter
	env        *template.Env
	assert     bool
	noCoercion bool
}

// typeError reports an input of the wrong kind.
func (c call) typeError(v value.Value, expected string) error {
	err := runerr.New(c.f.Span, runerr.FilterInvalidInputType, c.assert)
	err.Actual, err.Expected = v.Kind().String(), expected
	return err
}

// invalidValue reports an input of the right kind with an invalid value.
func (c call) invalidValue(reason string) error {
	err := runerr.New(c.f.Span, runerr.FilterInvalidInputValue, c.assert)
	err.Reason = reason
	return err
}

// arg renders the first template argument.
func (c call) arg() (string, error) { return c.env.Render(c.f.Arg.(*syntax.Template)) }

// arg2 renders the second template argument.
func (c call) arg2() (string, error) { return c.env.Render(c.f.Arg2.(*syntax.Template)) }
