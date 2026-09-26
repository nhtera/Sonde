// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"unicode"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/syntax"
)

// source names Sonde in every diagnostic.
const source = "sonde"

// diagnostics returns d's parse errors followed by its semantic warnings.
func (s *Server) diagnostics(d *document) []Diagnostic {
	out := make([]Diagnostic, 0, len(d.errs))
	for _, e := range d.errs {
		out = append(out, Diagnostic{
			Range:    errorRange(d, e),
			Severity: severityError,
			Source:   source,
			Message:  e.Description() + ": " + e.Message(),
		})
	}
	return append(out, s.semanticDiagnostics(d)...)
}

// errorRange underlines the word the parser stopped at, or one character
// when it stopped on a space; at the end of a line or file the range is
// empty.
func errorRange(d *document, e *syntax.Error) Range {
	start := min(e.Pos.Offset, len(d.text))
	end := start
	for end < len(d.text) {
		c, size := utf8.DecodeRuneInString(d.text[end:])
		if c == '\n' || c == '\r' || (end > start && unicode.IsSpace(c)) {
			break
		}
		end += size
		if unicode.IsSpace(c) {
			break
		}
	}
	return d.lines.rangeOf(start, end)
}
