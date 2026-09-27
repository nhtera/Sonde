// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"sort"
	"strings"

	"github.com/nhtera/sonde/internal/docs"
	"github.com/nhtera/sonde/internal/syntax"
)

// notDoc and redactDoc document the two bare keywords that are not table
// entries: `not` before a predicate and `redact` after a capture.
const (
	notDoc    = "Negates the predicate that follows."
	redactDoc = "Marks the captured value as a secret: Sonde never prints, logs it or lets it appear in an assertion's expected value."
)

// queryArgHints names the placeholder text used in a snippet for queries
// that take an argument, keyed by query name.
var queryArgHints = map[string]string{
	"header": "Name", "cookie": "name", "xpath": "expr", "jsonpath": "$.expr",
	"regex": "pattern", "variable": "name", "certificate": "Subject",
}

// optionShape classifies an [Options] value for value-hint completion. It
// mirrors internal/syntax/parse_option.go's unexported optionShapes, which
// this package cannot import; completion_sections_test.go's
// TestOptionShapesMatchTable and TestOptionShapeSamplesParse guard against
// drift between the two copies and against drift from docs.Table.Options.
type optionShape int

const (
	shapeString optionShape = iota
	shapeFilename
	shapeFilenamePassword
	shapeBoolean
	shapeNatural
	shapeCount
	shapeDuration
	shapeVariable
	shapeVerbosity
)

var optionShapes = map[string]optionShape{
	"aws-sigv4": shapeString, "cacert": shapeFilename, "cert": shapeFilenamePassword,
	"compressed": shapeBoolean, "connect-to": shapeString, "connect-timeout": shapeDuration,
	"delay": shapeDuration, "digest": shapeBoolean, "insecure": shapeBoolean, "header": shapeString,
	"http1.0": shapeBoolean, "http1.1": shapeBoolean, "http2": shapeBoolean, "http3": shapeBoolean,
	"ipv4": shapeBoolean, "ipv6": shapeBoolean, "key": shapeFilename, "limit-rate": shapeNatural,
	"location": shapeBoolean, "location-trusted": shapeBoolean, "max-redirs": shapeCount,
	"max-time": shapeDuration, "negotiate": shapeBoolean, "netrc": shapeBoolean,
	"netrc-file": shapeString, "netrc-optional": shapeBoolean, "ntlm": shapeBoolean,
	"output": shapeFilename, "path-as-is": shapeBoolean, "pinnedpubkey": shapeString,
	"proxy": shapeString, "repeat": shapeCount, "resolve": shapeString, "retry": shapeCount,
	"retry-interval": shapeDuration, "skip": shapeBoolean, "unix-socket": shapeString,
	"user": shapeString, "variable": shapeVariable, "verbose": shapeBoolean,
	"verbosity": shapeVerbosity, "very-verbose": shapeBoolean,
	// Sonde extensions, .sonde files only.
	"sonde-stream-count": shapeNatural, "sonde-stream-max-bytes": shapeNatural,
	"sonde-stream-timeout": shapeDuration,
}

// sonde reports whether d accepts the Sonde extensions.
func sonde(d *document) bool { return d.dialect == syntax.DialectSonde }

// completeSectionName proposes section names right after "[" at the start
// of a line: request sections, or response sections once the response's
// status line has been seen.
func (s *Server) completeSectionName(d *document, line, lineStart int, text string, bodyStart, col int) []CompletionItem {
	rng := d.lines.rangeOf(lineStart+bodyStart, lineStart+col)
	closeBracket := !strings.HasPrefix(text[col:], "]")
	m := precedingMarker(d, line)
	response := m.kind == markerStatus || (m.kind == markerSection && (m.section == "Captures" || m.section == "Asserts"))
	names := requestSectionNames
	if response {
		names = responseSectionNames
	} else if sonde(d) {
		names = append(append([]string(nil), names...), "SondeMessages")
	}
	items := make([]CompletionItem, len(names))
	for i, n := range names {
		newText := n
		if closeBracket {
			newText += "]"
		}
		items[i] = CompletionItem{
			Label: n, Kind: kindModule, Detail: "section",
			TextEdit: &TextEdit{Range: rng, NewText: newText}, FilterText: n, SortText: n,
		}
	}
	return items
}

