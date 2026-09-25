// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// templatize splits decoded characters into literal parts and {{ }}
// placeholders. Braces only count when written literally (an escaped `{`
// has a different source). end is where an unclosed placeholder is reported.
func templatize(r *reader, chars []encodedChar, end Pos) ([]TemplateElement, *Error) {
	const (
		inString = iota
		inTemplate
		firstOpen
		firstClose
	)
	var elements []TemplateElement
	var value, source strings.Builder
	state := inString
	var exprStart Pos
	exprStarted := false

	flush := func() {
		if value.Len() > 0 || source.Len() > 0 {
			elements = append(elements, &TemplateString{Value: value.String(), Source: source.String()})
		}
		value.Reset()
		source.Reset()
	}

	for _, ch := range chars {
		switch state {
		case inString:
			if ch.src == "{" {
				state = firstOpen
			} else {
				value.WriteRune(ch.c)
				source.WriteString(ch.src)
			}
		case firstOpen:
			if ch.src == "{" {
				flush()
				state = inTemplate
			} else {
				value.WriteByte('{')
				source.WriteByte('{')
				value.WriteRune(ch.c)
				source.WriteString(ch.src)
				state = inString
			}
		case inTemplate:
			if !exprStarted {
				exprStart, exprStarted = ch.pos, true
			}
			if ch.src == "}" {
				state = firstClose
			} else {
				source.WriteString(ch.src)
			}
		case firstClose:
			if ch.src == "}" {
				ph, err := placeholderContent(r, exprStart, exprStart.Offset+source.Len())
				if err != nil {
					return nil, err
				}
				elements = append(elements, ph)
				value.Reset()
				source.Reset()
				exprStarted = false
				state = inString
			} else {
				source.WriteByte('}')
				source.WriteString(ch.src)
				state = inTemplate
			}
		}
	}
	switch state {
	case firstOpen:
		value.WriteByte('{')
		source.WriteByte('{')
	case inTemplate, firstClose:
		return nil, expecting(end, false, "}}")
	}
	if value.Len() > 0 {
		flush()
	}
	return elements, nil
}

// placeholderContent parses `space expr space` between {{ and }} from the
// file range [from, to). Anything left after that is kept as Trailing.
func placeholderContent(r *reader, from Pos, to int) (*Placeholder, *Error) {
	sub := r.subReader(from, to)
	space0, _ := zeroOrMoreSpaces(sub)
	expr, err := expression(sub)
	if err != nil {
		return nil, err
	}
	space1, _ := zeroOrMoreSpaces(sub)
	return &Placeholder{Space0: space0, Expr: expr, Space1: space1, Trailing: r.src[sub.pos.Offset:to]}, nil
}

// placeholder parses `{{ expr }}` at the current position.
func placeholder(r *reader) (*Placeholder, *Error) {
	if err := tryLiteral(r, "{{"); err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	expr, err := expression(r)
	if err != nil {
		return nil, err
	}
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, "}}"); err != nil {
		return nil, err
	}
	return &Placeholder{Space0: space0, Expr: expr, Space1: space1}, nil
}

func isNameChar(c rune) bool { return isAlphanumeric(c) || c == '_' || c == '-' }

// expression is a function name (newDate, newUuid) or a variable name.
func expression(r *reader) (Expr, *Error) {
	start := r.pos
	name := r.readWhile(isNameChar)
	kind := ExprVariable
	switch name {
	case "newDate", "newUuid":
		kind = ExprFunction
	case "":
		return Expr{}, errAt(start, false, ErrTemplateVariable, "")
	}
	return Expr{Kind: kind, Name: name, Span: Span{start, r.pos}}, nil
}
