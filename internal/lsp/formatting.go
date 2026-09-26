// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import "github.com/nhtera/sonde/internal/syntax"

// format returns the edits that give d its canonical layout: none when it
// is already formatted or does not parse (formatting a partial parse would
// drop the broken entries).
func (s *Server) format(d *document) []TextEdit {
	if len(d.errs) > 0 {
		return []TextEdit{}
	}
	out := string(syntax.Format(d.file))
	if out == d.text {
		return []TextEdit{}
	}
	return []TextEdit{{Range: d.lines.rangeOf(0, len(d.text)), NewText: out}}
}
