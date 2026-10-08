// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strconv"

// maxJSONDepth bounds JSON nesting, as the reference does since 8.1.0
// (which also keeps hostile input from exhausting the stack).
const maxJSONDepth = 128

// jsonParser carries the nesting depth through the recursive JSON rules.
type jsonParser struct{ depth int }

// jsonValue parses a JSON value where JSON strings may hold placeholders and
// a bare {{ }} may stand for a whole value.
func jsonValue(r *reader) (JSONValue, *Error) {
	return (&jsonParser{}).value(r)
}

var numberPrefixes = []string{"-", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

func (p *jsonParser) value(r *reader) (JSONValue, *Error) {
	return prefixChoice(r, []alt[JSONValue]{
		{[]string{"null"}, func(r *reader) (JSONValue, *Error) { return asJSON(null(r)) }},
		{[]string{"true", "false"}, func(r *reader) (JSONValue, *Error) { return asJSON(boolean(r)) }},
		{[]string{`"`}, func(r *reader) (JSONValue, *Error) { return asJSON(jsonString(r)) }},
		{numberPrefixes, func(r *reader) (JSONValue, *Error) { return asJSON(jsonNumber(r)) }},
		{[]string{"{{"}, func(r *reader) (JSONValue, *Error) { return asJSON(placeholder(r)) }},
		{[]string{"["}, p.list},
		{[]string{"{"}, p.object},
	})
}

func asJSON[T JSONValue](v T, err *Error) (JSONValue, *Error) {
	if err != nil {
		return nil, err
	}
	return v, nil
}

// valueInJSON is a value inside a list or object: failures are fatal.
func (p *jsonParser) valueInJSON(r *reader) (JSONValue, *Error) {
	if r.peekIs(',') {
		return nil, errAt(r.pos, false, ErrJSONEmptyElement, "")
	}
	v, err := p.value(r)
	if err != nil {
		if err.recoverable {
			return nil, errAt(err.Pos, false, ErrJSONExpectingElement, "")
		}
		return nil, err
	}
	return v, nil
}

func jsonString(r *reader) (*Template, *Error) {
	begin := r.pos
	if err := tryLiteral(r, `"`); err != nil {
		return nil, err
	}
	chars := r.scratch[:0]
	for !r.peekIs('"') && !r.isEOF() {
		ch, err := jsonChar(r)
		if err != nil {
			return nil, err
		}
		chars = append(chars, ch)
	}
	r.scratch = chars
	end := r.pos
	if err := literal(r, `"`); err != nil {
		return nil, err
	}
	elements, err := templatize(r, chars, end)
	if err != nil {
		return nil, err
	}
	return &Template{Delimiter: '"', Elements: elements, Span: Span{begin, r.pos}}, nil
}

func jsonChar(r *reader) (encodedChar, *Error) {
	start := r.pos
	if r.peekIs('\\') {
		c, err := jsonEscapeChar(r)
		if err != nil {
			return encodedChar{}, err
		}
		return encodedChar{c, r.slice(start), start}, nil
	}
	c, ok := r.read()
	if !ok || c == '\\' || c == '\b' || c == '\n' || c == '\f' || c == '\r' || c == '\t' {
		return encodedChar{}, expecting(start, true, "char")
	}
	return encodedChar{c, r.slice(start), start}, nil
}

func jsonEscapeChar(r *reader) (rune, *Error) {
	if err := tryLiteral(r, `\`); err != nil {
		return 0, err
	}
	start := r.pos
	c, _ := r.read()
	switch c {
	case '"', '\\', '/':
		return c, nil
	case 'b':
		return '\b', nil
	case 'n':
		return '\n', nil
	case 'f':
		return '\f', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'u':
		return jsonUnicode(r)
	}
	return 0, errAt(start, false, ErrEscapeChar, "")
}

// jsonUnicode parses XXXX after \u, combining a surrogate pair \uXXXX\uXXXX.
func jsonUnicode(r *reader) (rune, *Error) {
	cp, err := jsonHex4(r)
	if err != nil {
		return 0, err
	}
	if cp >= 0xD800 && cp < 0xE000 {
		if err := literal(r, `\u`); err != nil {
			return 0, err
		}
		start2 := r.pos
		cp2, err := jsonHex4(r)
		if err != nil {
			return 0, err
		}
		if cp >= 0xDC00 || cp2 < 0xDC00 || cp2 >= 0xE000 {
			return 0, errAt(start2, false, ErrUnicode, "")
		}
		return rune((cp-0xD800)<<10 | (cp2 - 0xDC00) + 0x10000), nil //nolint:gosec // G115: < 0x110000 by construction
	}
	return rune(cp), nil //nolint:gosec // G115: four hex digits fit in a rune
}

func jsonHex4(r *reader) (int, *Error) {
	v := 0
	for range 4 {
		d, err := committing(hexDigit)(r)
		if err != nil {
			return 0, err
		}
		v = v*16 + int(d)
	}
	return v, nil
}

func isDigit(c rune) bool { return c >= '0' && c <= '9' }

func jsonNumber(r *reader) (*JSONNumber, *Error) {
	start := r.pos
	r.consume("-")
	if !r.consume("0") {
		if r.readWhile(isDigit) == "" {
			return nil, expecting(start, true, "number")
		}
	}
	if r.consume(".") {
		if r.readWhile(isDigit) == "" {
			return nil, expecting(r.pos, false, "digits")
		}
	}
	if r.peekIs('e') || r.peekIs('E') {
		r.read()
		if !r.consume("-") {
			r.consume("+")
		}
		r.readWhile(isDigit)
	}
	return &JSONNumber{Source: r.slice(start)}, nil
}

func isJSONWhitespace(c rune) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// enter goes one level deeper, at the bracket that opens it.
func (p *jsonParser) enter(open Pos) *Error {
	p.depth++
	if p.depth > maxJSONDepth {
		return errAt(open, false, ErrNestingTooDeep, strconv.Itoa(maxJSONDepth))
	}
	return nil
}

func (p *jsonParser) list(r *reader) (JSONValue, *Error) {
	open := r.pos
	if err := tryLiteral(r, "["); err != nil {
		return nil, err
	}
	if err := p.enter(open); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	list := &JSONList{Space0: r.readWhile(isJSONWhitespace)}
	if !r.peekIs(']') {
		for {
			el, err := p.listElement(r)
			if err != nil {
				return nil, err
			}
			list.Elements = append(list.Elements, el)
			if !r.peekIs(',') {
				break
			}
			save := r.pos
			r.read()
			if c, ok := r.peekFirst(func(c rune) bool { return !isJSONWhitespace(c) }); ok && c == ']' {
				return nil, errAt(save, false, ErrJSONTrailingComma, "")
			}
		}
	}
	if err := literal(r, "]"); err != nil {
		return nil, err
	}
	return list, nil
}

func (p *jsonParser) listElement(r *reader) (*JSONListElement, *Error) {
	space0 := r.readWhile(isJSONWhitespace)
	v, err := p.valueInJSON(r)
	if err != nil {
		return nil, err
	}
	return &JSONListElement{Space0: space0, Value: v, Space1: r.readWhile(isJSONWhitespace)}, nil
}

// jsonObject parses an object at the top level (used by GraphQL variables).
func jsonObject(r *reader) (*JSONObject, *Error) {
	v, err := (&jsonParser{}).object(r)
	if err != nil {
		return nil, err
	}
	return v.(*JSONObject), nil
}

func (p *jsonParser) object(r *reader) (JSONValue, *Error) {
	open := r.pos
	if err := tryLiteral(r, "{"); err != nil {
		return nil, err
	}
	if err := p.enter(open); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	obj := &JSONObject{Space0: r.readWhile(isJSONWhitespace)}
	if !r.peekIs('}') {
		for {
			el, err := p.objectElement(r)
			if err != nil {
				return nil, err
			}
			obj.Elements = append(obj.Elements, el)
			if !r.peekIs(',') {
				break
			}
			save := r.pos
			r.read()
			if c, ok := r.peekFirst(func(c rune) bool { return !isJSONWhitespace(c) }); ok && c == '}' {
				return nil, errAt(save, false, ErrJSONTrailingComma, "")
			}
		}
	}
	if err := literal(r, "}"); err != nil {
		return nil, err
	}
	return obj, nil
}

func (p *jsonParser) objectElement(r *reader) (*JSONObjectElement, *Error) {
	space0 := r.readWhile(isJSONWhitespace)
	name, err := committing(jsonString)(r)
	if err != nil {
		return nil, err
	}
	space1 := r.readWhile(isJSONWhitespace)
	if err := literal(r, ":"); err != nil {
		return nil, err
	}
	save := r.pos
	space2 := r.readWhile(isJSONWhitespace)
	if c, ok := r.peek(); !ok || c == '}' {
		return nil, errAt(save, false, ErrJSONEmptyElement, "")
	}
	v, err := p.valueInJSON(r)
	if err != nil {
		return nil, err
	}
	return &JSONObjectElement{Space0: space0, Name: name, Space1: space1, Space2: space2,
		Value: v, Space3: r.readWhile(isJSONWhitespace)}, nil
}
