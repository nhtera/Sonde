// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package value is the typed value model of evaluation: what queries return,
// filters transform, predicates compare and templates render.
//
// A Value is one of Null, Bool, Int, BigInt, Float, String, Bytes, List,
// Object, Date, Nodeset, Unit, Regex or HTTPResponse. Numbers compare across
// representations (Int 1 equals Float 1.0). Objects keep the member order they
// were built with; DecodeJSON sorts members by key.
//
// Values have three textual forms: Display (plain text, used inside lists and
// messages), Repr (`kind <display>`, the actual side of an assert failure) and
// Expected (the expected side). Render is the form a template placeholder
// produces; only scalar values are renderable.
package value

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/regex"
)

// Value is a typed evaluation value. The set of implementations is closed.
type Value interface {
	Kind() Kind
	isValue()
}

// Kind is the type of a value as shown in messages.
type Kind int

// Value kinds. BigInt values have KindInteger.
const (
	KindNull Kind = iota
	KindBool
	KindInteger
	KindFloat
	KindString
	KindBytes
	KindList
	KindObject
	KindDate
	KindNodeset
	KindUnit
	KindRegex
	KindHTTPResponse
)

var kindNames = [...]string{
	KindNull:         "null",
	KindBool:         "boolean",
	KindInteger:      "integer",
	KindFloat:        "float",
	KindString:       "string",
	KindBytes:        "bytes",
	KindList:         "list",
	KindObject:       "object",
	KindDate:         "date",
	KindNodeset:      "nodeset",
	KindUnit:         "unit",
	KindRegex:        "regex",
	KindHTTPResponse: "http response",
}

func (k Kind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return "Kind(" + strconv.Itoa(int(k)) + ")"
	}
	return kindNames[k]
}

type (
	// Null is the JSON null.
	Null struct{}
	// Bool is a boolean.
	Bool bool
	// Int is an integer that fits in 64 bits.
	Int int64
	// BigInt is a number too large for Int or Float, kept as its decimal
	// text (for example a 30-digit JSON integer).
	BigInt string
	// Float is a 64-bit floating point number.
	Float float64
	// String is a UTF-8 string.
	String string
	// Bytes is a byte sequence.
	Bytes []byte
	// List is an ordered list of values.
	List []Value
	// Object is a list of members.
	Object []Member
	// Date is an instant; it is always handled in UTC.
	Date time.Time
	// Nodeset is the result of an XPath query: only its size is kept.
	Nodeset int
	// Unit is the value of a query that matched without producing data,
	// such as a cookie attribute flag.
	Unit struct{}
	// Regex is a compiled regular expression with its source.
	Regex struct {
		Source string
		Re     *regexp.Regexp
	}
	// HTTPResponse is one hop of a redirect chain.
	HTTPResponse struct {
		// Location is the absolute URL of the next hop, "" when HasLocation is false.
		Location    string
		HasLocation bool
		Status      int
	}
)

// Member is an object member.
type Member struct {
	Key   string
	Value Value
}

func (Null) Kind() Kind         { return KindNull }
func (Bool) Kind() Kind         { return KindBool }
func (Int) Kind() Kind          { return KindInteger }
func (BigInt) Kind() Kind       { return KindInteger }
func (Float) Kind() Kind        { return KindFloat }
func (String) Kind() Kind       { return KindString }
func (Bytes) Kind() Kind        { return KindBytes }
func (List) Kind() Kind         { return KindList }
func (Object) Kind() Kind       { return KindObject }
func (Date) Kind() Kind         { return KindDate }
func (Nodeset) Kind() Kind      { return KindNodeset }
func (Unit) Kind() Kind         { return KindUnit }
func (Regex) Kind() Kind        { return KindRegex }
func (HTTPResponse) Kind() Kind { return KindHTTPResponse }

func (Null) isValue()         {}
func (Bool) isValue()         {}
func (Int) isValue()          {}
func (BigInt) isValue()       {}
func (Float) isValue()        {}
func (String) isValue()       {}
func (Bytes) isValue()        {}
func (List) isValue()         {}
func (Object) isValue()       {}
func (Date) isValue()         {}
func (Nodeset) isValue()      {}
func (Unit) isValue()         {}
func (Regex) isValue()        {}
func (HTTPResponse) isValue() {}

