// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"regexp"
	"strings"
)

// scriptNeverExecuted matches the marker line an importer writes above a
// kept script, e.g. "prerequest script (never executed):" (see
// internal/convert/postman/auth.go's eventPhase/scriptComments).
var scriptNeverExecuted = regexp.MustCompile(`^\S+ script \(never executed\):$`)

// hoverComment explains a comment an importer left in place of code it
// could not run or translate: a prerequest/test script (Postman) or an
// OpenCollection script, assertion or action (see
// internal/convert/opencollection/scripts.go's buildRuntime). It works
// directly on the line's text, so it applies even to an entry that does not
// parse yet.
func (s *Server) hoverComment(d *document, offset int) *Hover {
	line := int(d.lines.position(offset).Line)
	lineStart, lineEnd := d.lines.starts[line], d.lines.lineEnd(line)
	text := d.text[lineStart:lineEnd]
	trimmed := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		return nil
	}
	body := strings.TrimPrefix(strings.TrimPrefix(trimmed, "#"), " ")
	var msg string
	switch {
	case scriptNeverExecuted.MatchString(body):
		msg = "Kept as a comment by the importer: Sonde does not run pre-request or test scripts, " +
			"so the original script is preserved here instead of being executed."
	case strings.HasPrefix(body, "opencollection assertion:"):
		msg = "Kept as a comment by the importer: Sonde could not map this OpenCollection assertion " +
			"to a [Asserts] predicate, so it is preserved here instead of being translated."
	case strings.HasPrefix(body, "opencollection "):
		msg = "Kept as a comment by the importer: Sonde does not run OpenCollection scripts or actions, " +
			"so the original is preserved here instead of being executed."
	default:
		return nil
	}
	rng := d.lines.rangeOf(lineStart, lineEnd)
	return &Hover{Contents: *s.markup(msg), Range: &rng}
}
