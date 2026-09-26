// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"strings"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/syntax"
)

// definitionKind says how a document defines a variable.
type definitionKind int

const (
	defCapture definitionKind = iota // [Captures] name: query
	defOption                        // [Options] variable: name=value
)

// definition is a variable a document defines itself.
type definition struct {
	name string
	kind definitionKind
	// span is the name as written.
	span syntax.Span
	// from is the offset where the variable becomes usable: the start of
	// the entry whose [Options] defines it, or the start of the response
	// whose [Captures] does (captures run before asserts, whatever the
	// section order).
	from int
	// redact is set for `redact` captures, whose value is a secret.
	redact bool
}

// definitions returns every variable f defines, in source order.
func definitions(f *syntax.File) []definition {
	var defs []definition
	for _, e := range f.Entries {
		for _, sec := range e.Request.Sections {
			for _, o := range sec.Options {
				if v, ok := o.Value.(*syntax.VariableDefinition); ok {
					name := v.Span
					name.End = name.Start
					name.End.Offset += len(v.Name)
					name.End.Col += utf8.RuneCountInString(v.Name)
					defs = append(defs, definition{name: v.Name, kind: defOption, span: name, from: e.Request.Span.Start.Offset})
				}
			}
		}
		if e.Response == nil {
			continue
		}
		for _, sec := range e.Response.Sections {
			for _, c := range sec.Captures {
				name, ok := literal(c.Name)
				if !ok {
					continue
				}
				defs = append(defs, definition{
					name: name, kind: defCapture, span: c.Name.Span,
					from: e.Response.Span.Start.Offset, redact: c.Redact,
				})
			}
		}
	}
	return defs
}

// visibleAt returns the definitions usable at offset, the latest one for
// each name.
func visibleAt(defs []definition, offset int) map[string]definition {
	out := map[string]definition{}
	for _, d := range defs {
		if d.from <= offset {
			out[d.name] = d
		}
	}
	return out
}

// use is one {{name}} reference to a variable.
type use struct {
	name string
	span syntax.Span // the name inside the braces
}

// literal returns t's text when it has no placeholders.
func literal(t *syntax.Template) (string, bool) {
	if t == nil {
		return "", false
	}
	var b strings.Builder
	for _, el := range t.Elements {
		s, ok := el.(*syntax.TemplateString)
		if !ok {
			return "", false
		}
		b.WriteString(s.Value)
	}
	return b.String(), true
}
