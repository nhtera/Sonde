// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"time"

	"github.com/nhtera/sonde/internal/value"
)

// ValueKind is the type of a Value. New kinds may be added in minor
// versions: switch on it with a default case.
type ValueKind string

// Value kinds.
const (
	ValueNull    ValueKind = "null"
	ValueBool    ValueKind = "boolean"
	ValueInteger ValueKind = "integer" // including integers beyond 64 bits
	ValueFloat   ValueKind = "float"
	ValueString  ValueKind = "string"
	ValueBytes   ValueKind = "bytes"
	ValueList    ValueKind = "list"
	ValueObject  ValueKind = "object"
	ValueDate    ValueKind = "date"
	// ValueNodeset is an XPath result; only its size is kept.
	ValueNodeset ValueKind = "nodeset"
	// ValueUnit is the result of a query that matched without data (e.g.
	// a cookie attribute flag).
	ValueUnit ValueKind = "unit"
	// ValueRegex is a regular expression; no capture produces one today.
	ValueRegex ValueKind = "regex"
	// ValueRedirect is one hop of a `redirects` query.
	ValueRedirect ValueKind = "redirect"
)

var valueKinds = [value.KindCount]ValueKind{
	value.KindNull:         ValueNull,
	value.KindBool:         ValueBool,
	value.KindInteger:      ValueInteger,
	value.KindFloat:        ValueFloat,
	value.KindString:       ValueString,
	value.KindBytes:        ValueBytes,
	value.KindList:         ValueList,
	value.KindObject:       ValueObject,
	value.KindDate:         ValueDate,
	value.KindNodeset:      ValueNodeset,
	value.KindUnit:         ValueUnit,
	value.KindRegex:        ValueRegex,
	value.KindHTTPResponse: ValueRedirect,
}

// Value is a typed value, such as a captured one. The zero Value has no
// kind. Values are also accepted as variables (Options.Variables). Values
// are not comparable with ==: compare their kinds and contents.
type Value struct {
	_ [0]func() // not comparable: a list or bytes value would panic
	v value.Value
}

// Field is a member of an object Value.
type Field struct {
	Key   string
	Value Value
}

// Redirect is one hop of a `redirects` query: the status of the response
// and its Location header, if any.
type Redirect struct {
	Status      int
	Location    string
	HasLocation bool
}

// Kind returns the type of v; "" for the zero Value.
func (v Value) Kind() ValueKind {
	if v.v == nil {
		return ""
	}
	return valueKinds[v.v.Kind()]
}

// String returns v as displayed in messages: a string's own text, an
// integer's digits, bytes in hexadecimal, a date like
// "2006-01-02 15:04:05.123 UTC" (fraction only when non-zero); "none"
// for the zero Value.
func (v Value) String() string { return value.Display(v.v) }

// Bool returns a boolean value.
func (v Value) Bool() (b, ok bool) {
	x, ok := v.v.(value.Bool)
	return bool(x), ok
}

// Int returns an integer value. It is false for an integer beyond 64 bits,
// whose digits String returns.
func (v Value) Int() (int64, bool) {
	x, ok := v.v.(value.Int)
	return int64(x), ok
}

// Float returns a float value (not an integer one).
func (v Value) Float() (float64, bool) {
	x, ok := v.v.(value.Float)
	return float64(x), ok
}

// Text returns a string value.
func (v Value) Text() (string, bool) {
	x, ok := v.v.(value.String)
	return string(x), ok
}

// Bytes returns a copy of a bytes value.
func (v Value) Bytes() ([]byte, bool) {
	x, ok := v.v.(value.Bytes)
	if !ok {
		return nil, false
	}
	return append([]byte{}, x...), true
}

// Time returns a date value, in UTC.
func (v Value) Time() (time.Time, bool) {
	x, ok := v.v.(value.Date)
	return x.UTC(), ok
}

// Regex returns the pattern of a regex value.
func (v Value) Regex() (string, bool) {
	x, ok := v.v.(value.Regex)
	return x.Source, ok
}

// Redirect returns a redirect value.
func (v Value) Redirect() (Redirect, bool) {
	x, ok := v.v.(value.HTTPResponse)
	return Redirect{Status: x.Status, Location: x.Location, HasLocation: x.HasLocation}, ok
}

// List returns the elements of a list value; nil for other kinds.
func (v Value) List() []Value {
	l, ok := v.v.(value.List)
	if !ok {
		return nil
	}
	out := make([]Value, len(l))
	for i, e := range l {
		out[i] = Value{v: e}
	}
	return out
}

// Fields returns the members of an object value in their order; nil for
// other kinds.
func (v Value) Fields() []Field {
	o, ok := v.v.(value.Object)
	if !ok {
		return nil
	}
	out := make([]Field, len(o))
	for i, m := range o {
		out[i] = Field{Key: m.Key, Value: Value{v: m.Value}}
	}
	return out
}

// Get returns the member key of an object value.
func (v Value) Get(key string) (Value, bool) {
	o, ok := v.v.(value.Object)
	if !ok {
		return Value{}, false
	}
	m, ok := o.Get(key)
	return Value{v: m}, ok
}

// Len returns the number of elements of a list, members of an object or
// nodes of a nodeset; 0 for other kinds.
func (v Value) Len() int {
	switch x := v.v.(type) {
	case value.List:
		return len(x)
	case value.Object:
		return len(x)
	case value.Nodeset:
		return int(x)
	}
	return 0
}
