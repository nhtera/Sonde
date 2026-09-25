// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"fmt"
	"strings"
)

// tryStringLiteral parses a single- or double-quoted string literal
// (RFC 9535 §2.3.1.1), decoding backslash escapes and \uXXXX / surrogate
// pair escapes.
func tryStringLiteral(r *reader) (string, bool, error) {
	if s, ok, err := tryQuotedString(r, '"'); err != nil {
		return "", false, err
	} else if ok {
		return s, true, nil
	}
	return tryQuotedString(r, '\'')
}

func tryQuotedString(r *reader, quote rune) (string, bool, error) {
	if !matchStr(string(quote), r) {
		return "", false, nil
	}
	other := '\''
	if quote == '\'' {
		other = '"'
	}

	var value strings.Builder
	for !r.isEOF() {
		pos := r.cursor()
		c, _ := r.peek()
		switch {
		case c == quote:
			r.read()
			return value.String(), true, nil
		case c == other:
			value.WriteRune(c)
			r.read()
		case c == '\\':
			r.read()
			escaped, err := parseEscapeSequence(r, quote)
			if err != nil {
				return "", false, err
			}
			value.WriteRune(escaped)
		case isUnescapedChar(c):
			value.WriteRune(c)
			r.read()
		default:
			return "", false, newParseError(pos, fmt.Sprintf("invalid character %q", c))
		}
	}
	return "", false, newParseError(r.cursor(), "expecting "+quoteStr(string(quote)))
}

// isUnescapedChar reports whether c may appear literally (unescaped)
// inside a string literal: unescaped = %x20-21 / %x23-26 / %x28-5B /
// %x5D-D7FF / %xE000-10FFFF (the surrounding quote and backslash are
// excluded by construction).
func isUnescapedChar(c rune) bool {
	switch {
	case c >= 0x20 && c <= 0x21:
		return true
	case c >= 0x23 && c <= 0x26:
		return true
	case c >= 0x28 && c <= 0x5B:
		return true
	case c >= 0x5D && c <= 0xD7FF:
		return true
	case c >= 0xE000 && c <= 0x10FFFF:
		return true
	}
	return false
}

func parseEscapeSequence(r *reader, quoteChar rune) (rune, error) {
	pos := r.cursor()
	c, ok := r.read()
	if !ok {
		return 0, newParseError(pos, "expecting escape character")
	}
	switch c {
	case 'b':
		return '\u0008', nil
	case 'f':
		return '\u000C', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case '/':
		return '/', nil
	case '\\':
		return '\\', nil
	case '"':
		if quoteChar == '"' {
			return '"', nil
		}
	case '\'':
		if quoteChar == '\'' {
			return '\'', nil
		}
	case 'u':
		return parseUnicodeEscape(r)
	}
	return 0, newParseError(pos, "invalid escape sequence \\"+string(c))
}

// parseUnicodeEscape parses the four hex digits after "\u", or (for a
// high surrogate) a following "\uXXXX" low surrogate.
func parseUnicodeEscape(r *reader) (rune, error) {
	if c, ok, err := tryNonSurrogate(r); err != nil {
		return 0, err
	} else if ok {
		return c, nil
	}
	if c, ok, err := trySurrogatePair(r); err != nil {
		return 0, err
	} else if ok {
		return c, nil
	}
	return 0, newParseError(r.cursor(), "invalid unicode escape")
}

func tryNonSurrogate(r *reader) (rune, bool, error) {
	save := r.cursor()
	c1, err := hexDigit(r)
	if err != nil {
		return 0, false, err
	}
	if c1 == 13 { // 'D' or 'd': possibly a surrogate.
		c2, err := hexDigit(r)
		if err != nil {
			return 0, false, err
		}
		if c2 >= 8 {
			// D8xx-DFxx: a surrogate, not a standalone code point.
			r.seek(save)
			return 0, false, nil
		}
		return readCodepoint(r, save, c1, c2)
	}
	return readCodepoint(r, save, c1, -1)
}

// readCodepoint reads the remaining hex digits of a \uXXXX escape. c1 and
// (if >= 0) c2 have already been read.
func readCodepoint(r *reader, savedPos int, c1, c2 int) (rune, bool, error) {
	if c2 < 0 {
		v, err := hexDigit(r)
		if err != nil {
			return 0, false, err
		}
		c2 = v
	}
	c3, err := hexDigit(r)
	if err != nil {
		return 0, false, err
	}
	c4, err := hexDigit(r)
	if err != nil {
		return 0, false, err
	}
	cp := c1*4096 + c2*256 + c3*16 + c4
	if !isValidCodepoint(cp) {
		return 0, false, newParseError(savedPos, fmt.Sprintf("invalid unicode escape %04X", cp))
	}
	return rune(cp), true, nil //nolint:gosec // isValidCodepoint bounds cp to [0, 0x10FFFF]
}

func trySurrogatePair(r *reader) (rune, bool, error) {
	pos := r.cursor()
	hi, ok, err := tryHighSurrogate(r)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	if err := expectStr("\\u", r); err != nil {
		return 0, false, err
	}
	lo, err := lowSurrogate(r)
	if err != nil {
		return 0, false, err
	}
	combined := 0x10000 + (hi << 10) + lo
	if !isValidCodepoint(combined) {
		return 0, false, newParseError(pos, fmt.Sprintf("invalid unicode escape %06X", combined))
	}
	return rune(combined), true, nil //nolint:gosec // isValidCodepoint bounds combined to [0, 0x10FFFF]
}

// tryHighSurrogate parses a high surrogate D800-DBFF, returning its
// low 10 bits. The leading digit is matched as the literal character
// "D" (matching the reference implementation this module is ported
// from), so a lowercase "\ud800" escape is not recognized as a
// surrogate lead.
func tryHighSurrogate(r *reader) (int, bool, error) {
	if !matchStr("D", r) {
		return 0, false, nil
	}
	c1, err := hexDigit(r)
	if err != nil {
		return 0, false, err
	}
	if c1 < 8 || c1 > 11 {
		return 0, false, nil
	}
	c2, err := hexDigit(r)
	if err != nil {
		return 0, false, err
	}
	c3, err := hexDigit(r)
	if err != nil {
		return 0, false, err
	}
	return (c1-8)*256 + c2*16 + c3, true, nil
}

// lowSurrogate parses a low surrogate DC00-DFFF, returning its low 10
// bits.
func lowSurrogate(r *reader) (int, error) {
	pos := r.cursor()
	if err := expectStr("D", r); err != nil {
		return 0, newParseError(pos, "expecting low surrogate")
	}
	c1, err := hexDigit(r)
	if err != nil {
		return 0, err
	}
	if c1 < 12 {
		return 0, newParseError(pos, "expecting low surrogate")
	}
	c2, err := hexDigit(r)
	if err != nil {
		return 0, err
	}
	c3, err := hexDigit(r)
	if err != nil {
		return 0, err
	}
	return (c1-12)*256 + c2*16 + c3, nil
}

func hexDigit(r *reader) (int, error) {
	pos := r.cursor()
	c, ok := r.read()
	if !ok {
		return 0, newParseError(pos, "expecting hex digit")
	}
	v := hexValue(c)
	if v < 0 {
		return 0, newParseError(pos, "expecting hex digit")
	}
	return v, nil
}

func hexValue(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func isValidCodepoint(cp int) bool {
	return cp >= 0 && cp <= 0x10FFFF && (cp < 0xD800 || cp > 0xDFFF)
}
