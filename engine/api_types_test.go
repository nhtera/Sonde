// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// TestErrorKinds checks that every runtime error kind has its own public
// kind.
func TestErrorKinds(t *testing.T) {
	seen := map[ErrorKind]runerr.Kind{ErrorParse: -1}
	for k := range runerr.KindCount {
		pk := (&Error{run: &runerr.Error{Kind: k}}).Kind()
		if pk == "" {
			t.Errorf("runerr kind %d has no public kind", k)
			continue
		}
		if prev, ok := seen[pk]; ok {
			t.Errorf("kinds %d and %d both map to %q", prev, k, pk)
		}
		seen[pk] = k
	}
}

// TestValueKinds checks that every value kind has its own public kind.
func TestValueKinds(t *testing.T) {
	seen := map[ValueKind]bool{}
	for k, pk := range valueKinds {
		if pk == "" || seen[pk] {
			t.Errorf("value kind %d maps to %q (empty or duplicate)", k, pk)
		}
		seen[pk] = true
	}
}

// TestErrorRender checks that an Error renders exactly like the error it
// wraps, located in its file.
func TestErrorRender(t *testing.T) {
	src := []byte("GET http://a\nHTTP 200\n")
	re := runerr.New(syntax.Span{Start: syntax.Pos{Offset: 18, Line: 2, Col: 6}, End: syntax.Pos{Offset: 21, Line: 2, Col: 9}}, runerr.AssertStatus, false)
	re.Actual = "404"
	e := &Error{run: re, file: "a.hurl", src: src, entry: 1}
	if got, want := e.Render(), re.Render("a.hurl", string(src), 1); got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
	if got, want := e.RenderColor(), re.RenderColor("a.hurl", string(src), 1); got != want {
		t.Errorf("RenderColor() = %q, want %q", got, want)
	}
	if e.Kind() != ErrorAssertStatus || e.Assert() || e.Actual() != "404" || e.Span().Start.Line != 2 {
		t.Errorf("kind %q assert %v actual %q span %+v", e.Kind(), e.Assert(), e.Actual(), e.Span())
	}
	if e.Error() != re.Error() {
		t.Errorf("Error() = %q, want %q", e.Error(), re.Error())
	}
}

// TestParseErrorSkipsBOM checks that a parse error of a file starting with
// a byte order mark shows the line without it, like the parser counts.
func TestParseErrorSkipsBOM(t *testing.T) {
	src := []byte("\xEF\xBB\xBFGET\n")
	res, err := NewRunner(Options{}).RunSource(context.Background(), "a.hurl", src)
	if err != nil {
		t.Fatal(err)
	}
	pe := res.ParseError
	if pe == nil || pe.Kind() != ErrorParse || pe.Assert() {
		t.Fatalf("ParseError = %v", pe)
	}
	var serr *syntax.Error
	if _, perr := syntax.Parse("a.hurl", src, syntax.DialectHurl); !errors.As(perr, &serr) {
		t.Fatal("no syntax error")
	}
	if got, want := pe.Render(), serr.Render("a.hurl", src[3:]); got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
	if pe.Span().Start != pe.Span().End {
		t.Errorf("parse error span %+v is not a position", pe.Span())
	}
}

// TestRenderCurlParseError checks that RenderCurl reports a file that
// does not parse as an *Error of kind ErrorParse.
func TestRenderCurlParseError(t *testing.T) {
	_, err := NewRunner(Options{}).RenderCurl(context.Background(), "a.hurl", []byte("GET\n"))
	var e *Error
	if !errors.As(err, &e) || e.Kind() != ErrorParse {
		t.Fatalf("err = %v, want a parse *Error", err)
	}
}

