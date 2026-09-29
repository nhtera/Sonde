// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"bytes"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/syntaxedit"
)

// Request is an entry of a request file, for the tree and the index.
type Request struct {
	File   string `json:"file"`
	Entry  int    `json:"entry"` // 1-based
	Method string `json:"method"`
	URL    string `json:"url"`
	// Title is the comment right above the request, if any.
	Title string `json:"title,omitempty"`
	Line  int    `json:"line"` // 1-based
}

// requestsOf lists the entries of src (file is its project path). A file
// that does not parse lists nothing.
func requestsOf(file string, src []byte) []Request {
	models, err := syntaxedit.Model(file, src)
	if err != nil {
		return nil
	}
	lines := lineStarts(src)
	out := make([]Request, 0, len(models))
	for _, m := range models {
		line := lineOfUTF16(src, lines, m.Range.Start)
		out = append(out, Request{
			File: file, Entry: m.Index, Method: m.Method, URL: m.URL,
			Title: titleAbove(src, lines, line), Line: line,
		})
	}
	return out
}

// lineStarts returns the byte offset of each line.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// lineOfUTF16 converts a UTF-16 offset of src to a 1-based line.
func lineOfUTF16(src []byte, starts []int, offset int) int {
	units, i := 0, 0
	for i < len(src) && units < offset {
		r, size := utf8.DecodeRune(src[i:])
		units += utf16.RuneLen(r)
		if units > offset {
			break
		}
		i += size
	}
	line := 1
	for line < len(starts) && starts[line] <= i {
		line++
	}
	return line
}

// titleAbove is the comment on the line right above line, without its #.
func titleAbove(src []byte, starts []int, line int) string {
	if line < 2 {
		return ""
	}
	start, end := starts[line-2], starts[line-1]
	text := strings.TrimSpace(string(bytes.TrimRight(src[start:end], "\r\n")))
	if !strings.HasPrefix(text, "#") {
		return ""
	}
	return strings.TrimSpace(strings.TrimLeft(text, "#"))
}