// completeAssertOrCapture proposes the token at offset in an [Asserts] or
// [Captures] line: the query name first, then filters and (a predicate and
// optional leading `not`, or `redact`).
func (s *Server) completeAssertOrCapture(d *document, lineStart, lineEnd, offset int, capture bool) []CompletionItem {
	bodyLo := lineStart
	if capture {
		colon := strings.IndexByte(d.text[lineStart:lineEnd], ':')
		if colon < 0 {
			return []CompletionItem{} // still typing the capture name: free text
		}
		colonOffset := lineStart + colon
		if offset <= colonOffset {
			return []CompletionItem{}
		}
		bodyLo = colonOffset + 1
	}
	segs := segments(d.text, bodyLo, lineEnd)
	pos := segmentPosition(segs, offset)
	rng := d.lines.rangeOf(replaceStart(segs, pos, offset), offset)
	if pos == 0 {
		return s.queryItems(rng, sonde(d))
	}
	items := s.filterItems(rng)
	if capture {
		return append(items, s.keywordItem("redact", redactDoc, rng))
	}
	if segs[pos-1].text != "not" {
		items = append(items, s.keywordItem("not", notDoc, rng))
	}
	return append(items, s.predicateItems(rng)...)
}

// completeOption proposes an [Options] name before the ':', or a value hint
// after it.
func (s *Server) completeOption(d *document, lineStart, lineEnd, offset int, text string) []CompletionItem {
	colon := strings.IndexByte(text, ':')
	if colon < 0 || offset <= lineStart+colon {
		start, end := identRange(d.text, offset, lineStart, lineEnd, isOptionNameByte)
		rng := d.lines.rangeOf(start, end)
		items := make([]CompletionItem, 0, len(s.table.Options))
		for _, e := range s.table.Options {
			items = append(items, s.tableItem(e, kindProperty, rng, e.Name+": "))
		}
		if sonde(d) {
			for _, e := range s.table.Sonde.Options {
				items = append(items, s.tableItem(e, kindProperty, rng, e.Name+": "))
			}
		}
		return items
	}
	name := strings.TrimSpace(text[:colon])
	notSpace := func(c byte) bool { return c != ' ' && c != '\t' }
	start, end := identRange(d.text, offset, lineStart+colon+1, lineEnd, notSpace)
	return s.optionValueItems(name, d.lines.rangeOf(start, end))
}

func (s *Server) optionValueItems(name string, rng Range) []CompletionItem {
	switch optionShapes[name] {
	case shapeBoolean:
		return []CompletionItem{s.keywordItem("true", "", rng), s.keywordItem("false", "", rng)}
	case shapeDuration:
		items := make([]CompletionItem, 0, 4)
		for _, v := range []string{"500ms", "2s", "30s", "1m"} {
			items = append(items, s.keywordItem(v, "A natural number followed by ms, s, m or h.", rng))
		}
		return items
	case shapeVerbosity:
		items := make([]CompletionItem, 0, 3)
		for _, v := range []string{"brief", "verbose", "debug"} {
			items = append(items, s.keywordItem(v, "", rng))
		}
		return items
	}
	return []CompletionItem{}
}

