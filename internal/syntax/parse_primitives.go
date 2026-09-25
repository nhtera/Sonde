// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"regexp"
	"strings"
)

func isSpace(c rune) bool { return c == ' ' || c == '\t' }

func zeroOrMoreSpaces(r *reader) (Whitespace, *Error) {
	start := r.pos
	v := r.readWhile(isSpace)
	return Whitespace{Value: v, Span: Span{start, r.pos}}, nil
}

func oneOrMoreSpaces(r *reader) (Whitespace, *Error) {
	start := r.pos
	if c, ok := r.peek(); !ok || !isSpace(c) {
		return Whitespace{}, errAt(start, false, ErrSpace, "")
	}
	return zeroOrMoreSpaces(r)
}

func emptyWhitespace(p Pos) Whitespace { return Whitespace{Span: Span{p, p}} }

// lineTerminator is `sp* comment? newline`, or `sp* comment?` at EOF.
func lineTerminator(r *reader) (*LineTerminator, *Error) {
	space0, _ := zeroOrMoreSpaces(r)
	var c *Comment
	if r.peekIs('#') {
		c, _ = comment(r)
	}
	var nl Whitespace
	if r.isEOF() {
		nl = emptyWhitespace(r.pos)
	} else {
		if !r.peekPrefix("\n") && !r.peekPrefix("\r\n") {
			return nil, expecting(r.pos, false, "line_terminator")
		}
		nl, _ = newline(r)
	}
	return &LineTerminator{Space0: space0, Comment: c, Newline: nl}, nil
}

// optionalLineTerminators reads blank and comment lines (a trailing
// `sp* comment?` at EOF counts as one).
func optionalLineTerminators(r *reader) ([]*LineTerminator, *Error) {
	var lts []*LineTerminator
	for !r.isEOF() {
		save := r.pos
		r.readWhile(isSpace)
		ok := r.isEOF() || r.peekIs('#') || r.peekPrefix("\n") || r.peekPrefix("\r\n")
		r.pos = save
		if !ok {
			break
		}
		lt, err := lineTerminator(r)
		if err != nil {
			return nil, err
		}
		lts = append(lts, lt)
	}
	return lts, nil
}

func comment(r *reader) (*Comment, *Error) {
	if err := tryLiteral(r, "#"); err != nil {
		return nil, err
	}
	start := r.pos
	for !r.isEOF() && !r.peekPrefix("\n") && !r.peekPrefix("\r\n") {
		r.read()
	}
	return &Comment{Value: r.slice(start), Span: Span{start, r.pos}}, nil
}

// advance consumes the next n bytes (a prefix already checked by the caller).
func (r *reader) advance(n int) {
	end := r.pos.Offset + n
	for r.pos.Offset < end {
		r.read()
	}
}

// consume consumes s if the input starts with it.
func (r *reader) consume(s string) bool {
	if !r.peekPrefix(s) {
		return false
	}
	r.advance(len(s))
	return true
}

// literal consumes s or fails (non-recoverably) at the current position.
func literal(r *reader, s string) *Error {
	if !r.peekPrefix(s) {
		return expecting(r.pos, false, s)
	}
	r.advance(len(s))
	return nil
}

// tryLiteral consumes s or fails recoverably without consuming anything.
func tryLiteral(r *reader, s string) *Error {
	if !r.peekPrefix(s) {
		return expecting(r.pos, true, s)
	}
	r.advance(len(s))
	return nil
}

func newline(r *reader) (Whitespace, *Error) {
	start := r.pos
	switch {
	case r.peekPrefix("\r\n"):
		r.advance(2)
	case r.peekPrefix("\n"):
		r.advance(1)
	default:
		return Whitespace{}, expecting(start, false, "newline")
	}
	return Whitespace{Value: r.slice(start), Span: Span{start, r.pos}}, nil
}

func keyValue(r *reader) (*KeyValue, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	key, err := recovering(keyString)(r)
	if err != nil {
		return nil, err
	}
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, ":"); err != nil {
		return nil, asRecoverable(err, true)
	}
	space2, _ := zeroOrMoreSpaces(r)
	value, err := unquotedTemplate(r)
	if err != nil {
		return nil, err
	}
	lt0, err := lineTerminator(r)
	if err != nil {
		return nil, err
	}
	return &KeyValue{LineTerminators: lts, Space0: space0, Key: key, Space1: space1,
		Space2: space2, Value: value, LineTerminator0: lt0}, nil
}

func hexDigitValue(c rune) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return byte(c - '0'), true
	case c >= 'a' && c <= 'f':
		return byte(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return byte(c-'A') + 10, true
	}
	return 0, false
}

func hexDigit(r *reader) (byte, *Error) {
	start := r.pos
	c, ok := r.read()
	if !ok {
		return 0, errAt(start, true, ErrHexDigit, "")
	}
	d, ok := hexDigitValue(c)
	if !ok {
		return 0, errAt(start, true, ErrHexDigit, "")
	}
	return d, nil
}

func hexBytes(r *reader) (*Hex, *Error) {
	if err := tryLiteral(r, "hex"); err != nil {
		return nil, err
	}
	if err := literal(r, ","); err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	start := r.pos
	var value []byte
	var high byte
	odd := false
	for {
		save := r.pos
		d, err := hexDigit(r)
		if err != nil {
			r.pos = save
			break
		}
		if odd {
			value = append(value, high<<4|d)
		} else {
			high = d
		}
		odd = !odd
	}
	if odd {
		return nil, errAt(r.pos, false, ErrOddNumberOfHexDigits, "")
	}
	source := r.slice(start)
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, ";"); err != nil {
		return nil, err
	}
	return &Hex{Space0: space0, Value: value, Source: source, Space1: space1}, nil
}

