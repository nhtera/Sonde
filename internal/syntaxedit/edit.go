// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxedit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// Every operation takes the file's name (for its dialect), its source and
// a 1-based entry index, makes one splice and parses the result: an edit
// that would not parse, or would not read back as asked, is refused and
// the source is unchanged.

// ErrInvalid reports an edit refused because its result does not read as
// asked (a value that would start a comment, break a line, ...).
var ErrInvalid = errors.New("syntaxedit: invalid edit")

// splice replaces src[start:end] with text and checks the result with
// check (nil: parsing is enough).
func (d *doc) splice(start, end int, text string, check func(*doc) error) (*Result, error) {
	text = d.eol(text)
	out := make([]byte, 0, len(d.src)-(end-start)+len(text))
	out = append(out, d.src[:start]...)
	out = append(out, text...)
	out = append(out, d.src[end:]...)
	nd, err := parse(d.name, out)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if check != nil {
		if err := check(nd); err != nil {
			return nil, err
		}
	}
	return &Result{Source: out, Edits: []TextEdit{{Range: d.off.rng(start, end), NewText: text}}}, nil
}

// eol writes the line breaks of text as the source does.
func (d *doc) eol(text string) string {
	if d.nl == "\n" {
		return text
	}
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", d.nl)
}

// load parses src and returns entry n (1-based).
func load(name string, src []byte, n int) (*doc, *entry, error) {
	d, err := parse(name, src)
	if err != nil {
		return nil, nil, err
	}
	if n < 1 || n > len(d.entries) {
		return nil, nil, fmt.Errorf("syntaxedit: entry %d out of range (the file has %d)", n, len(d.entries))
	}
	return d, d.entries[n-1], nil
}

// singleLine refuses a value with a line break.
func singleLine(what, s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return fmt.Errorf("%w: %s holds a line break", ErrInvalid, what)
	}
	return nil
}

// sameEntries checks that the edit kept the number of entries.
func sameEntries(before *doc) func(*doc) error {
	return func(after *doc) error {
		if len(after.entries) != len(before.entries) {
			return fmt.Errorf("%w: the edit changes the entries of the file", ErrInvalid)
		}
		return nil
	}
}

// both runs every check.
func both(checks ...func(*doc) error) func(*doc) error {
	return func(d *doc) error {
		for _, c := range checks {
			if err := c(d); err != nil {
				return err
			}
		}
		return nil
	}
}

// SetMethod replaces the method of entry n.
func SetMethod(name string, src []byte, n int, method string) (*Result, error) {
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	sp := en.e.Request.Method.Span
	return d.splice(sp.Start.Offset, sp.End.Offset, method, both(sameEntries(d), func(nd *doc) error {
		if got := nd.entries[n-1].e.Request.Method.Value; got != method {
			return fmt.Errorf("%w: method %q reads as %q", ErrInvalid, method, got)
		}
		return nil
	}))
}

// SetURL replaces the URL of entry n with url, source text.
func SetURL(name string, src []byte, n int, url string) (*Result, error) {
	if err := singleLine("the URL", url); err != nil {
		return nil, err
	}
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	sp := en.e.Request.URL.Span
	return d.splice(sp.Start.Offset, sp.End.Offset, url, both(sameEntries(d), func(nd *doc) error {
		if got := nd.text(nd.entries[n-1].e.Request.URL.Span); got != url {
			return fmt.Errorf("%w: URL %q reads as %q", ErrInvalid, url, got)
		}
		return nil
	}))
}

// SetStatus sets the expected status of entry n ("*" or digits), adding
// an `HTTP <status>` response when the entry has none.
func SetStatus(name string, src []byte, n int, status string) (*Result, error) {
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	check := both(sameEntries(d), func(nd *doc) error {
		resp := nd.entries[n-1].e.Response
		if resp == nil || resp.Status.Value != status {
			return fmt.Errorf("%w: status %q", ErrInvalid, status)
		}
		return nil
	})
	if resp := en.e.Response; resp != nil {
		return d.splice(resp.Status.Span.Start.Offset, resp.Status.Span.End.Offset, status, check)
	}
	at, lead := d.insertionPoint(en.reqEnd)
	return d.splice(at, at, lead+"HTTP "+status+"\n", check)
}

