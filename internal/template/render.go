// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package template evaluates `{{ }}` expressions and renders templates with
// typed variables.
package template

import (
	"errors"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// Variable is a named value; secret values are redacted from every output.
type Variable struct {
	Value  value.Value
	Secret bool
}

// Vars maps variable names to values.
type Vars map[string]Variable

// Set defines a public variable.
func (v Vars) Set(name string, val value.Value) { v[name] = Variable{Value: val} }

// SetSecret defines a secret string variable.
func (v Vars) SetSecret(name, s string) { v[name] = Variable{Value: value.String(s), Secret: true} }

// Get returns the value of a variable.
func (v Vars) Get(name string) (value.Value, bool) {
	x, ok := v[name]
	return x.Value, ok
}

// Env is the evaluation environment of an entry: its variables, the
// sources of the template functions and file access. Now and UUID default
// to the system clock and a random version 4 UUID; without ReadFile no file
// can be read.
type Env struct {
	Vars Vars
	Now  func() time.Time
	UUID func() string
	// ReadFile reads a file named in the request file. It returns an error
	// wrapping ErrFileAccessDenied for a path outside the allowed root.
	ReadFile func(name string) ([]byte, error)
	// Missing, when set, is consulted by Eval for a variable Vars does
	// not define, instead of failing with an UndefinedVariable error: it
	// returns the value to use in its place and whether it applied
	// (false falls through to the normal undefined-variable error, so
	// Missing may itself be selective). A run never sets this (nil,
	// meaning every undefined variable is an error, as always); it
	// exists for engine.RenderCurl, which renders a curl command line
	// for a variable a capture would only define once the entry actually
	// ran, and must not fail just because that hasn't happened. Vars
	// itself is never mutated to remember a Missing result: each call
	// site (an entry, in RenderCurl's case) gets its own answer, so nothing
	// leaks to a different context sharing the same Env.
	Missing func(name string) (value.Value, bool)
}

// ErrFileAccessDenied reports a file outside the allowed root.
var ErrFileAccessDenied = errors.New("file access denied")

// File renders a file name and reads the file.
func (e *Env) File(name *syntax.Template) ([]byte, error) {
	path, err := e.Render(name)
	if err != nil {
		return nil, err
	}
	if e.ReadFile != nil {
		b, err := e.ReadFile(path)
		if err == nil {
			return b, nil
		}
		if errors.Is(err, ErrFileAccessDenied) {
			rerr := runerr.New(name.Span, runerr.UnauthorizedFileAccess, false)
			rerr.Value = path
			return nil, rerr
		}
	}
	rerr := runerr.New(name.Span, runerr.FileReadAccess, false)
	rerr.Value = path
	return nil, rerr
}

// Eval evaluates an expression: a variable lookup or a function call.
func (e *Env) Eval(x syntax.Expr) (value.Value, error) {
	if x.Kind == syntax.ExprFunction {
		return e.call(x.Name), nil
	}
	v, ok := e.Vars.Get(x.Name)
	if !ok {
		if e.Missing != nil {
			if mv, ok := e.Missing(x.Name); ok {
				return mv, nil
			}
		}
		err := runerr.New(x.Span, runerr.UndefinedVariable, false)
		err.Value = x.Name
		return nil, err
	}
	return v, nil
}

// call runs a template function; the parser only accepts known names.
func (e *Env) call(name string) value.Value {
	switch name {
	case "newDate":
		now := time.Now
		if e.Now != nil {
			now = e.Now
		}
		return value.Date(now().UTC())
	default: // newUuid
		if e.UUID != nil {
			return value.String(e.UUID())
		}
		return value.String(newUUID())
	}
}

// RenderExpr evaluates an expression and renders its value as text.
func (e *Env) RenderExpr(x syntax.Expr) (string, error) {
	v, err := e.Eval(x)
	if err != nil {
		return "", err
	}
	s, ok := value.Render(v)
	if !ok {
		err := runerr.New(x.Span, runerr.Unrenderable, false)
		err.Value = value.Display(v)
		return "", err
	}
	return s, nil
}

// Render renders a template. Text after a placeholder's expression (as in
// `{{a b}}`) is ignored.
func (e *Env) Render(t *syntax.Template) (string, error) {
	if len(t.Elements) == 1 {
		if s, ok := t.Elements[0].(*syntax.TemplateString); ok {
			return s.Value, nil
		}
	}
	var b strings.Builder
	for _, el := range t.Elements {
		switch el := el.(type) {
		case *syntax.TemplateString:
			b.WriteString(el.Value)
		case *syntax.Placeholder:
			s, err := e.RenderExpr(el.Expr)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		}
	}
	return b.String(), nil
}
