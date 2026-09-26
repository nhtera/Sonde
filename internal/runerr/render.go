// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runerr

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/styled"
	"github.com/nhtera/sonde/internal/syntax"
)

// Render formats the error for display: the description, the location, the
// first line of the entry when it is not the error line (entryLine 0 omits
// it), the source line and the message, all with a line-number gutter.
func (e *Error) Render(filename, content string, entryLine int) string {
	return e.render(filename, content, entryLine).String(false)
}

// RenderColor is Render with ANSI colors: a blue gutter, the entry line
// in gray and the message in red (a body diff in red and green).
func (e *Error) RenderColor(filename, content string, entryLine int) string {
	return e.render(filename, content, entryLine).String(true)
}

func (e *Error) render(filename, content string, entryLine int) styled.Text {
	lines := syntax.SourceLines(content)
	width := max(len(strconv.Itoa(len(lines))), 2)
	spaces := strings.Repeat(" ", width)
	var prefix styled.Text
	prefix.Push(spaces+" |", styled.Bold|styled.Blue)
	line, col := e.Span.Start.Line, e.Span.Start.Col

	var t styled.Text
	t.Push(e.Description(), styled.Bold)
	t.Push("\n"+spaces, styled.Plain)
	t.Push("-->", styled.Bold|styled.Blue)
	t.Push(fmt.Sprintf(" %s:%d:%d\n", filename, line, col), styled.Plain)
	t.Append(prefix)
	if entryLine > 0 {
		if entryLine != line {
			t.Push("\n", styled.Plain)
			t.Append(prefix)
			t.Push(" ", styled.Plain)
			t.Push(untab(lineAt(lines, entryLine)), styled.Gray)
		}
		if line-entryLine > 1 {
			t.Push("\n", styled.Plain)
			t.Append(prefix)
			t.Push(" ...", styled.Gray)
		}
	}

	if e.Kind == AssertBodyDiff {
		t.Append(e.renderDiff(lines, width, prefix))
		return t
	}
	var msg styled.Text
	msg.Push(" "+untab(lineAt(lines, line))+"\n", styled.Plain)
	for i, l := range strings.Split(e.fixme(lineAt(lines, line)), "\n") {
		if i > 0 {
			msg.Push("\n", styled.Plain)
		}
		msg.Push(l, styled.Bold|styled.Red)
	}
	var numbered styled.Text
	numbered.Push(fmt.Sprintf("%*d |", width, line), styled.Bold|styled.Blue)
	for i, l := range msg.Split("\n") {
		t.Push("\n", styled.Plain)
		if i == 0 {
			t.Append(numbered)
		} else {
			t.Append(prefix)
		}
		t.Append(l)
	}
	if !t.HasSuffix("|") {
		t.Push("\n", styled.Plain)
		t.Append(prefix)
	}
	return t
}

// renderDiff shows the first differing body line and the diff hunk, with
// removed lines in red and added lines in green.
func (e *Error) renderDiff(lines []string, width int, prefix styled.Text) styled.Text {
	var t styled.Text
	line := e.Span.Start.Line
	t.Push("\n", styled.Plain)
	t.Push(fmt.Sprintf("%*d | %s\n", width, line, lineAt(lines, line)), styled.Bold|styled.Blue)
	for i, l := range strings.Split(e.Reason, "\n") {
		if i > 0 {
			t.Push("\n", styled.Plain)
		}
		t.Append(prefix)
		if l == "" {
			continue
		}
		t.Push("   ", styled.Plain)
		switch l[0] {
		case '-':
			t.Push(l, styled.Red)
		case '+':
			t.Push(l, styled.Green)
		default:
			t.Push(l, styled.Plain)
		}
	}
	return t
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
