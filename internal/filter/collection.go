// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"strconv"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

func (c call) count(v value.Value) (value.Value, error) {
	switch v := v.(type) {
	case value.List:
		return value.Int(len(v)), nil
	case value.Bytes:
		return value.Int(len(v)), nil
	case value.Nodeset:
		return value.Int(v), nil
	}
	return nil, c.typeError(v, "list, bytes or nodeset")
}

func (c call) firstOrLast(v value.Value, first bool) (value.Value, error) {
	list, ok := v.(value.List)
	if !ok {
		return nil, c.typeError(v, "list")
	}
	if len(list) == 0 {
		return nil, c.invalidValue("list is empty")
	}
	if first {
		return list[0], nil
	}
	return list[len(list)-1], nil
}

func (c call) nth(v value.Value) (value.Value, error) {
	n, err := c.index()
	if err != nil {
		return nil, err
	}
	list, ok := v.(value.List)
	if !ok {
		return nil, c.typeError(v, "list")
	}
	size := int64(len(list))
	switch {
	case n >= 0 && n < size:
		return list[n], nil
	case n < 0 && n >= -size:
		return list[size+n], nil
	}
	return nil, c.invalidValue("out of bound - size is " + strconv.FormatInt(size, 10))
}

// index evaluates the nth argument: an integer literal or a placeholder
// whose value must be an integer.
func (c call) index() (int64, error) {
	switch a := c.f.Arg.(type) {
	case *syntax.Number:
		return a.Int, nil
	case *syntax.Placeholder:
		v, err := c.env.Eval(a.Expr)
		if err != nil {
			return 0, err
		}
		if i, ok := v.(value.Int); ok {
			return int64(i), nil
		}
		e := runerr.New(a.Expr.Span, runerr.ExpressionInvalidType, false)
		e.Actual, e.Expected = value.Repr(v), "integer"
		return 0, e
	}
	return 0, c.invalidValue("invalid index")
}

func (c call) location(v value.Value) (value.Value, error) {
	r, ok := v.(value.HTTPResponse)
	if !ok {
		return nil, c.typeError(v, "http response")
	}
	if !r.HasLocation {
		return nil, nil
	}
	return value.String(r.Location), nil
}
