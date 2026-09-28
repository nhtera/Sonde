// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/docs"
)

// completionTriggers are the characters that open completion by themselves.
// A space is deliberately not one: every request/response line has a fixed
// shape recovered from the raw text (see completion_context.go), so
// completion is just as accurate on an explicit Ctrl+Space as on a trigger
// character, and a space trigger would pop the menu constantly while typing
// query and filter arguments.
var completionTriggers = []string{"[", "{"}

// httpMethods are offered at the start of a new entry.
var httpMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE"}

// commonHeaders are offered at the start of a header line, in both the
// request and the response part.
var commonHeaders = []string{
	"Accept", "Accept-Encoding", "Accept-Language", "Authorization", "Cache-Control",
	"Connection", "Content-Disposition", "Content-Length", "Content-Type", "Cookie",
	"ETag", "Host", "If-Match", "If-None-Match", "Location", "Origin", "Referer",
	"Set-Cookie", "User-Agent", "X-Requested-With",
}

// requestSectionNames and responseSectionNames are the canonical section
// names offered after "[" (aliases like QueryStringParams are accepted by
// the parser but not proposed).
var requestSectionNames = []string{"Query", "Form", "Multipart", "Cookies", "BasicAuth", "Options"}
var responseSectionNames = []string{"Captures", "Asserts"}

// completion returns the proposals at offset in d. The result is never nil:
// LSP requires textDocument/completion to answer with an item array, never
// null, even when there is nothing to propose.
func (s *Server) completion(d *document, offset int) []CompletionItem {
	if items, ok := s.completeVariable(d, offset); ok {
		return items
	}
	line := int(d.lines.position(offset).Line)
	lineStart, lineEnd := d.lines.starts[line], d.lines.lineEnd(line)
	text := d.text[lineStart:lineEnd]
	col := offset - lineStart

	if bodyStart, ok := bracketContext(text, col); ok {
		return s.completeSectionName(d, line, lineStart, text, bodyStart, col)
	}

	m := precedingMarker(d, line)
	if m.kind == markerSection {
		switch m.section {
		case "Asserts":
			return s.completeAssertOrCapture(d, lineStart, lineEnd, offset, false)
		case "Captures":
			return s.completeAssertOrCapture(d, lineStart, lineEnd, offset, true)
		case "Options":
			return s.completeOption(d, lineStart, lineEnd, offset, text)
		case "SondeMessages":
			return s.completeStep(d, lineStart, offset, text, col)
		case "SondeGrpc":
			return s.completeGrpcKey(d, lineStart, offset, text, col)
		}
		return []CompletionItem{}
	}
	if start, ok := atFirstToken(text, col); ok {
		rng := d.lines.rangeOf(lineStart+start, offset)
		switch m.kind {
		case markerNone:
			return s.methodItems(rng)
		case markerMethod:
			return append(s.headerItems(rng), s.httpLineItem(rng))
		case markerStatus:
			return s.headerItems(rng)
		}
	}
	return []CompletionItem{}
}

func (s *Server) methodItems(rng Range) []CompletionItem {
	items := make([]CompletionItem, len(httpMethods))
	for i, m := range httpMethods {
		items[i] = CompletionItem{
			Label: m, Kind: kindMethod, Detail: "HTTP method",
			TextEdit: &TextEdit{Range: rng, NewText: m}, FilterText: m, SortText: m,
		}
	}
	return items
}

func (s *Server) headerItems(rng Range) []CompletionItem {
	items := make([]CompletionItem, len(commonHeaders))
	for i, h := range commonHeaders {
		items[i] = CompletionItem{
			Label: h, Kind: kindField, Detail: "header",
			TextEdit: &TextEdit{Range: rng, NewText: h + ": "}, FilterText: h, SortText: h,
		}
	}
	return items
}

// httpLineItem begins the response, once the request headers/sections are
// done, without a section: the "HTTP" version line.
func (s *Server) httpLineItem(rng Range) CompletionItem {
	return CompletionItem{
		Label: "HTTP", Kind: kindKeyword, Detail: "begins the expected response",
		TextEdit: &TextEdit{Range: rng, NewText: "HTTP/1.1 200"}, FilterText: "HTTP", SortText: "HTTP",
	}
}

// tableItem builds a completion proposal for a docs.Table entry (a query,
// filter, predicate, function or option name), replacing rng with name.
func (s *Server) tableItem(e docs.Entry, kind int, rng Range, name string) CompletionItem {
	item := CompletionItem{
		Label:         e.Name,
		Kind:          kind,
		Detail:        entryDetail(e),
		Documentation: s.markup(entryDoc(e)),
		FilterText:    e.Name,
		SortText:      e.Name,
		TextEdit:      &TextEdit{Range: rng, NewText: name},
	}
	if e.Deprecated != "" {
		item.Tags = []int{tagDeprecated}
		item.SortText = "~" + e.Name // sorts after every non-deprecated entry
	}
	return item
}

// entryDetail is the one-line status note shown next to a table entry, "" for
// a plain supported entry with no replacement.
func entryDetail(e docs.Entry) string {
	switch e.Status {
	case docs.StatusUnsupported:
		return "not supported yet: " + e.Reason
	case docs.StatusPartial:
		return "partially supported: " + e.Reason
	case docs.StatusPlanned:
		return fmt.Sprintf("planned, phase %d", e.Phase)
	}
	if e.Deprecated != "" {
		return "deprecated: use " + e.Deprecated
	}
	return ""
}

// entryDoc is the full hover/documentation text for a table entry.
func entryDoc(e docs.Entry) string {
	var b strings.Builder
	b.WriteString(e.Doc)
	switch e.Status {
	case docs.StatusUnsupported:
		fmt.Fprintf(&b, "\n\nSonde does not support this yet: %s.", e.Reason)
	case docs.StatusPartial:
		fmt.Fprintf(&b, "\n\nPartial support: %s.", e.Reason)
	case docs.StatusPlanned:
		fmt.Fprintf(&b, "\n\nPlanned for phase %d.", e.Phase)
	}
	if e.Deprecated != "" {
		fmt.Fprintf(&b, "\n\nDeprecated: use %s instead.", e.Deprecated)
	}
	return b.String()
}

// markup wraps text as markdown when the client accepts it, plaintext
// otherwise.
func (s *Server) markup(text string) *MarkupContent {
	if text == "" {
		return nil
	}
	kind := markupPlainText
	if s.markdown {
		kind = markupMarkdown
	}
	return &MarkupContent{Kind: kind, Value: text}
}
