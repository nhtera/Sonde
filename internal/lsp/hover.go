// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"fmt"

	"github.com/nhtera/sonde/internal/docs"
	"github.com/nhtera/sonde/internal/syntax"
)

// sectionDocs documents section headers, which are not a docs.Table kind
// (table.yaml only covers queries, filters, predicates, functions and
// options).
var sectionDocs = map[syntax.SectionKind]string{
	syntax.SectionQueryParams: "URL query string parameters added to the request.",
	syntax.SectionFormParams:  "application/x-www-form-urlencoded request body fields.",
	syntax.SectionMultipart:   "multipart/form-data request body parts.",
	syntax.SectionCookies:     "Cookies sent with the request.",
	syntax.SectionBasicAuth:   "HTTP Basic authentication credentials.",
	syntax.SectionOptions:     "Per-entry client options: timeouts, retries, variables and more.",
	syntax.SectionCaptures:    "Variables captured from the response, usable by later entries.",
	syntax.SectionAsserts:     "Assertions checked against the response.",
	syntax.SectionMessages:    "WebSocket steps (.sonde only): send, receive and close, in order.",
	syntax.SectionGrpc:        "Makes the entry a gRPC call (.sonde only): its message types come from proto or protoset files, or from server reflection when the section is empty.",
}

// lookup finds a docs.Table entry of kind, or of the Sonde extensions of
// that kind.
func (s *Server) lookup(kind, name string) (docs.Entry, bool) {
	if e, ok := s.table.Lookup(kind, name); ok {
		return e, true
	}
	return s.table.Lookup("sonde-"+kind, name)
}

// hover returns documentation for the element at offset in d, or nil.
func (s *Server) hover(d *document, offset int) *Hover {
	if h := s.hoverComment(d, offset); h != nil {
		return h
	}
	if h := s.hoverVariable(d, offset); h != nil {
		return h
	}
	return s.hoverKeyword(d, offset)
}

// hoverVariable answers a {{ }} name: its source, or that it is undefined.
// It never shows a value.
func (s *Server) hoverVariable(d *document, offset int) *Hover {
	inside, start, end := placeholderAt(d, offset)
	if !inside || start == end {
		return nil
	}
	name := d.text[start:end]
	rng := d.lines.rangeOf(start, end)
	if e, ok := s.table.Lookup("functions", name); ok {
		return &Hover{Contents: *s.markup(entryDoc(e)), Range: &rng}
	}
	if def, ok := visibleAt(definitions(d.file), offset)[name]; ok {
		return &Hover{Contents: *s.markup(variableSourceText(d, def, s.markdown)), Range: &rng}
	}
	pv := s.config.variables(d)
	if src, ok := pv.names[name]; ok {
		return &Hover{Contents: *s.markup(externalSourceText(src, pv)), Range: &rng}
	}
	return &Hover{Contents: *s.markup(fmt.Sprintf("%s: undefined variable.", s.code(name))), Range: &rng}
}

// code quotes name as inline code for a markdown client, or as a plain
// quoted string otherwise, so a plaintext client never sees a literal
// backtick.
func (s *Server) code(name string) string {
	if s.markdown {
		return "`" + name + "`"
	}
	return `"` + name + `"`
}

// variableSourceText describes def's source. markdown selects how the
// `redact` keyword is quoted; pass false when the result goes into a
// CompletionItem.Detail, which LSP always renders as plain text.
func variableSourceText(d *document, def definition, markdown bool) string {
	line := d.lines.position(def.span.Start.Offset).Line + 1
	if def.kind == defCapture {
		if def.redact {
			redact := `"redact"`
			if markdown {
				redact = "`redact`"
			}
			return fmt.Sprintf("Capture at line %d (value hidden: captured with %s).", line, redact)
		}
		return fmt.Sprintf("Capture at line %d.", line)
	}
	return fmt.Sprintf("[Options] variable at line %d.", line)
}

func externalSourceText(src varSource, pv *projectVars) string {
	switch src.kind {
	case srcEnvironment:
		return fmt.Sprintf("sonde.yaml environment %q (value hidden).", pv.env)
	case srcVariablesFile:
		return fmt.Sprintf("Variables file %s (value hidden).", src.file)
	case srcSecretsFile:
		return fmt.Sprintf("Secrets file %s (value hidden).", src.file)
	case srcProcessSecret:
		return "Process environment secret (value hidden)."
	case srcExtra:
		return "Defined by the client (value hidden)."
	default: // srcProcessEnv
		return "Process environment variable (value hidden)."
	}
}

// hoverKeyword answers a query, filter, predicate, option name or section
// header from the parsed AST. It only sees entries that parsed: an entry
// being actively edited usually has no AST yet (see completion_context.go).
func (s *Server) hoverKeyword(d *document, offset int) *Hover {
	if d.file == nil {
		return nil
	}
	for _, e := range d.file.Entries {
		if e.Request != nil && spanContains(e.Request.Span, offset) {
			if h := s.hoverSections(d, e.Request.Sections, offset); h != nil {
				return h
			}
		}
		if e.Response != nil && spanContains(e.Response.Span, offset) {
			if h := s.hoverSections(d, e.Response.Sections, offset); h != nil {
				return h
			}
		}
	}
	return nil
}

