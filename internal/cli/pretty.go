// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"strings"

	"github.com/nhtera/sonde/exchange"
)

// ANSI SGR codes for --pretty --color, one per JSON token kind (ansiReset
// is shared with logger.go).
const (
	ansiPunct  = "\x1b[1;39m" // braces, brackets, ':', ','
	ansiKey    = "\x1b[1;34m" // object member names
	ansiString = "\x1b[0;32m" // string values
	ansiNumber = "\x1b[0;36m"
	ansiBool   = "\x1b[0;33m"
	ansiNull   = "\x1b[0;35m"
)

// prettyBody re-indents a JSON response body two spaces per level,
// optionally colorizing it (color); any other content type, or a body
// that does not parse as valid JSON, is returned unchanged. String and
// number literals are copied byte for byte from the source, so escapes
// and numeric formatting are never altered.
func prettyBody(body []byte, r *exchange.Response, color bool) []byte {
	ct, _ := r.ContentType()
	if !strings.Contains(strings.ToLower(ct), "json") {
		return body
	}
	p := &jsonPrettyPrinter{src: body, color: color}
	if !p.printValue() || !p.atEnd() {
		return body
	}
	p.buf.WriteByte('\n')
	return p.buf.Bytes()
}

// jsonPrettyPrinter is a small recursive-descent JSON re-printer: it never
// builds a value tree, only replays the source's own tokens with new
// whitespace (and, optionally, color) around them.
type jsonPrettyPrinter struct {
	src   []byte
	pos   int
	depth int
	color bool
	buf   bytes.Buffer
}

func (p *jsonPrettyPrinter) atEnd() bool {
	p.skipWS()
	return p.pos >= len(p.src)
}

func (p *jsonPrettyPrinter) skipWS() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonPrettyPrinter) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

// write appends text, wrapped in color when set and colorizing is on.
func (p *jsonPrettyPrinter) write(color, text string) {
	if p.color && color != "" {
		p.buf.WriteString(color)
		p.buf.WriteString(text)
		p.buf.WriteString(ansiReset)
		return
	}
	p.buf.WriteString(text)
}

func (p *jsonPrettyPrinter) newlineIndent() {
	p.buf.WriteByte('\n')
	for range p.depth {
		p.buf.WriteString("  ")
	}
}

// printValue writes the JSON value at the current position (after
// skipping leading whitespace) and reports whether one was found.
func (p *jsonPrettyPrinter) printValue() bool {
	p.skipWS()
	switch p.peek() {
	case '{':
		return p.printObject()
	case '[':
		return p.printArray()
	case '"':
		s, ok := p.scanString()
		if !ok {
			return false
		}
		p.write(ansiString, s)
		return true
	case 't':
		return p.printLiteral("true", ansiBool)
	case 'f':
		return p.printLiteral("false", ansiBool)
	case 'n':
		return p.printLiteral("null", ansiNull)
	default:
		return p.printNumber()
	}
}

func (p *jsonPrettyPrinter) printLiteral(word, color string) bool {
	end := p.pos + len(word)
	if end > len(p.src) || string(p.src[p.pos:end]) != word {
		return false
	}
	p.write(color, word)
	p.pos = end
	return true
}

// printNumber scans the maximal run of characters a JSON number literal
// may contain; the caller reports invalid JSON as "no value found" by
// comparing pos before and after.
func (p *jsonPrettyPrinter) printNumber() bool {
	start := p.pos
	for p.pos < len(p.src) {
		switch c := p.src[p.pos]; {
		case c >= '0' && c <= '9', c == '-', c == '+', c == '.', c == 'e', c == 'E':
			p.pos++
		default:
			goto done
		}
	}
done:
	if p.pos == start {
		return false
	}
	p.write(ansiNumber, string(p.src[start:p.pos]))
	return true
}

// scanString consumes a JSON string literal (quotes included) and returns
// its exact source text, respecting backslash escapes.
func (p *jsonPrettyPrinter) scanString() (string, bool) {
	if p.peek() != '"' {
		return "", false
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\\' {
			p.pos += 2
			continue
		}
		p.pos++
		if c == '"' {
			return string(p.src[start:p.pos]), true
		}
	}
	return "", false
}

func (p *jsonPrettyPrinter) printPunct(c byte) bool {
	if p.peek() != c {
		return false
	}
	p.write(ansiPunct, string(c))
	p.pos++
	return true
}

func (p *jsonPrettyPrinter) printObject() bool {
	if p.peek() != '{' {
		return false
	}
	p.pos++
	p.skipWS()
	if p.peek() == '}' {
		p.pos++
		p.write(ansiPunct, "{}") // an empty container is one token
		return true
	}
	p.write(ansiPunct, "{")
	p.depth++
	for {
		p.newlineIndent()
		p.skipWS()
		key, ok := p.scanString()
		if !ok {
			return false
		}
		p.write(ansiKey, key)
		p.skipWS()
		if !p.printPunct(':') {
			return false
		}
		p.buf.WriteByte(' ')
		if !p.printValue() {
			return false
		}
		p.skipWS()
		if p.peek() != ',' {
			break
		}
		p.printPunct(',')
		p.skipWS()
	}
	p.depth--
	p.newlineIndent()
	return p.printPunct('}')
}

func (p *jsonPrettyPrinter) printArray() bool {
	if p.peek() != '[' {
		return false
	}
	p.pos++
	p.skipWS()
	if p.peek() == ']' {
		p.pos++
		p.write(ansiPunct, "[]") // an empty container is one token
		return true
	}
	p.write(ansiPunct, "[")
	p.depth++
	for {
		p.newlineIndent()
		if !p.printValue() {
			return false
		}
		p.skipWS()
		if p.peek() != ',' {
			break
		}
		p.printPunct(',')
		p.skipWS()
	}
	p.depth--
	p.newlineIndent()
	return p.printPunct(']')
}