// insertionPoint returns where to insert lines after the line ending at
// end, and the newline to write first when that line has none (the end of
// the source).
func (d *doc) insertionPoint(end int) (int, string) {
	if end > 0 && d.src[end-1] != '\n' {
		return end, "\n"
	}
	return end, ""
}

// rowText renders a row of section name.
func rowText(name Section, key, value string) string {
	if !sectionInfo[name].keyed {
		return value
	}
	if value == "" {
		return key + ":"
	}
	return key + ": " + value
}

// checkRow checks that section name of entry n has want rows and that row
// i reads as key and value (i < 0: no row to check).
func checkRow(n int, name Section, want, i int, key, value string, disabled bool) func(*doc) error {
	return func(nd *doc) error {
		s := nd.entries[n-1].sections[name]
		if s == nil || len(s.rows) != want {
			got := 0
			if s != nil {
				got = len(s.rows)
			}
			return fmt.Errorf("%w: the %s section would have %d rows, not %d", ErrInvalid, name, got, want)
		}
		if i < 0 {
			return nil
		}
		r := s.rows[i]
		if r.Key != key || r.Value != value || r.Disabled != disabled {
			return fmt.Errorf("%w: the row %q reads as %q", ErrInvalid, rowText(name, key, value), rowText(name, r.Key, r.Value))
		}
		return nil
	}
}

// rowAt returns row i of section name of en.
func rowAt(en *entry, name Section, i int) (*section, row, error) {
	s := en.sections[name]
	if s == nil || i < 0 || i >= len(s.rows) {
		return nil, row{}, fmt.Errorf("syntaxedit: no row %d in the %s section", i, name)
	}
	return s, s.rows[i], nil
}

// SetRow replaces the key and value (source text) of row i (0-based,
// disabled rows counted) of section name of entry n. A disabled row stays
// disabled.
func SetRow(name string, src []byte, n int, section Section, i int, key, value string) (*Result, error) {
	if err := validRow(section, key, value); err != nil {
		return nil, err
	}
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	s, r, err := rowAt(en, section, i)
	if err != nil {
		return nil, err
	}
	if key == r.Key && value == r.Value {
		return &Result{Source: src}, nil // as written, spacing included
	}
	return d.splice(r.start, r.end, rowText(section, key, value), both(sameEntries(d), checkRow(n, section, len(s.rows), i, key, value, r.Disabled)))
}

// validRow checks a row's parts before an edit.
func validRow(section Section, key, value string) error {
	if _, ok := sectionInfo[section]; !ok {
		return fmt.Errorf("syntaxedit: unknown section %q", section)
	}
	if err := singleLine("the key", key); err != nil {
		return err
	}
	return singleLine("the value", value)
}

// AddRow appends a row to section name of entry n, adding the section
// (and an `HTTP *` response for a response section) when missing.
func AddRow(name string, src []byte, n int, section Section, key, value string) (*Result, error) {
	if err := validRow(section, key, value); err != nil {
		return nil, err
	}
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	count := 0
	if s := en.sections[section]; s != nil {
		count = len(s.rows)
	}
	at, text := d.rowInsertion(en, section)
	text += rowText(section, key, value) + "\n"
	return d.splice(at, at, text, both(sameEntries(d), checkRow(n, section, count+1, count, key, value, false)))
}

// rowInsertion returns where a new row of section name goes in en, and
// the lines to write before it (a newline, a section header, a response
// line).
func (d *doc) rowInsertion(en *entry, name Section) (int, string) {
	info := sectionInfo[name]
	if s := en.sections[name]; s != nil && (info.name != "" || len(s.rows) > 0) {
		at, lead := d.insertionPoint(s.end)
		return at, lead
	}
	var head string
	if info.name != "" {
		head = "[" + info.name + "]\n"
	}
	if !info.response {
		// Headers go right after the method line; a new section after the
		// last one, before the body.
		if info.name == "" {
			at, lead := d.insertionPoint(ltEnd(en.e.Request.LineTerminator0))
			return at, lead
		}
		end := ltEnd(en.e.Request.LineTerminator0)
		for _, sn := range en.sectionOrder {
			if !sectionInfo[sn].response {
				end = max(end, en.sections[sn].end)
			}
		}
		at, lead := d.insertionPoint(end)
		return at, lead + head
	}
	resp := en.e.Response
	if resp == nil {
		at, lead := d.insertionPoint(en.reqEnd)
		return at, lead + "HTTP *\n" + head
	}
	if info.name == "" {
		at, lead := d.insertionPoint(ltEnd(resp.LineTerminator0))
		return at, lead
	}
	end := ltEnd(resp.LineTerminator0)
	if h := en.sections[ResponseHeaders]; h != nil {
		end = max(end, h.end)
	}
	// Captures go before an [Asserts] section, as a formatted file has
	// them.
	if a := en.sections[Asserts]; name == Captures && a != nil {
		return a.headerStart, head
	}
	for _, sn := range en.sectionOrder {
		if sectionInfo[sn].response {
			end = max(end, en.sections[sn].end)
		}
	}
	at, lead := d.insertionPoint(end)
	return at, lead + head
}

