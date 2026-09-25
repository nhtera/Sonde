// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"unicode"
)

// encodedChar is one decoded character with its source text and position.
type encodedChar struct {
	c   rune
	src string
	pos Pos
}

// unquotedTemplate reads a value-string up to a comment or end of line;
// trailing spaces are left for the line terminator.
func unquotedTemplate(r *reader) (*Template, *Error) {
	start := r.pos
	end := start
	chars := r.scratch[:0]
	var spaces []encodedChar
	for {
		pos := r.pos
		c, src, err := anyChar(r, "#")
		if err == errEndOfString {
			break
		}
		if err != nil {
			return nil, err
		}
		if src == " " {
			spaces = append(spaces, encodedChar{c, src, pos})
			continue
		}
		chars = append(chars, spaces...)
		spaces = spaces[:0]
		chars = append(chars, encodedChar{c, src, pos})
		end = r.pos
	}
	r.pos = end
	r.scratch = chars
	elements, err := templatize(r, chars, end)
	if err != nil {
		return nil, err
	}
	return &Template{Elements: elements, Span: Span{start, end}}, nil
}

// quotedOnelineString reads "..." without escapes and returns the content.
func quotedOnelineString(r *reader) (string, *Error) {
	if err := literal(r, `"`); err != nil {
		return "", err
	}
	s := r.readWhile(func(c rune) bool { return c != '"' && c != '\n' })
	if err := literal(r, `"`); err != nil {
		return "", err
	}
	return s, nil
}

func quotedTemplate(r *reader) (*Template, *Error) {
	return delimitedTemplate(r, '"', `"`)
}

func backtickTemplate(r *reader) (*Template, *Error) {
	return delimitedTemplate(r, '`', "`\n")
}

func delimitedTemplate(r *reader, delim rune, except string) (*Template, *Error) {
	start := r.pos
	if err := tryLiteral(r, string(delim)); err != nil {
		return nil, err
	}
	end := r.pos
	chars := r.scratch[:0]
	for {
		pos := r.pos
		c, src, err := anyChar(r, except)
		if err == errEndOfString {
			break
		}
		if err != nil {
			return nil, err
		}
		chars = append(chars, encodedChar{c, src, pos})
		end = r.pos
	}
	r.scratch = chars
	if err := literal(r, string(delim)); err != nil {
		return nil, err
	}
	elements, err := templatize(r, chars, end)
	if err != nil {
		return nil, err
	}
	return &Template{Delimiter: delim, Elements: elements, Span: Span{start, r.pos}}, nil
}

// anyChar reads one (possibly escaped) character that is not in except and
// not a raw control character.
func anyChar(r *reader, except string) (rune, string, *Error) {
	start := r.pos
	if r.peekIs('\\') {
		c, err := escapeChar(r)
		if err != nil {
			return 0, "", err
		}
		return c, r.slice(start), nil
	}
	c, ok := r.read()
	if !ok || strings.ContainsRune(except, c) || strings.ContainsRune("\\\b\n\f\r\t", c) {
		r.pos = start
		return 0, "", errEndOfString
	}
	return c, r.slice(start), nil
}

// errEndOfString is the shared, recoverable "no more characters" result of
// anyChar. Callers only test it for identity and never modify it.
var errEndOfString = &Error{Kind: ErrExpecting, Arg: "char", recoverable: true}

func escapeChar(r *reader) (rune, *Error) {
	if err := tryLiteral(r, `\`); err != nil {
		return 0, err
	}
	start := r.pos
	c, _ := r.read()
	switch c {
	case '#', '"', '`', '\\', '/':
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
		return unicodeEscape(r)
	}
	return 0, errAt(start, false, ErrEscapeChar, "")
}

// unicodeEscape parses `{hexdigits}` after `\u`.
func unicodeEscape(r *reader) (rune, *Error) {
	if err := literal(r, "{"); err != nil {
		return 0, err
	}
	digits, err := oneOrMore(r, hexDigit)
	if err != nil {
		return 0, err
	}
	v := 0
	for _, d := range digits {
		v = min(v*16+int(d), unicode.MaxRune+1)
	}
	if v > unicode.MaxRune || (v >= 0xD800 && v < 0xE000) {
		return 0, errAt(r.pos, false, ErrUnicode, "")
	}
	if err := literal(r, "}"); err != nil {
		return 0, err
	}
	return rune(v), nil //nolint:gosec // G115: v <= unicode.MaxRune checked above
}