func (s *Server) hoverSections(d *document, sections []*syntax.Section, offset int) *Hover {
	for _, sec := range sections {
		if spanContains(sec.Span, offset) {
			rng := d.lines.spanRange(sec.Span)
			return &Hover{Contents: *s.markup(sectionDocs[sec.Kind]), Range: &rng}
		}
		switch sec.Kind {
		case syntax.SectionOptions:
			if h := s.hoverOptions(d, sec.Options, offset); h != nil {
				return h
			}
		case syntax.SectionCaptures:
			if h := s.hoverCaptures(d, sec.Captures, offset); h != nil {
				return h
			}
		case syntax.SectionAsserts:
			if h := s.hoverAsserts(d, sec.Asserts, offset); h != nil {
				return h
			}
		case syntax.SectionMessages:
			if h := s.hoverSteps(d, sec.Messages, offset); h != nil {
				return h
			}
		case syntax.SectionGrpc:
			if h := s.hoverGrpcKeys(d, sec.KeyValues, offset); h != nil {
				return h
			}
		}
	}
	return nil
}

func (s *Server) hoverOptions(d *document, opts []*syntax.Option, offset int) *Hover {
	for _, o := range opts {
		start := o.Space0.Span.End.Offset
		end := start + len(o.Name)
		if offset >= start && offset <= end {
			if e, ok := s.lookup("options", o.Name); ok {
				rng := d.lines.rangeOf(start, end)
				return &Hover{Contents: *s.markup(entryDoc(e)), Range: &rng}
			}
		}
	}
	return nil
}

func (s *Server) hoverSteps(d *document, steps []*syntax.MessageStep, offset int) *Hover {
	for _, m := range steps {
		start := m.Span.Start.Offset
		end := start + len(m.Kind.String())
		if offset >= start && offset <= end {
			if e, ok := s.lookup("steps", m.Kind.String()); ok {
				rng := d.lines.rangeOf(start, end)
				return &Hover{Contents: *s.markup(entryDoc(e)), Range: &rng}
			}
		}
	}
	return nil
}

func (s *Server) hoverGrpcKeys(d *document, kvs []*syntax.KeyValue, offset int) *Hover {
	for _, kv := range kvs {
		if !spanContains(kv.Key.Span, offset) {
			continue
		}
		for _, el := range kv.Key.Elements {
			if ts, ok := el.(*syntax.TemplateString); ok {
				if e, ok := s.lookup("grpc-keys", ts.Value); ok {
					rng := d.lines.spanRange(kv.Key.Span)
					return &Hover{Contents: *s.markup(entryDoc(e)), Range: &rng}
				}
			}
		}
	}
	return nil
}

func (s *Server) hoverCaptures(d *document, caps []*syntax.Capture, offset int) *Hover {
	for _, c := range caps {
		if h := s.hoverQueryAndFilters(d, c.Query, c.Filters, offset); h != nil {
			return h
		}
		if c.Redact {
			end := c.Span.End.Offset
			start := end - len("redact")
			if offset >= start && offset <= end {
				rng := d.lines.rangeOf(start, end)
				return &Hover{Contents: *s.markup(redactDoc), Range: &rng}
			}
		}
	}
	return nil
}

func (s *Server) hoverAsserts(d *document, asserts []*syntax.Assert, offset int) *Hover {
	for _, a := range asserts {
		if h := s.hoverQueryAndFilters(d, a.Query, a.Filters, offset); h != nil {
			return h
		}
		if a.Predicate == nil {
			continue
		}
		if a.Predicate.Not {
			end := a.Predicate.Func.Span.Start.Offset - len(a.Predicate.Space0.Value)
			start := end - len("not")
			if offset >= start && offset <= end {
				rng := d.lines.rangeOf(start, end)
				return &Hover{Contents: *s.markup(notDoc), Range: &rng}
			}
		}
		f := a.Predicate.Func
		if f != nil && spanContains(f.Span, offset) {
			if e, ok := s.table.Lookup("predicates", f.Kind.String()); ok {
				rng := d.lines.spanRange(f.Span)
				return &Hover{Contents: *s.markup(entryDoc(e)), Range: &rng}
			}
		}
	}
	return nil
}

func (s *Server) hoverQueryAndFilters(d *document, q *syntax.Query, filters []*syntax.FilterItem, offset int) *Hover {
	if q != nil && spanContains(q.Span, offset) {
		if e, ok := s.lookup("queries", q.Kind.String()); ok {
			rng := d.lines.spanRange(q.Span)
			return &Hover{Contents: *s.markup(entryDoc(e)), Range: &rng}
		}
	}
	for _, fi := range filters {
		if fi.Filter != nil && spanContains(fi.Filter.Span, offset) {
			if e, ok := s.table.Lookup("filters", fi.Filter.Kind.String()); ok {
				rng := d.lines.spanRange(fi.Filter.Span)
				return &Hover{Contents: *s.markup(entryDoc(e)), Range: &rng}
			}
		}
	}
	return nil
}

func spanContains(span syntax.Span, offset int) bool {
	return offset >= span.Start.Offset && offset <= span.End.Offset
}