// RemoveRow removes row i of section name of entry n, with its line. A
// section left empty keeps its header.
func RemoveRow(name string, src []byte, n int, section Section, i int) (*Result, error) {
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	s, r, err := rowAt(en, section, i)
	if err != nil {
		return nil, err
	}
	return d.splice(r.lineStart, r.lineEnd, "", both(sameEntries(d), checkRow(n, section, len(s.rows)-1, -1, "", "", false)))
}

// ToggleRow disables row i of section name of entry n (it becomes a
// `# ` comment line) or enables a disabled one.
func ToggleRow(name string, src []byte, n int, section Section, i int) (*Result, error) {
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	s, r, err := rowAt(en, section, i)
	if err != nil {
		return nil, err
	}
	check := both(sameEntries(d), checkRow(n, section, len(s.rows), i, r.Key, r.Value, !r.Disabled))
	if r.Disabled {
		// The comment runs from the line's indentation to the row text.
		lead := r.lineStart + len(d.src[r.lineStart:r.start]) - len(strings.TrimLeft(string(d.src[r.lineStart:r.start]), " \t"))
		return d.splice(lead, r.start, "", check)
	}
	return d.splice(r.start, r.start, "# ", check)
}

// EnsureSection adds an empty section name to entry n when missing (and
// an `HTTP *` response for a response section).
func EnsureSection(name string, src []byte, n int, section Section) (*Result, error) {
	info, ok := sectionInfo[section]
	if !ok || info.name == "" {
		return nil, fmt.Errorf("syntaxedit: %q has no section header", section)
	}
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	if en.sections[section] != nil {
		return &Result{Source: src}, nil
	}
	at, text := d.rowInsertion(en, section)
	return d.splice(at, at, text, both(sameEntries(d), func(nd *doc) error {
		if nd.entries[n-1].sections[section] == nil {
			return fmt.Errorf("%w: section %s", ErrInvalid, section)
		}
		return nil
	}))
}

// RemoveSection removes section name of entry n, its header and rows
// (disabled ones too); a missing section is left as is.
func RemoveSection(name string, src []byte, n int, section Section) (*Result, error) {
	info, ok := sectionInfo[section]
	if !ok || info.name == "" {
		return nil, fmt.Errorf("syntaxedit: %q has no section header", section)
	}
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	s := en.sections[section]
	if s == nil {
		return &Result{Source: src}, nil
	}
	return d.splice(s.headerStart, s.end, "", both(sameEntries(d), func(nd *doc) error {
		if nd.entries[n-1].sections[section] != nil {
			return fmt.Errorf("%w: section %s is still there", ErrInvalid, section)
		}
		return nil
	}))
}

// SetBody sets the request body of entry n to body, source text (JSON, a
// multiline string, ...); an empty body removes it.
func SetBody(name string, src []byte, n int, body string) (*Result, error) {
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	check := both(sameEntries(d), func(nd *doc) error {
		b := nd.entries[n-1].e.Request.Body
		switch {
		case body == "" && b != nil, body != "" && (b == nil || nd.text(b.Span) != d.eol(body)):
			return fmt.Errorf("%w: the body does not read back as written", ErrInvalid)
		}
		return nil
	})
	if b := en.e.Request.Body; b != nil {
		if body == "" {
			return d.splice(d.lineStart(b.Span.Start.Offset), ltEnd(b.LineTerminator0), "", check)
		}
		return d.splice(b.Span.Start.Offset, b.Span.End.Offset, body, check)
	}
	if body == "" {
		return &Result{Source: src}, nil
	}
	at, lead := d.insertionPoint(en.reqEnd)
	return d.splice(at, at, lead+body+"\n", check)
}