// isLetter matches Unicode alphabetic characters.
func isLetter(c rune) bool { return unicode.IsLetter(c) || unicode.Is(unicode.Other_Alphabetic, c) }

// isAlphanumeric matches Unicode alphabetic or numeric characters.
func isAlphanumeric(c rune) bool {
	return isLetter(c) || unicode.IsNumber(c)
}

func isKeyStringChar(c rune) bool {
	return isAlphanumeric(c) || strings.ContainsRune("_-.[]@$", c)
}

// keyString reads a key: key-string text, escapes and placeholders.
func keyString(r *reader) (*Template, *Error) {
	return escapedTemplate(r, keyStringContent, "key-string", func(p Pos) *Error {
		return expecting(p, false, "key-string")
	})
}

// filename reads a file name: anything but `#;{} \n\r\` unless escaped.
func filename(r *reader) (*Template, *Error) {
	return escapedTemplate(r, filenameContent(false), "filename", func(p Pos) *Error {
		return errAt(p, false, ErrFilename, "")
	})
}

// filenamePassword is filename that also accepts `\:` (kept escaped in the
// value so the password separator stays unambiguous).
func filenamePassword(r *reader) (*Template, *Error) {
	return escapedTemplate(r, filenameContent(true), "filename", func(p Pos) *Error {
		return errAt(p, false, ErrFilename, "")
	})
}

// escapedTemplate alternates placeholders and escaped text read by content.
// It must read something and may not start with `[`.
func escapedTemplate(r *reader, content func(*reader) (string, *Error), what string, empty func(Pos) *Error) (*Template, *Error) {
	start := r.pos
	var elements []TemplateElement
	for {
		save := r.pos
		ph, err := placeholder(r)
		if err == nil {
			elements = append(elements, ph)
			continue
		}
		if !err.recoverable {
			return nil, err
		}
		r.pos = save
		value, err := content(r)
		if err != nil {
			return nil, err
		}
		if value == "" {
			break
		}
		elements = append(elements, &TemplateString{Value: value, Source: r.slice(save)})
	}
	if len(elements) == 0 {
		return nil, empty(start)
	}
	if s, ok := elements[0].(*TemplateString); ok && strings.HasPrefix(s.Source, "[") {
		return nil, expecting(start, false, what)
	}
	return &Template{Elements: elements, Span: Span{start, r.pos}}, nil
}

func keyStringContent(r *reader) (string, *Error) {
	var b strings.Builder
	for {
		if r.peekIs('\\') {
			c, err := keyStringEscapedChar(r)
			if err != nil {
				return "", err
			}
			b.WriteRune(c)
			continue
		}
		s := r.readWhile(isKeyStringChar)
		if s == "" {
			return b.String(), nil
		}
		b.WriteString(s)
	}
}

func keyStringEscapedChar(r *reader) (rune, *Error) {
	if err := tryLiteral(r, `\`); err != nil {
		return 0, err
	}
	start := r.pos
	c, _ := r.read()
	switch c {
	case '#', ':', '\\', '/':
		return c, nil
	case 'b':
		return '\b', nil
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'u':
		return unicodeEscape(r)
	}
	return 0, errAt(start, false, ErrEscapeChar, "")
}

func filenameContent(password bool) func(*reader) (string, *Error) {
	return func(r *reader) (string, *Error) {
		var b strings.Builder
		for {
			if r.peekIs('\\') {
				c, err := filenameEscapedChar(r, password)
				if err != nil {
					return "", err
				}
				if c == ':' {
					b.WriteByte('\\')
				}
				b.WriteRune(c)
				continue
			}
			s := r.readWhile(func(c rune) bool { return !strings.ContainsRune("#;{} \n\r\\", c) })
			if s == "" {
				return b.String(), nil
			}
			b.WriteString(s)
		}
	}
}

func filenameEscapedChar(r *reader, password bool) (rune, *Error) {
	if err := tryLiteral(r, `\`); err != nil {
		return 0, err
	}
	start := r.pos
	c, _ := r.read()
	switch c {
	case '\\', '#', ';', ' ', '{', '}':
		return c, nil
	case ':':
		if password {
			return c, nil
		}
	case 'b':
		return '\b', nil
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'u':
		return unicodeEscape(r)
	}
	return 0, errAt(start, false, ErrEscapeChar, "")
}