// regexLiteral parses /pattern/ where `\/` escapes a slash.
func regexLiteral(r *reader) (*Regex, *Error) {
	begin := r.pos
	if err := tryLiteral(r, "/"); err != nil {
		return nil, err
	}
	start := r.pos
	var pattern strings.Builder
	for {
		c, ok := r.read()
		if !ok {
			return nil, errAt(r.pos, false, ErrRegexExpr, "unexpected end of file")
		}
		if c == '/' {
			break
		}
		if c == '\\' && r.peekIs('/') {
			r.read()
			pattern.WriteByte('/')
			continue
		}
		pattern.WriteRune(c)
	}
	if msg := validateRegex(pattern.String()); msg != "" {
		return nil, errAt(start, false, ErrRegexExpr, msg)
	}
	return &Regex{Source: r.slice(begin), Pattern: pattern.String()}, nil
}

var countedRepetition = regexp.MustCompile(`^\{[0-9]+(,[0-9]*)?\}`)

// validateRegex compiles pattern and also rejects a `{` that does not start
// a valid counted repetition (Go's engine would silently treat it as a
// literal brace). Braces inside escapes such as \p{L} or \x{4F} and inside
// character classes are not repetitions. It returns an error message or "".
func validateRegex(pattern string) string {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i = skipEscape(pattern, i)
		case '[':
			i = skipClass(pattern, i)
		case '{':
			if !countedRepetition.MatchString(pattern[i:]) {
				return "repetition quantifier expects a valid decimal"
			}
		}
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return strings.TrimPrefix(err.Error(), "error parsing regexp: ")
	}
	return ""
}

// skipEscape returns the index of the last byte of the escape at p[i].
func skipEscape(p string, i int) int {
	if i+1 >= len(p) {
		return i
	}
	switch p[i+1] {
	case 'p', 'P', 'x':
		if i+2 < len(p) && p[i+2] == '{' {
			if j := strings.IndexByte(p[i+2:], '}'); j >= 0 {
				return i + 2 + j
			}
		}
	}
	return i + 1
}

// skipClass returns the index of the `]` closing the class opened at p[i]
// (a leading `]` is literal; `[:name:]` and nested classes are skipped).
func skipClass(p string, i int) int {
	j := i + 1
	if j < len(p) && p[j] == '^' {
		j++
	}
	if j < len(p) && p[j] == ']' {
		j++
	}
	for ; j < len(p); j++ {
		switch p[j] {
		case '\\':
			j = skipEscape(p, j)
		case '[':
			if strings.HasPrefix(p[j:], "[:") {
				if k := strings.Index(p[j+2:], ":]"); k >= 0 {
					j += 2 + k + 1
					continue
				}
			}
			j = skipClass(p, j)
		case ']':
			return j
		}
	}
	return len(p) - 1
}

func null(r *reader) (*Null, *Error) {
	if err := tryLiteral(r, "null"); err != nil {
		return nil, err
	}
	return &Null{}, nil
}

func boolean(r *reader) (*Boolean, *Error) {
	start := r.pos
	if r.consume("true") {
		return &Boolean{Value: true}, nil
	}
	if r.consume("false") {
		return &Boolean{Value: false}, nil
	}
	return nil, expecting(start, true, "true|false")
}

func fileRef(r *reader) (*FileRef, *Error) {
	if err := tryLiteral(r, "file"); err != nil {
		return nil, err
	}
	if err := literal(r, ","); err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	name, err := filename(r)
	if err != nil {
		return nil, err
	}
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, ";"); err != nil {
		return nil, err
	}
	return &FileRef{Space0: space0, Filename: name, Space1: space1}, nil
}

func base64Bytes(r *reader) (*Base64, *Error) {
	if err := tryLiteral(r, "base64"); err != nil {
		return nil, err
	}
	if err := literal(r, ","); err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	start := r.pos
	value := decodeBase64(r)
	source := r.slice(start)
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, ";"); err != nil {
		return nil, err
	}
	return &Base64{Space0: space0, Value: value, Source: source, Space1: space1}, nil
}

// decodeBase64 leniently decodes standard base64, skipping spaces, tabs and
// newlines and stopping at the first `=` run or foreign character.
func decodeBase64(r *reader) []byte {
	var out []byte
	var buf []byte
	for r.readWhile(func(c rune) bool { return c == '=' }) == "" {
		save := r.pos
		c, ok := r.read()
		if !ok {
			break
		}
		if c == ' ' || c == '\n' || c == '\t' {
			continue
		}
		v, ok := base64Value(c)
		if !ok {
			r.pos = save
			break
		}
		buf = append(buf, v)
		if len(buf) == 4 {
			out = append(out, buf[0]<<2|buf[1]>>4, buf[1]<<4|buf[2]>>2, buf[2]<<6|buf[3])
			buf = buf[:0]
		}
	}
	switch len(buf) {
	case 2:
		out = append(out, buf[0]<<2|buf[1]>>4)
	case 3:
		out = append(out, buf[0]<<2|buf[1]>>4, buf[1]<<4|buf[2]>>2)
	}
	return out
}

// base64Value returns the 6-bit value of a base64 digit.
func base64Value(c rune) (byte, bool) {
	switch {
	case c >= 'A' && c <= 'Z':
		return byte(c - 'A'), true
	case c >= 'a' && c <= 'z':
		return byte(c-'a') + 26, true
	case c >= '0' && c <= '9':
		return byte(c-'0') + 52, true
	case c == '+':
		return 62, true
	case c == '/':
		return 63, true
	}
	return 0, false
}

func eof(r *reader) *Error {
	if r.isEOF() {
		return nil
	}
	return expecting(r.pos, false, "eof")
}
