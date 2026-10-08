// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import "sort"

// Lint renders f in the reference formatter's canonical form, the output
// `sonde fmt` prints. It is Format plus these rules:
//
//   - request sections are reordered: [Options], [Query]/[QueryStringParams],
//     [BasicAuth], [Form]/[FormParams], [Multipart]/[MultipartFormData],
//     [Cookies], then the .sonde-only [SondeGrpc] and [SondeMessages];
//     response sections: [Captures], then [Asserts]. Comments and blank
//     lines above a section move with it; an empty [BasicAuth] is dropped;
//   - a unitless connect-timeout, delay, max-time or retry-interval option
//     gets the default unit, ms;
//   - comment lines lose their indentation (a comment after an item keeps
//     the spaces before it), and comments lose trailing whitespace;
//   - line endings are kept (a missing final one is added), as is the
//     whitespace after a multiline string's language;
//   - a key-value pair whose value is the empty string prints `key:`;
//   - a byte order mark is dropped.
//
// f itself is not modified. Lint is idempotent.
func Lint(f *File) []byte {
	p := printer{canonical: true, lint: true}
	p.file(lintFile(f))
	return []byte(p.String())
}

// requestSectionOrder and responseSectionOrder rank the sections of each
// side in their canonical order.
var (
	requestSectionOrder = map[SectionKind]int{
		SectionOptions: 0, SectionQueryParams: 1, SectionBasicAuth: 2, SectionFormParams: 3,
		SectionMultipart: 4, SectionCookies: 5, SectionGrpc: 6, SectionMessages: 7,
	}
	responseSectionOrder = map[SectionKind]int{SectionCaptures: 0, SectionAsserts: 1}
)

// defaultMillisecondOptions are the duration options whose unitless value
// is in milliseconds.
var defaultMillisecondOptions = map[string]bool{
	"connect-timeout": true, "delay": true, "max-time": true, "retry-interval": true,
}

// lintFile returns a shallow copy of f with the structural rules applied:
// section order, empty [BasicAuth] removal and duration units. Nodes it
// does not change are shared with f.
func lintFile(f *File) *File {
	out := *f
	out.BOM = false
	out.Entries = make([]*Entry, len(f.Entries))
	for i, e := range f.Entries {
		ne := &Entry{}
		if e.Request != nil {
			r := *e.Request
			r.Sections = lintSections(r.Sections, requestSectionOrder)
			ne.Request = &r
		}
		if e.Response != nil {
			r := *e.Response
			r.Sections = lintSections(r.Sections, responseSectionOrder)
			ne.Response = &r
		}
		out.Entries[i] = ne
	}
	return &out
}

func lintSections(sections []*Section, order map[SectionKind]int) []*Section {
	out := make([]*Section, 0, len(sections))
	for _, s := range sections {
		if s.Kind == SectionBasicAuth && len(s.KeyValues) == 0 {
			continue
		}
		if s.Kind == SectionOptions {
			s = lintOptionsSection(s)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Kind] < order[out[j].Kind] })
	return out
}

func lintOptionsSection(s *Section) *Section {
	ns := *s
	ns.Options = make([]*Option, len(s.Options))
	for i, o := range s.Options {
		if d, ok := o.Value.(*Duration); ok && d.Unit == "" && defaultMillisecondOptions[o.Name] {
			no := *o
			no.Value = &Duration{Value: d.Value, Unit: "ms"}
			o = &no
		}
		ns.Options[i] = o
	}
	return &ns
}
