// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import "github.com/nhtera/sonde/internal/syntax"

// variableUses returns every {{name}} variable reference in f, in source
// order; function calls (newUuid, newDate) are skipped. It visits only the
// nodes that can hold a placeholder, which keeps diagnostics on large files
// within budget. semantic_variable_uses_test.go checks it against a
// reflection walk of the whole tree, so a new placeholder location in the
// grammar fails that test until it is added here.
func variableUses(f *syntax.File) []use {
	var out []use
	for _, e := range f.Entries {
		collectRequestUses(&out, e.Request)
		if e.Response != nil {
			collectResponseUses(&out, e.Response)
		}
	}
	return out
}

func collectRequestUses(out *[]use, r *syntax.Request) {
	collectTemplateUses(out, r.URL)
	collectKeyValueUses(out, r.Headers)
	collectSectionUses(out, r.Sections)
	collectBodyUses(out, r.Body)
}

func collectResponseUses(out *[]use, r *syntax.Response) {
	collectKeyValueUses(out, r.Headers)
	collectSectionUses(out, r.Sections)
	collectBodyUses(out, r.Body)
}

func collectSectionUses(out *[]use, secs []*syntax.Section) {
	for _, sec := range secs {
		switch sec.Kind {
		case syntax.SectionQueryParams, syntax.SectionFormParams, syntax.SectionCookies, syntax.SectionBasicAuth, syntax.SectionGrpc:
			collectKeyValueUses(out, sec.KeyValues)
		case syntax.SectionMultipart:
			collectMultipartUses(out, sec.Multipart)
		case syntax.SectionOptions:
			for _, o := range sec.Options {
				collectNodeUses(out, o.Value)
			}
		case syntax.SectionCaptures:
			collectCaptureUses(out, sec.Captures)
		case syntax.SectionAsserts:
			collectAssertUses(out, sec.Asserts)
		case syntax.SectionMessages:
			for _, m := range sec.Messages {
				collectNodeUses(out, m.Value)
			}
		}
	}
}

func collectKeyValueUses(out *[]use, kvs []*syntax.KeyValue) {
	for _, kv := range kvs {
		collectTemplateUses(out, kv.Key)
		collectTemplateUses(out, kv.Value)
	}
}

func collectMultipartUses(out *[]use, params []syntax.MultipartParam) {
	for _, p := range params {
		switch v := p.(type) {
		case *syntax.KeyValue:
			collectTemplateUses(out, v.Key)
			collectTemplateUses(out, v.Value)
		case *syntax.FilenameParam:
			collectTemplateUses(out, v.Key)
			if v.Value != nil {
				collectTemplateUses(out, v.Value.Filename)
				collectTemplateUses(out, v.Value.ContentType)
			}
		}
	}
}

func collectCaptureUses(out *[]use, caps []*syntax.Capture) {
	for _, c := range caps {
		collectTemplateUses(out, c.Name)
		if c.Query != nil {
			collectNodeUses(out, c.Query.Arg)
		}
		collectFilterUses(out, c.Filters)
	}
}

func collectAssertUses(out *[]use, asserts []*syntax.Assert) {
	for _, a := range asserts {
		if a.Query != nil {
			collectNodeUses(out, a.Query.Arg)
		}
		collectFilterUses(out, a.Filters)
		if a.Predicate != nil && a.Predicate.Func != nil {
			collectNodeUses(out, a.Predicate.Func.Value)
		}
	}
}

func collectFilterUses(out *[]use, items []*syntax.FilterItem) {
	for _, it := range items {
		if it.Filter == nil {
			continue
		}
		collectNodeUses(out, it.Filter.Arg)
		collectNodeUses(out, it.Filter.Arg2)
	}
}

func collectBodyUses(out *[]use, b *syntax.Body) {
	if b == nil {
		return
	}
	collectNodeUses(out, b.Value)
}

func collectTemplateUses(out *[]use, t *syntax.Template) {
	if t == nil {
		return
	}
	for _, el := range t.Elements {
		if p, ok := el.(*syntax.Placeholder); ok {
			addUse(out, p.Expr)
		}
	}
}

// collectNodeUses dispatches on n's concrete type: every polymorphic field
// that can carry a placeholder (Query.Arg, Filter.Arg/Arg2, Option.Value,
// Predicate.Func.Value, Body.Value, a JSON element's Value, ...) is typed
// as syntax.Node or an interface embedding it (Bytes, JSONValue,
// PredicateValue), so a single dispatcher covers them all. A type with no
// case here (Null, Boolean, Number, JSONNumber, Regex, XML, Base64, Hex,
// CertificateAttribute) never holds a placeholder.
func collectNodeUses(out *[]use, n syntax.Node) {
	switch v := n.(type) {
	case *syntax.Template:
		collectTemplateUses(out, v)
	case *syntax.Placeholder:
		addUse(out, v.Expr)
	case *syntax.JSONList:
		for _, el := range v.Elements {
			collectNodeUses(out, el.Value)
		}
	case *syntax.JSONObject:
		for _, el := range v.Elements {
			collectTemplateUses(out, el.Name)
			collectNodeUses(out, el.Value)
		}
	case *syntax.MultilineString:
		collectTemplateUses(out, v.Value)
		if v.Variables != nil {
			collectNodeUses(out, v.Variables.Value)
		}
	case *syntax.VariableDefinition:
		collectNodeUses(out, v.Value)
	case *syntax.FileRef:
		collectTemplateUses(out, v.Filename)
	case *syntax.CookiePath:
		collectTemplateUses(out, v.Name)
	}
}

func addUse(out *[]use, e syntax.Expr) {
	if e.Kind == syntax.ExprVariable {
		*out = append(*out, use{name: e.Name, span: e.Span})
	}
}
