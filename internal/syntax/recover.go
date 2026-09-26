// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"unicode/utf8"
)

// ParseAll parses src like Parse but does not stop at the first error: after
// a failure it resumes at the next line that starts with an HTTP method, so
// every broken entry gets its own error. The returned File holds the entries
// that parsed (LineTerminators only when the file parsed to its end); errs
// holds at most limit errors, in source order. The first error is always the
// one Parse would return.
func ParseAll(name string, src []byte, d Dialect, limit int) (f *File, errs []*Error) {
	_ = name
	_ = d
	f = &File{}
	if len(src) > MaxFileSize {
		return f, []*Error{errAt(Pos{Line: 1, Col: 1}, false, ErrFileTooLarge, "64 MiB")}
	}
	if !utf8.Valid(src) {
		return f, []*Error{errAt(invalidUTF8Pos(src), false, ErrInvalidUTF8, "")}
	}
	r := newReader(string(src))
	if strings.HasPrefix(r.src, utf8BOM) {
		f.BOM = true
		r.pos.Offset = len(utf8BOM)
	}
	for limit <= 0 || len(errs) < limit {
		err := entriesToEOF(r, f)
		if err == nil {
			return f, errs
		}
		errs = append(errs, err)
		from := err.Pos
		if from.Offset < r.pos.Offset {
			from = r.pos
		}
		next, ok := nextMethodLine(r.src, from.Offset)
		if !ok {
			return f, errs
		}
		r.pos = advance(r.src, from, next)
	}
	return f, errs
}

// entriesToEOF appends the entries it parses from r to f and then reads the
// trailing blank lines to EOF, exactly as hurlFile does.
func entriesToEOF(r *reader, f *File) *Error {
	for !r.isEOF() {
		save := r.pos
		e, err := entry(r)
		if err != nil {
			if !err.recoverable {
				return err
			}
			r.pos = save
			break
		}
		f.Entries = append(f.Entries, e)
	}
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return err
	}
	if err := eof(r); err != nil {
		return err
	}
	f.LineTerminators = lts
	return nil
}

// nextMethodLine returns the offset of the first line after the one holding
// offset whose first word (after spaces) is an uppercase method name other
// than HTTP followed by a space.
func nextMethodLine(src string, offset int) (int, bool) {
	for {
		nl := strings.IndexByte(src[offset:], '\n')
		if nl < 0 {
			return 0, false
		}
		offset += nl + 1
		if IsMethodLine(src[offset:]) {
			return offset, true
		}
	}
}

// IsMethodLine reports whether s starts, after spaces and tabs, with an
// uppercase method name other than HTTP followed by a space or tab: the
// first line of a request.
func IsMethodLine(s string) bool {
	s = strings.TrimLeft(s, " \t")
	n := 0
	for n < len(s) && s[n] >= 'A' && s[n] <= 'Z' {
		n++
	}
	return n > 0 && n < len(s) && (s[n] == ' ' || s[n] == '\t') && s[:n] != "HTTP"
}

// advance returns the position of offset, reading forward from p.
func advance(src string, p Pos, offset int) Pos {
	r := &reader{src: src, end: offset, pos: p}
	for !r.isEOF() {
		r.read()
	}
	return r.pos
}