func TestValueAccessors(t *testing.T) {
	date := time.Date(2026, 9, 27, 1, 2, 3, 0, time.UTC)
	obj := Value{v: value.Object{{Key: "b", Value: value.Int(1)}, {Key: "a", Value: value.List{value.String("x"), value.Null{}}}}}
	tests := []struct {
		v    Value
		kind ValueKind
		str  string
	}{
		{Value{}, "", "none"},
		{Value{v: value.Null{}}, ValueNull, "null"},
		{Value{v: value.Bool(true)}, ValueBool, "true"},
		{Value{v: value.Int(3)}, ValueInteger, "3"},
		{Value{v: value.BigInt("123456789012345678901234567890")}, ValueInteger, "123456789012345678901234567890"},
		{Value{v: value.Float(1.5)}, ValueFloat, "1.5"},
		{Value{v: value.String("s")}, ValueString, "s"},
		{Value{v: value.Bytes("hi")}, ValueBytes, "6869"},
		{Value{v: value.Date(date)}, ValueDate, "2026-09-27 01:02:03 UTC"},
		{Value{v: value.Nodeset(4)}, ValueNodeset, "Nodeset(size=4)"},
		{Value{v: value.Unit{}}, ValueUnit, "Unit"},
		{Value{v: value.HTTPResponse{Status: 302, Location: "/x", HasLocation: true}}, ValueRedirect, "Response(location=/x, status=302)"},
		{obj, ValueObject, "Object()"},
	}
	for _, tt := range tests {
		if tt.v.Kind() != tt.kind || tt.v.String() != tt.str {
			t.Errorf("%#v: kind %q string %q, want %q %q", tt.v, tt.v.Kind(), tt.v.String(), tt.kind, tt.str)
		}
	}
	if i, ok := (Value{v: value.Int(3)}).Int(); !ok || i != 3 {
		t.Errorf("Int() = %d, %v", i, ok)
	}
	if _, ok := (Value{v: value.BigInt("123456789012345678901234567890")}).Int(); ok {
		t.Error("Int() of a big integer is ok")
	}
	if _, ok := (Value{v: value.Int(3)}).Float(); ok {
		t.Error("Float() of an integer is ok")
	}
	if d, ok := (Value{v: value.Date(date)}).Time(); !ok || !d.Equal(date) {
		t.Errorf("Time() = %v, %v", d, ok)
	}
	if r, ok := (Value{v: value.HTTPResponse{Status: 302, Location: "/x", HasLocation: true}}).Redirect(); !ok || r != (Redirect{Status: 302, Location: "/x", HasLocation: true}) {
		t.Errorf("Redirect() = %+v, %v", r, ok)
	}
	fields := obj.Fields()
	if len(fields) != 2 || fields[0].Key != "b" || fields[1].Key != "a" || obj.Len() != 2 {
		t.Fatalf("Fields() = %+v", fields)
	}
	a, ok := obj.Get("a")
	if !ok || a.Kind() != ValueList || a.Len() != 2 || a.List()[0].String() != "x" {
		t.Errorf("Get(a) = %v, %v", a, ok)
	}
	if s, ok := a.List()[0].Text(); !ok || s != "x" {
		t.Errorf("Text() = %q, %v", s, ok)
	}
	if (Value{v: value.Int(1)}).List() != nil || (Value{v: value.Int(1)}).Fields() != nil {
		t.Error("List/Fields of an integer are not nil")
	}
}

// TestValueAsVariable checks that a Value, such as a capture of another
// run, is accepted as a variable.
func TestValueAsVariable(t *testing.T) {
	for _, v := range []Value{{}, {v: value.Int(2)}, {v: value.String("x")}} {
		got, err := toValue(v)
		if err != nil {
			t.Fatal(err)
		}
		if want := v.v; want == nil {
			if _, ok := got.(value.Null); !ok {
				t.Errorf("zero Value became %#v, want null", got)
			}
		} else if got != want {
			t.Errorf("toValue(%v) = %#v", v, got)
		}
	}
}

// TestZeroError checks that the zero Error does not panic.
func TestZeroError(t *testing.T) {
	var e Error
	if e.Error() != "" || e.Kind() != "" || e.Assert() || e.Span() != (Span{}) || e.Description() != "" ||
		e.Message() != "" || e.Actual() != "" || e.Expected() != "" || e.Render() != "" || e.RenderColor() != "" {
		t.Error("zero Error is not empty")
	}
}

// TestValueBytesCopy checks that Bytes does not expose the captured value.
func TestValueBytesCopy(t *testing.T) {
	v := Value{v: value.Bytes("hi")}
	b, _ := v.Bytes()
	b[0] = 'X'
	if got, _ := v.Bytes(); string(got) != "hi" {
		t.Errorf("value changed to %q", got)
	}
}

// TestValueNotComparable checks that Value can't be compared with ==,
// which would panic on a list or bytes value.
func TestValueNotComparable(t *testing.T) {
	if reflect.TypeFor[Value]().Comparable() {
		t.Error("Value is comparable")
	}
}
