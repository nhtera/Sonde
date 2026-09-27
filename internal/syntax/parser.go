// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Dialect selects the accepted language: .hurl files are strictly the
// standard grammar; .sonde files add the Sonde-prefixed extensions of
// docs/decisions/0004-streaming-protocols.md ([SondeMessages],
// sonde-stream-* options, the sondeStream query).
type Dialect int

// Dialects.
const (
	DialectHurl Dialect = iota
	DialectSonde
)

// DialectFor picks the dialect from a file name's extension.
func DialectFor(name string) Dialect {
	if strings.EqualFold(filepath.Ext(name), ".sonde") {
		return DialectSonde
	}
	return DialectHurl
}

const utf8BOM = "\uFEFF"

// MaxFileSize is the largest source Parse accepts.
const MaxFileSize = 64 << 20

// Parse parses src. name is used only by callers for diagnostics. It returns
// the first syntax error as *Error.
func Parse(name string, src []byte, d Dialect) (*File, error) {
	_ = name
	if len(src) > MaxFileSize {
		return nil, errAt(Pos{Line: 1, Col: 1}, false, ErrFileTooLarge, "64 MiB")
	}
	if !utf8.Valid(src) {
		return nil, errAt(invalidUTF8Pos(src), false, ErrInvalidUTF8, "")
	}
	r := newReader(string(src), d)
	bom := strings.HasPrefix(r.src, utf8BOM)
	if bom {
		r.pos.Offset = len(utf8BOM)
	}
	f, err := hurlFile(r)
	if err != nil {
		return nil, err
	}
	f.BOM = bom
	return f, nil
}

func invalidUTF8Pos(src []byte) Pos {
	r := newReader(string(src), DialectHurl)
	for !r.isEOF() {
		c, size := utf8.DecodeRuneInString(r.src[r.pos.Offset:])
		if c == utf8.RuneError && size == 1 {
			break
		}
		r.read()
	}
	return r.pos
}

func hurlFile(r *reader) (*File, *Error) {
	entries, err := zeroOrMore(r, entry)
	if err != nil {
		return nil, err
	}
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	if err := eof(r); err != nil {
		return nil, err
	}
	return &File{Entries: entries, LineTerminators: lts}, nil
}

func entry(r *reader) (*Entry, *Error) {
	req, err := request(r)
	if err != nil {
		return nil, err
	}
	resp, _, err := optional(r, response)
	if err != nil {
		return nil, err
	}
	return &Entry{Request: req, Response: resp}, nil
}

func request(r *reader) (*Request, *Error) {
	start := r.pos
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	req := &Request{LineTerminators: lts}
	req.Space0, _ = zeroOrMoreSpaces(r)
	if req.Method, err = method(r); err != nil {
		return nil, err
	}
	if req.Space1, err = oneOrMoreSpaces(r); err != nil {
		return nil, err
	}
	if req.URL, err = unquotedTemplate(r); err != nil {
		return nil, err
	}
	if req.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	if req.Headers, err = zeroOrMore(r, keyValue); err != nil {
		return nil, err
	}
	if req.Sections, err = zeroOrMore(r, requestSection); err != nil {
		return nil, err
	}
	if req.Body, _, err = optional(r, body); err != nil {
		return nil, err
	}
	req.Span = Span{start, r.pos}
	if err := checkDuplicatedSections(req.Sections); err != nil {
		return nil, err
	}
	return req, nil
}

func response(r *reader) (*Response, *Error) {
	start := r.pos
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	resp := &Response{LineTerminators: lts}
	resp.Space0, _ = zeroOrMoreSpaces(r)
	if resp.Version, err = version(r); err != nil {
		return nil, err
	}
	if resp.Space1, err = oneOrMoreSpaces(r); err != nil {
		return nil, err
	}
	if resp.Status, err = status(r); err != nil {
		return nil, err
	}
	if resp.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	if resp.Headers, err = zeroOrMore(r, keyValue); err != nil {
		return nil, err
	}
	if resp.Sections, err = zeroOrMore(r, responseSection); err != nil {
		return nil, err
	}
	if resp.Body, _, err = optional(r, body); err != nil {
		return nil, err
	}
	resp.Span = Span{start, r.pos}
	if err := checkDuplicatedSections(resp.Sections); err != nil {
		return nil, err
	}
	return resp, nil
}

func checkDuplicatedSections(sections []*Section) *Error {
	seen := map[string]bool{}
	for _, s := range sections {
		if seen[s.Name] {
			return errAt(s.Span.Start, false, ErrDuplicateSection, "")
		}
		seen[s.Name] = true
	}
	return nil
}

func method(r *reader) (Method, *Error) {
	start := r.pos
	if r.isEOF() {
		return Method{}, errAt(start, true, ErrMethod, "<EOF>")
	}
	name := r.readWhile(func(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' })
	if name == "" || strings.ToUpper(name) != name {
		return Method{}, errAt(start, false, ErrMethod, name)
	}
	return Method{Value: name, Span: Span{start, r.pos}}, nil
}

func version(r *reader) (Version, *Error) {
	start := r.pos
	if err := tryLiteral(r, "HTTP"); err != nil {
		return Version{}, err
	}
	c, _ := r.peek()
	switch c {
	case '/':
		for _, v := range []string{"/1.0", "/1.1", "/2", "/3"} {
			if r.consume(v) {
				return Version{Value: r.slice(start), Span: Span{start, r.pos}}, nil
			}
		}
	case ' ', '\t':
		return Version{Value: r.slice(start), Span: Span{start, r.pos}}, nil
	}
	return Version{}, errAt(start, false, ErrVersion, "")
}

func status(r *reader) (Status, *Error) {
	start := r.pos
	if !r.consume("*") {
		digits := r.readWhile(isDigit)
		if _, perr := strconv.ParseUint(digits, 10, 64); perr != nil {
			return Status{}, errAt(start, false, ErrStatus, "")
		}
	}
	return Status{Value: r.slice(start), Span: Span{start, r.pos}}, nil
}

func body(r *reader) (*Body, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	b := &Body{LineTerminators: lts}
	b.Space0, _ = zeroOrMoreSpaces(r)
	start := r.pos
	if b.Value, err = bodyBytes(r); err != nil {
		return nil, err
	}
	b.Span = Span{start, r.pos}
	if b.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	return b, nil
}