// completeVariable proposes, inside {{ }}: variables visible at offset
// (this document's captures and [Options] definitions, then the project's
// environment/variables/secrets files and process environment), followed
// by the template functions. No variable's value is ever included.
func (s *Server) completeVariable(d *document, offset int) ([]CompletionItem, bool) {
	inside, start, end := placeholderAt(d, offset)
	if !inside {
		return nil, false
	}
	rng := d.lines.rangeOf(start, end)
	items := []CompletionItem{}
	seen := map[string]bool{}

	defs := visibleAt(definitions(d.file), offset)
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		items = append(items, s.variableItem(d, name, defs[name], rng))
		seen[name] = true
	}

	pv := s.config.variables(d)
	extNames := make([]string, 0, len(pv.names))
	for name := range pv.names {
		if !seen[name] {
			extNames = append(extNames, name)
		}
	}
	sort.Strings(extNames)
	for _, name := range extNames {
		items = append(items, externalVariableItem(name, pv.names[name], pv, rng))
	}

	for _, e := range s.table.Functions {
		items = append(items, s.tableItem(e, kindFunction, rng, e.Name))
	}
	return items, true
}

func (s *Server) variableItem(d *document, name string, def definition, rng Range) CompletionItem {
	return CompletionItem{
		Label: name, Kind: kindVariable, Detail: variableSourceText(d, def, false), // Detail is always plain text
		TextEdit: &TextEdit{Range: rng, NewText: name}, FilterText: name, SortText: name,
	}
}

func externalVariableItem(name string, src varSource, pv *projectVars, rng Range) CompletionItem {
	return CompletionItem{
		Label: name, Kind: kindVariable, Detail: externalSourceText(src, pv),
		TextEdit: &TextEdit{Range: rng, NewText: name}, FilterText: name, SortText: name,
	}
}

func (s *Server) queryItems(rng Range, sonde bool) []CompletionItem {
	items := make([]CompletionItem, 0, len(s.table.Queries))
	for _, e := range s.table.Queries {
		items = append(items, s.queryItem(e, rng))
	}
	if sonde {
		for _, e := range s.table.Sonde.Queries {
			items = append(items, s.tableItem(e, kindProperty, rng, e.Name))
		}
	}
	return items
}

// completeStep proposes a [SondeMessages] step name at the start of a line.
func (s *Server) completeStep(d *document, lineStart, offset int, text string, col int) []CompletionItem {
	start, ok := atFirstToken(text, col)
	if !ok {
		return []CompletionItem{}
	}
	rng := d.lines.rangeOf(lineStart+start, offset)
	items := make([]CompletionItem, 0, len(s.table.Sonde.Steps))
	for _, e := range s.table.Sonde.Steps {
		insert := e.Name
		if e.Name == "send" {
			insert += ": "
		}
		items = append(items, s.tableItem(e, kindKeyword, rng, insert))
	}
	return items
}

// queryItem snippets a placeholder argument for queries that take one, when
// the client supports snippets; otherwise it inserts the keyword and the
// opening quote, leaving the rest to the user.
func (s *Server) queryItem(e docs.Entry, rng Range) CompletionItem {
	hint, hasArg := queryArgHints[e.Name]
	if !hasArg {
		return s.tableItem(e, kindProperty, rng, e.Name)
	}
	item := s.tableItem(e, kindProperty, rng, e.Name+` "`)
	if s.snippets {
		item.InsertTextFormat = formatSnippet
		item.TextEdit.NewText = e.Name + ` "${1:` + hint + `}"$0`
	}
	return item
}

func (s *Server) filterItems(rng Range) []CompletionItem {
	items := make([]CompletionItem, 0, len(s.table.Filters))
	for _, e := range s.table.Filters {
		items = append(items, s.tableItem(e, kindFunction, rng, e.Name))
	}
	return items
}

func (s *Server) predicateItems(rng Range) []CompletionItem {
	items := make([]CompletionItem, 0, len(s.table.Predicates))
	for _, e := range s.table.Predicates {
		items = append(items, s.tableItem(e, kindOperator, rng, e.Name))
	}
	return items
}

func (s *Server) keywordItem(name, doc string, rng Range) CompletionItem {
	return CompletionItem{
		Label: name, Kind: kindKeyword, Documentation: s.markup(doc),
		TextEdit: &TextEdit{Range: rng, NewText: name}, FilterText: name, SortText: name,
	}
}
