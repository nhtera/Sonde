// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package syntaxedit reads the entries of a request file as a model a
// form can show, and edits the file one element at a time: every edit
// replaces only the bytes of its target, keeping comments, blank lines
// and formatting elsewhere, and is checked by parsing the result. Every
// offset of the API is in UTF-16 code units, as the language server's.
package syntaxedit

import (
	"bytes"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// Range is a half-open range of UTF-16 code units [Start, End).
type Range struct{ Start, End int }

// TextEdit replaces Range of the original source with NewText.
type TextEdit struct {
	Range   Range
	NewText string
}

// Result is an edited source and the edit that produced it from the
// original.
type Result struct {
	Source []byte
	Edits  []TextEdit
}

// Section names a list of rows of an entry.
type Section string

// Sections. Request headers and response headers are rows without a
// section header.
const (
	Headers         Section = "headers"
	Query           Section = "query"
	Form            Section = "form"
	Multipart       Section = "multipart"
	Cookies         Section = "cookies"
	BasicAuth       Section = "basic-auth"
	Options         Section = "options"
	ResponseHeaders Section = "response-headers"
	Captures        Section = "captures"
	Asserts         Section = "asserts"
	// Grpc is [SondeGrpc] (.sonde only): proto, import-path, protoset.
	Grpc Section = "grpc"
)

// sectionInfo is how a Section is written.
var sectionInfo = map[Section]struct {
	name     string // the header written for a new section ("" for headers)
	kind     syntax.SectionKind
	response bool
	keyed    bool // rows are `key: value`; asserts are text only
}{
	Headers:         {"", 0, false, true},
	Query:           {"Query", syntax.SectionQueryParams, false, true},
	Form:            {"Form", syntax.SectionFormParams, false, true},
	Multipart:       {"Multipart", syntax.SectionMultipart, false, true},
	Cookies:         {"Cookies", syntax.SectionCookies, false, true},
	BasicAuth:       {"BasicAuth", syntax.SectionBasicAuth, false, true},
	Options:         {"Options", syntax.SectionOptions, false, true},
	ResponseHeaders: {"", 0, true, true},
	Captures:        {"Captures", syntax.SectionCaptures, true, true},
	Asserts:         {"Asserts", syntax.SectionAsserts, true, false},
	Grpc:            {"SondeGrpc", syntax.SectionGrpc, false, true},
}

// Row is one row of a section. Key and Value are source text, as written
// (an assert has only a Value, its whole text). A disabled row is a
// `# key: value` comment line that reads as a row of its section.
type Row struct {
	Key, Value string
	Disabled   bool
	// Range is the row's text, from its key to the end of its value.
	Range Range
}

// EntryModel is an entry as a form shows it. Text values are source
// text, as written.
type EntryModel struct {
	// Index is the entry's 1-based position.
	Index int
	// Range spans the entry, from its method to the end of its last
	// line (comment lines above the method excluded).
	Range       Range
	Method      string
	MethodRange Range
	URL         string
	URLRange    Range
	// HasResponse tells whether the entry has an expected response.
	HasResponse bool
	Status      string
	StatusRange Range
	// HasBody tells whether the request has a body; Body is its source
	// text.
	HasBody   bool
	Body      string
	BodyRange Range
	// Rows are the rows of every section present, by section.
	Rows map[Section][]Row
}

// Model parses src (a file named name, for its dialect) into the model of
// its entries.
func Model(name string, src []byte) ([]EntryModel, error) {
	d, err := parse(name, src)
	if err != nil {
		return nil, err
	}
	out := make([]EntryModel, len(d.entries))
	for i := range d.entries {
		out[i] = d.model(i)
	}
	return out, nil
}

// EntryAt returns the 1-based index of the entry at offset (UTF-16) of the
// source the models were built from: the entry whose range holds it, or
// the last entry starting before it; 0 before the first entry.
func EntryAt(models []EntryModel, offset int) int {
	at := 0
	for _, m := range models {
		if m.Range.Start > offset {
			break
		}
		at = m.Index
	}
	return at
}

// doc is a parsed source with the line layout of its entries.
type doc struct {
	name    string
	src     []byte
	nl      string // the line ending of the source: "\r\n" or "\n"
	file    *syntax.File
	off     *offsets
	entries []*entry
}

// entry is an entry's layout: byte offsets of its parts and its rows.
type entry struct {
	e *syntax.Entry
	// start is the method's offset; end the end of the entry's last line.
	start, end int
	// reqEnd is the end of the request's last line (before the response
	// or the end); respEnd the response's.
	reqEnd, respEnd int
	sections        map[Section]*section
	// sectionOrder lists the request's, then the response's sections.
	sectionOrder []Section
}

// section is the layout of a section of an entry.
type section struct {
	// header is the byte range of the `[Name]` line (empty for headers).
	headerStart, headerEnd int
	// end is the end of the section's last line: its last row, or its
	// header.
	end  int
	rows []row
}

// row is a row's layout.
type row struct {
	Row
	// start and end are the row's text (key to value end); lineStart and
	// lineEnd its whole line, newline included.
	start, end         int
	lineStart, lineEnd int
}

func parse(name string, src []byte) (*doc, error) {
	f, err := syntax.Parse(name, src, syntax.DialectFor(name))
	if err != nil {
		return nil, err
	}
	d := &doc{name: name, src: src, file: f, off: newOffsets(src), nl: "\n"}
	if i := bytes.IndexByte(src, '\n'); i > 0 && src[i-1] == '\r' {
		d.nl = "\r\n"
	}
	for i, e := range f.Entries {
		next := len(src)
		if i+1 < len(f.Entries) {
			// The comment lines right above the next method are its own.
			next = d.firstLineAbove(f.Entries[i+1].Request.Method.Span.Start.Offset)
		}
		d.entries = append(d.entries, d.layout(e, next))
	}
	return d, nil
}

// lineStart returns the offset of the start of the line holding off.
func (d *doc) lineStart(off int) int {
	return bytes.LastIndexByte(d.src[:off], '\n') + 1
}

// firstLineAbove is the start of the first of the comment lines directly
// above the line holding off (no blank line between), or of that line.
func (d *doc) firstLineAbove(off int) int {
	start := d.lineStart(off)
	for start > 0 {
		prev := d.lineStart(start - 1)
		if !strings.HasPrefix(strings.TrimSpace(string(d.src[prev:start])), "#") {
			break
		}
		start = prev
	}
	return start
}

// lineEndAfter returns the offset after the newline ending the line
// holding off (the end of the source on the last line).
func (d *doc) lineEndAfter(off int) int {
	if i := bytes.IndexByte(d.src[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(d.src)
}

// ltEnd is the end of a line terminator: after its newline.
func ltEnd(lt *syntax.LineTerminator) int {
	return lt.Newline.Span.End.Offset
}

func (d *doc) text(s syntax.Span) string { return string(d.src[s.Start.Offset:s.End.Offset]) }

// layout lays out an entry that ends before next.
func (d *doc) layout(e *syntax.Entry, next int) *entry {
	r := e.Request
	en := &entry{e: e, start: r.Method.Span.Start.Offset, sections: map[Section]*section{}}
	en.reqEnd = ltEnd(r.LineTerminator0)
	en.addKeyValues(d, Headers, section{end: en.reqEnd}, r.Headers)
	for _, s := range r.Sections {
		en.addSection(d, s)
	}
	if r.Body != nil {
		en.reqEnd = max(en.reqEnd, ltEnd(r.Body.LineTerminator0))
	}
	en.respEnd = en.reqEnd
	if resp := e.Response; resp != nil {
		en.respEnd = ltEnd(resp.LineTerminator0)
		en.addKeyValues(d, ResponseHeaders, section{end: en.respEnd}, resp.Headers)
		for _, s := range resp.Sections {
			en.addSection(d, s)
		}
		if resp.Body != nil {
			en.respEnd = max(en.respEnd, ltEnd(resp.Body.LineTerminator0))
		}
	}
	d.findDisabled(en, next)
	en.end = en.respEnd
	return en
}

// addKeyValues adds header rows.
func (en *entry) addKeyValues(d *doc, name Section, s section, kvs []*syntax.KeyValue) {
	for _, kv := range kvs {
		s.rows = append(s.rows, d.keyValueRow(kv))
	}
	if n := len(s.rows); n > 0 {
		s.end = s.rows[n-1].lineEnd
		en.bump(name, s.end)
	}
	en.sections[name] = &s
	en.sectionOrder = append(en.sectionOrder, name)
}

// bump moves the end of the request or response past end.
func (en *entry) bump(name Section, end int) {
	if sectionInfo[name].response {
		en.respEnd = max(en.respEnd, end)
	} else {
		en.reqEnd = max(en.reqEnd, end)
	}
}

func (d *doc) keyValueRow(kv *syntax.KeyValue) row {
	return d.newRow(d.text(kv.Key.Span), d.text(kv.Value.Span), kv.Key.Span.Start.Offset, kv.Value.Span.End.Offset, kv.LineTerminator0)
}

func (d *doc) newRow(key, value string, start, end int, lt *syntax.LineTerminator) row {
	r := row{start: start, end: end, lineStart: d.lineStart(start), lineEnd: ltEnd(lt)}
	r.Key, r.Value = key, value
	r.Range = d.off.rng(start, end)
	return r
}

// addSection adds a [Section] and its rows.
func (en *entry) addSection(d *doc, s *syntax.Section) {
	name := sectionFor(s.Kind)
	if name == "" {
		return // [SondeMessages]: steps, not rows
	}
	sec := &section{headerStart: d.lineStart(s.Span.Start.Offset), headerEnd: ltEnd(s.LineTerminator0)}
	sec.end = sec.headerEnd
	for _, kv := range s.KeyValues {
		sec.rows = append(sec.rows, d.keyValueRow(kv))
	}
	for _, p := range s.Multipart {
		switch p := p.(type) {
		case *syntax.KeyValue:
			sec.rows = append(sec.rows, d.keyValueRow(p))
		case *syntax.FilenameParam:
			end := p.LineTerminator0.Space0.Span.Start.Offset
			sec.rows = append(sec.rows, d.newRow(d.text(p.Key.Span), string(d.src[p.Space2.Span.End.Offset:end]), p.Key.Span.Start.Offset, end, p.LineTerminator0))
		}
	}
	for _, o := range s.Options {
		start, end := o.Space0.Span.End.Offset, o.LineTerminator0.Space0.Span.Start.Offset
		sec.rows = append(sec.rows, d.newRow(o.Name, string(d.src[o.Space2.Span.End.Offset:end]), start, end, o.LineTerminator0))
	}
	for _, c := range s.Captures {
		sec.rows = append(sec.rows, d.newRow(d.text(c.Name.Span), string(d.src[c.Space2.Span.End.Offset:c.Span.End.Offset]), c.Span.Start.Offset, c.Span.End.Offset, c.LineTerminator0))
	}
	for _, a := range s.Asserts {
		sec.rows = append(sec.rows, d.newRow("", d.text(a.Span), a.Span.Start.Offset, a.Span.End.Offset, a.LineTerminator0))
	}
	if n := len(sec.rows); n > 0 {
		sec.end = sec.rows[n-1].lineEnd
	}
	en.sections[name] = sec
	en.sectionOrder = append(en.sectionOrder, name)
	en.bump(name, sec.end)
}

// sectionFor maps a parsed section kind to its Section.
func sectionFor(k syntax.SectionKind) Section {
	for name, info := range sectionInfo {
		if info.name != "" && info.kind == k {
			return name
		}
	}
	return ""
}

// findDisabled adds the disabled rows of every section: `# ` comment
// lines between the section's first line and the next part of the entry
// (next section, body, response, next entry at next) that read as one
// row of that section.
func (d *doc) findDisabled(en *entry, next int) {
	// boundaries are the offsets where each part of the entry starts.
	type part struct {
		name  Section
		start int // first line of the section (its header, or its first row)
	}
	var parts []part
	for _, name := range en.sectionOrder {
		s := en.sections[name]
		start := s.headerStart
		if sectionInfo[name].name == "" {
			// Headers start after the method (or status) line.
			start = en.start
			if sectionInfo[name].response {
				start = d.lineStart(en.e.Response.Status.Span.Start.Offset)
			}
			start = d.lineEndAfter(start)
		}
		parts = append(parts, part{name, start})
	}
	stops := []int{next}
	if b := en.e.Request.Body; b != nil {
		stops = append(stops, d.lineStart(b.Span.Start.Offset))
	}
	if r := en.e.Response; r != nil {
		stops = append(stops, d.lineStart(r.Version.Span.Start.Offset))
		if r.Body != nil {
			stops = append(stops, d.lineStart(r.Body.Span.Start.Offset))
		}
	}
	for i, p := range parts {
		limit := next
		for _, s := range stops {
			if s > p.start && s < limit {
				limit = s
			}
		}
		for _, q := range parts[i+1:] {
			if q.start > p.start && q.start < limit {
				limit = q.start
			}
		}
		d.scanDisabled(en, p.name, p.start, limit)
	}
}

// scanDisabled adds the disabled rows of section name found in the lines
// of [from, to), in order among its rows: comment lines outside its rows'
// own text (a multi-line value), before the first blank line that follows
// the section's content.
func (d *doc) scanDisabled(en *entry, name Section, from, to int) {
	s := en.sections[name]
	var covered [][2]int
	contentEnd := from
	for _, r := range s.rows {
		covered = append(covered, [2]int{r.lineStart, r.lineEnd})
		contentEnd = max(contentEnd, r.lineEnd)
	}
	inRow := func(pos int) bool {
		for _, c := range covered {
			if pos >= c[0] && pos < c[1] {
				return true
			}
		}
		return false
	}
	for pos := from; pos < to; {
		end := d.lineEndAfter(pos)
		if end > to {
			end = to
		}
		line := string(d.src[pos:end])
		body := strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimLeft(body, " \t")
		if trimmed == "" && pos >= contentEnd {
			return
		}
		if inRow(pos) {
			pos = end
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "#"); ok {
			text := strings.TrimPrefix(rest, " ")
			if key, value, ok := readRow(d.name, name, text); ok {
				// The row's text follows the `# `.
				start := pos + len(body) - len(text)
				r := row{start: start, end: pos + len(body), lineStart: pos, lineEnd: end}
				r.Key, r.Value, r.Disabled = key, value, true
				r.Range = d.off.rng(r.start, r.end)
				s.rows = insertRow(s.rows, r)
				contentEnd = max(contentEnd, end)
				if end > s.end {
					s.end = end
					en.bump(name, end)
				}
			}
		}
		pos = end
	}
}

// insertRow inserts r among rows in source order.
func insertRow(rows []row, r row) []row {
	i := 0
	for i < len(rows) && rows[i].start < r.start {
		i++
	}
	rows = append(rows, row{})
	copy(rows[i+1:], rows[i:])
	rows[i] = r
	return rows
}

// readRow reports whether text reads as exactly one row of section name,
// and returns its key and value source text.
func readRow(fileName string, name Section, text string) (key, value string, ok bool) {
	if text == "" || strings.ContainsAny(text, "\r\n") {
		return "", "", false
	}
	info := sectionInfo[name]
	var b strings.Builder
	b.WriteString("GET http://x\n")
	if info.response {
		b.WriteString("HTTP *\n")
	}
	if info.name != "" {
		b.WriteString("[" + info.name + "]\n")
	}
	prefix := b.Len()
	b.WriteString(text + "\n")
	d, err := parse(fileName, []byte(b.String()))
	if err != nil || len(d.entries) != 1 {
		return "", "", false
	}
	en := d.entries[0]
	e := en.e
	// Nothing but the row: no body, no other section.
	if e.Request.Body != nil || (e.Response != nil && e.Response.Body != nil) {
		return "", "", false
	}
	if len(en.sectionOrder) != 2+boolInt(info.response) && info.name != "" {
		return "", "", false
	}
	s := en.sections[name]
	if s == nil || len(s.rows) != 1 || s.rows[0].Disabled || s.rows[0].start != prefix {
		return "", "", false
	}
	return s.rows[0].Key, s.rows[0].Value, true
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// model builds the model of entry i.
func (d *doc) model(i int) EntryModel {
	en := d.entries[i]
	r := en.e.Request
	m := EntryModel{
		Index:       i + 1,
		Range:       d.off.rng(en.start, en.end),
		Method:      r.Method.Value,
		MethodRange: d.off.rng(r.Method.Span.Start.Offset, r.Method.Span.End.Offset),
		URL:         d.text(r.URL.Span),
		URLRange:    d.off.rng(r.URL.Span.Start.Offset, r.URL.Span.End.Offset),
		Rows:        map[Section][]Row{},
	}
	if resp := en.e.Response; resp != nil {
		m.HasResponse = true
		m.Status = resp.Status.Value
		m.StatusRange = d.off.rng(resp.Status.Span.Start.Offset, resp.Status.Span.End.Offset)
	}
	if b := r.Body; b != nil {
		m.HasBody = true
		m.Body = d.text(b.Span)
		m.BodyRange = d.off.rng(b.Span.Start.Offset, b.Span.End.Offset)
	}
	for name, s := range en.sections {
		if len(s.rows) == 0 && sectionInfo[name].name == "" {
			continue
		}
		rows := make([]Row, len(s.rows))
		for j, rw := range s.rows {
			rows[j] = rw.Row
		}
		m.Rows[name] = rows
	}
	return m
}
