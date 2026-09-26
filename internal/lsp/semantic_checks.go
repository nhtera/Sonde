// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"fmt"
	"sort"

	"github.com/nhtera/sonde/internal/docs"
	"github.com/nhtera/sonde/internal/syntax"
)

// maxSemanticDiagnostics caps warnings per document (parse errors have
// their own, separate cap: maxDiagnostics in documents.go): a minified,
// single-line file with thousands of placeholders and no sonde.yaml would
// otherwise flag every one of them, publishing an unbounded notification
// on every keystroke (phase 9 LSP review, H2).
const maxSemanticDiagnostics = 200

// semanticDiagnostics returns warnings for a document that parses: unknown
// variables, deprecated syntax and options Sonde cannot send yet.
func (s *Server) semanticDiagnostics(d *document) []Diagnostic {
	pv := s.config.variables(d)

	var out []Diagnostic
	if pv.err != nil {
		out = append(out, Diagnostic{
			Range:    d.lines.rangeOf(0, 0),
			Severity: severityWarning,
			Source:   source,
			Message:  pv.err.Error(),
		})
	}
	// A partial AST (parse errors present) would misreport definitions
	// that exist past the error, so this check only runs on a clean parse.
	// A document outside every workspace folder has no configuration at
	// all (projectVars.noFolder), so it cannot tell a genuinely undefined
	// variable from one defined in a sonde.yaml it simply never gets to
	// see; the check is skipped rather than flagging every use.
	if len(d.errs) == 0 && !pv.noFolder {
		out = append(out, undefinedVariableDiagnostics(d, pv)...)
	}
	out = append(out, deprecatedDiagnostics(s.table, d)...)
	out = append(out, unsupportedOptionDiagnostics(s.table, d)...)

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Range.Start, out[j].Range.Start
		if a != b {
			return a.Line < b.Line || (a.Line == b.Line && a.Character < b.Character)
		}
		return out[i].Message < out[j].Message
	})

	if len(out) > maxSemanticDiagnostics {
		hidden := len(out) - maxSemanticDiagnostics
		out = append(out[:maxSemanticDiagnostics], Diagnostic{
			Range:    d.lines.rangeOf(0, 0),
			Severity: severityInformation,
			Source:   source,
			Message:  fmt.Sprintf("%d more warnings not shown", hidden),
		})
	}
	return out
}

// undefinedVariableDiagnostics warns about a {{name}} use that is neither
// captured earlier in the document nor defined by its project or process
// environment.
//
// A name is visible at offset iff some definition of it has from <=
// offset; that is equivalent to (and cheaper than) asking visibleAt for a
// fresh map at every use, since it only takes the smallest from per name,
// computed once, rather than a defs-sized map rebuilt for every use.
func undefinedVariableDiagnostics(d *document, pv *projectVars) []Diagnostic {
	firstFrom := map[string]int{}
	for _, def := range definitions(d.file) {
		if from, ok := firstFrom[def.name]; !ok || def.from < from {
			firstFrom[def.name] = def.from
		}
	}

	var out []Diagnostic
	for _, u := range variableUses(d.file) {
		if from, ok := firstFrom[u.name]; ok && from <= u.span.Start.Offset {
			continue
		}
		if _, ok := pv.names[u.name]; ok {
			continue
		}
		out = append(out, Diagnostic{
			Range:    d.lines.spanRange(u.span),
			Severity: severityWarning,
			Source:   source,
			Message:  undefinedVariableMessage(u.name, pv.env),
		})
	}
	return out
}

func undefinedVariableMessage(name, env string) string {
	if env == "" {
		return fmt.Sprintf("undefined variable %q: not captured earlier and no sonde.yaml environment selected", name)
	}
	return fmt.Sprintf("undefined variable %q: not captured earlier and not defined in environment %q", name, env)
}

// deprecatedDiagnostics warns on a filter or predicate whose docs table
// entry names its replacement (e.g. decode, deprecated for charsetDecode).
func deprecatedDiagnostics(table *docs.Table, d *document) []Diagnostic {
	var out []Diagnostic
	for _, e := range d.file.Entries {
		if e.Response == nil {
			continue
		}
		for _, sec := range e.Response.Sections {
			switch sec.Kind {
			case syntax.SectionCaptures:
				for _, c := range sec.Captures {
					out = append(out, filterDeprecations(table, d.lines, c.Filters)...)
				}
			case syntax.SectionAsserts:
				for _, a := range sec.Asserts {
					out = append(out, filterDeprecations(table, d.lines, a.Filters)...)
					if a.Predicate == nil || a.Predicate.Func == nil {
						continue
					}
					f := a.Predicate.Func
					if diag, ok := deprecatedDiagnostic(table, d.lines, "predicates", f.Kind.String(), f.Span.Start); ok {
						out = append(out, diag)
					}
				}
			}
		}
	}
	return out
}

func filterDeprecations(table *docs.Table, lines *lineIndex, items []*syntax.FilterItem) []Diagnostic {
	var out []Diagnostic
	for _, it := range items {
		if it.Filter == nil {
			continue
		}
		if diag, ok := deprecatedDiagnostic(table, lines, "filters", it.Filter.Kind.String(), it.Filter.Span.Start); ok {
			out = append(out, diag)
		}
	}
	return out
}

// deprecatedDiagnostic looks up name in kind ("filters" or "predicates")
// and, when its entry names a replacement, returns a warning underlining
// just the keyword at the start of its span.
func deprecatedDiagnostic(table *docs.Table, lines *lineIndex, kind, name string, at syntax.Pos) (Diagnostic, bool) {
	entry, ok := table.Lookup(kind, name)
	if !ok || entry.Deprecated == "" {
		return Diagnostic{}, false
	}
	return Diagnostic{
		Range:    lines.rangeOf(at.Offset, at.Offset+len(name)),
		Severity: severityWarning,
		Source:   source,
		Tags:     []int{tagDeprecated},
		Message:  fmt.Sprintf("%q is deprecated; use %q", name, entry.Deprecated),
	}, true
}

// unsupportedOptionDiagnostics warns on an [Options] entry Sonde cannot
// send yet (Phase 8 carry-forward): its docs table status is unsupported
// or planned.
func unsupportedOptionDiagnostics(table *docs.Table, d *document) []Diagnostic {
	var out []Diagnostic
	for _, e := range d.file.Entries {
		for _, sec := range e.Request.Sections {
			if sec.Kind != syntax.SectionOptions {
				continue
			}
			for _, o := range sec.Options {
				entry, ok := table.Lookup("options", o.Name)
				if !ok || (entry.Status != docs.StatusUnsupported && entry.Status != docs.StatusPlanned) {
					continue
				}
				start := o.Space0.Span.End
				out = append(out, Diagnostic{
					Range:    d.lines.rangeOf(start.Offset, start.Offset+len(o.Name)),
					Severity: severityWarning,
					Source:   source,
					Message:  fmt.Sprintf("not supported by Sonde yet: %s", entry.Reason),
				})
			}
		}
	}
	return out
}