// AddAssert appends an assert (source text, e.g. `jsonpath "$.id" == 1`)
// to entry n.
func AddAssert(name string, src []byte, n int, assert string) (*Result, error) {
	return AddRow(name, src, n, Asserts, "", assert)
}

// AddCapture appends a capture `name: query...` (source text) to entry n.
func AddCapture(name string, src []byte, n int, capture, query string) (*Result, error) {
	return AddRow(name, src, n, Captures, capture, query)
}

// AddEntry appends entries built from specs to the file.
func AddEntry(name string, src []byte, specs ...syntax.EntrySpec) (*Result, error) {
	d, err := parse(name, src)
	if err != nil {
		return nil, err
	}
	text, err := buildEntries(name, specs)
	if err != nil {
		return nil, err
	}
	at, lead := d.insertionPoint(len(src))
	if len(src) > 0 {
		lead += "\n"
	}
	want := len(d.entries) + len(specs)
	return d.splice(at, at, lead+text, func(nd *doc) error {
		if len(nd.entries) != want {
			return fmt.Errorf("%w: %d entries, not %d", ErrInvalid, len(nd.entries), want)
		}
		return nil
	})
}

// buildEntries renders specs with the file's dialect.
func buildEntries(name string, specs []syntax.EntrySpec) (string, error) {
	f, err := syntax.BuildFile(specs, syntax.DialectFor(name))
	if err != nil {
		return "", err
	}
	return string(syntax.Format(f)), nil
}

// RemoveEntry removes entry n with the comment lines directly above it,
// up to the next entry's own comment lines (the last entry: from the end
// of the previous one).
func RemoveEntry(name string, src []byte, n int) (*Result, error) {
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	start, end := d.firstLineOf(en), len(src)
	switch {
	case n < len(d.entries):
		end = d.firstLineOf(d.entries[n])
	case n > 1:
		start = d.entries[n-2].end
	}
	want := len(d.entries) - 1
	return d.splice(start, end, "", func(nd *doc) error {
		if len(nd.entries) != want {
			return fmt.Errorf("%w: %d entries, not %d", ErrInvalid, len(nd.entries), want)
		}
		return nil
	})
}

// firstLineOf is the start of the comment lines directly above en's
// method (no blank line between).
func (d *doc) firstLineOf(en *entry) int { return d.firstLineAbove(en.start) }

// RowInsert is a row an edit made elsewhere (an import suggestion) adds
// to a file: Key and Value are source text, rendered like AddRow's.
type RowInsert struct {
	Entry      int
	Section    Section
	Key, Value string
}

// applicable are the sections ApplyEdits adds rows to: never [Options],
// [Multipart] or [BasicAuth], whose rows can reroute requests, read or
// write files, or send credentials.
var applicable = map[Section]bool{
	Headers: true, Query: true, Form: true, Cookies: true,
	ResponseHeaders: true, Captures: true, Asserts: true,
}

// ApplyEdits adds rows to a file, each rendered and checked like AddRow
// (one more row in its section, the same entries), in order. It is the
// only way an edit made elsewhere (an import suggestion) reaches a file.
// The result has one edit from src.
func ApplyEdits(name string, src []byte, rows []RowInsert) (*Result, error) {
	d, err := parse(name, src)
	if err != nil {
		return nil, err
	}
	out := src
	for _, r := range rows {
		if !applicable[r.Section] {
			return nil, fmt.Errorf("%w: rows are not added to the %s section from elsewhere", ErrInvalid, r.Section)
		}
		res, err := AddRow(name, out, r.Entry, r.Section, r.Key, r.Value)
		if err != nil {
			return nil, err
		}
		out = res.Source
	}
	// One edit: the part of src that changed.
	pre := 0
	for pre < len(src) && pre < len(out) && src[pre] == out[pre] {
		pre++
	}
	post := 0
	for post < len(src)-pre && post < len(out)-pre && src[len(src)-1-post] == out[len(out)-1-post] {
		post++
	}
	for pre > 0 && pre < len(src) && !utf8Start(src[pre]) { // keep the edit on rune boundaries
		pre--
	}
	for post > 0 && !utf8Start(src[len(src)-post]) {
		post--
	}
	edit := TextEdit{Range: d.off.rng(pre, len(src)-post), NewText: string(out[pre : len(out)-post])}
	return &Result{Source: out, Edits: []TextEdit{edit}}, nil
}

// utf8Start reports whether b starts a UTF-8 sequence.
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
