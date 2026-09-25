// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runerr

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// Render formats the error for display: the description, the location, the
// first line of the entry when it is not the error line (entryLine 0 omits
// it), the source line and the message, all with a line-number gutter.
func (e *Error) Render(filename, content string, entryLine int) string {
	lines := syntax.SourceLines(content)
	width := max(len(strconv.Itoa(len(lines))), 2)
	spaces := strings.Repeat(" ", width)
	prefix := spaces + " |"
	line, col := e.Span.Start.Line, e.Span.Start.Col

	var b strings.Builder
	b.WriteString(e.Description() + "\n")
	fmt.Fprintf(&b, "%s--> %s:%d:%d\n", spaces, filename, line, col)
	b.WriteString(prefix)
	if entryLine > 0 {
		if entryLine != line {
			b.WriteString("\n" + prefix + " " + untab(lineAt(lines, entryLine)))
		}
		if line-entryLine > 1 {
			b.WriteString("\n" + prefix + " ...")
		}
	}

	text := " " + untab(lineAt(lines, line)) + "\n" + e.fixme(lineAt(lines, line))
	out := ""
	for i, l := range strings.Split(text, "\n") {
		if i == 0 {
			out += fmt.Sprintf("\n%*d |%s", width, line, l)
		} else {
			out += "\n" + prefix + l
		}
	}
	if !strings.HasSuffix(out, "|") {
		out += "\n" + prefix
	}
	b.WriteString(out)
	return b.String()
}

// fixme is the message under the source line, pointed by carets except for
// assert failures.
func (e *Error) fixme(line string) string {
	if e.Kind == AssertFailure {
		return e.Message()
	}
	return e.carets(line) + indentTail(e.Message(), len(e.carets(line)))
}

// carets underlines the error span on its first line; each tab before the
// column is displayed as four spaces.
func (e *Error) carets(line string) string {
	col := e.Span.Start.Col
	width := 1
	if e.Span.End.Col > col {
		width = e.Span.End.Col - col
	}
	tabs := 0
	for i, c := range []rune(line) {
		if i >= col-1 {
			break
		}
		if c == '\t' {
			tabs++
		}
	}
	return strings.Repeat(" ", max(col+tabs*3, 0)) + strings.Repeat("^", width) + " "
}

// indentTail indents every non-empty line after the first by n spaces.
func indentTail(msg string, n int) string {
	parts := strings.Split(msg, "\n")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.Repeat(" ", n) + parts[i]
		}
	}
	return strings.Join(parts, "\n")
}

func lineAt(lines []string, n int) string {
	if n >= 1 && n <= len(lines) {
		return lines[n-1]
	}
	return ""
}

func untab(s string) string { return strings.ReplaceAll(s, "\t", "    ") }