// Get returns the value of the first member named key.
func (o Object) Get(key string) (Value, bool) {
	for _, m := range o {
		if m.Key == key {
			return m.Value, true
		}
	}
	return nil, false
}

// UTC returns the date as a time.Time in UTC.
func (d Date) UTC() time.Time { return time.Time(d).UTC() }

// NewRegex compiles a pattern with Unicode-aware character classes (see
// TranslateRegex).
func NewRegex(pattern string) (Regex, error) {
	if msg := regex.Check(pattern); msg != "" {
		return Regex{}, errors.New(msg)
	}
	re, err := regexp.Compile(TranslateRegex(pattern))
	if err != nil {
		return Regex{}, err
	}
	return Regex{Source: pattern, Re: re}, nil
}

// Display returns the plain text form of v.
func Display(v Value) string {
	var b strings.Builder
	writeDisplay(&b, v)
	return b.String()
}

func writeDisplay(b *strings.Builder, v Value) {
	switch v := v.(type) {
	case nil:
		b.WriteString("none")
	case Null:
		b.WriteString("null")
	case Bool:
		b.WriteString(strconv.FormatBool(bool(v)))
	case Int:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case BigInt:
		b.WriteString(string(v))
	case Float:
		b.WriteString(FormatFloat(float64(v)))
	case String:
		b.WriteString(string(v))
	case Bytes:
		b.WriteString(hexLower(v))
	case List:
		b.WriteByte('[')
		for i, e := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			writeDisplay(b, e)
		}
		b.WriteByte(']')
	case Object:
		b.WriteString("Object()")
	case Date:
		b.WriteString(displayDate(v.UTC()))
	case Nodeset:
		b.WriteString("Nodeset(size=" + strconv.Itoa(int(v)) + ")")
	case Unit:
		b.WriteString("Unit")
	case Regex:
		b.WriteString("/" + strings.ReplaceAll(v.Source, "/", `\/`) + "/")
	case HTTPResponse:
		loc := "None"
		if v.HasLocation {
			loc = v.Location
		}
		b.WriteString("Response(location=" + loc + ", status=" + strconv.Itoa(v.Status) + ")")
	}
}

// Repr returns `kind <display>`, just the kind for Unit, or "none" for nil.
func Repr(v Value) string {
	switch v.(type) {
	case nil:
		return "none"
	case Unit:
		return KindUnit.String()
	}
	return v.Kind().String() + " <" + Display(v) + ">"
}

// Expected describes v as the expected side of a predicate.
func Expected(v Value) string {
	switch v := v.(type) {
	case Bool:
		return "boolean <" + Display(v) + ">"
	case Bytes:
		if len(v) > 1 {
			return strconv.Itoa(len(v)) + " bytes"
		}
		return strconv.Itoa(len(v)) + " byte"
	case Date:
		return "date <" + Display(v) + ">"
	case HTTPResponse:
		return "HTTP response <" + Display(v) + ">"
	case List:
		return "list of size " + strconv.Itoa(len(v))
	case Nodeset:
		return "list of size " + strconv.Itoa(int(v))
	case Null:
		return "null"
	case Int:
		return "integer <" + Display(v) + ">"
	case Float:
		return "float <" + Display(v) + ">"
	case BigInt:
		return "number <" + string(v) + ">"
	case Object:
		return "list of size " + strconv.Itoa(len(v))
	case Regex:
		return "regex <" + Display(v) + ">"
	case String:
		return "string <" + string(v) + ">"
	case Unit:
		return "something"
	}
	return Display(v)
}

// Render returns the text a template placeholder produces for v. Only
// null, booleans, numbers, strings and dates are renderable.
func Render(v Value) (string, bool) {
	switch v := v.(type) {
	case Null, Bool, Int, BigInt, Float, String:
		return Display(v), true
	case Date:
		return v.UTC().Format("2006-01-02T15:04:05.000000Z"), true
	}
	return "", false
}

const hexDigits = "0123456789abcdef"

func hexLower(b []byte) string {
	out := make([]byte, 2*len(b))
	for i, c := range b {
		out[2*i] = hexDigits[c>>4]
		out[2*i+1] = hexDigits[c&0x0f]
	}
	return string(out)
}
