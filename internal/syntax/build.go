// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Build constructs request files from plain data, for importers that must
// never hand-assemble source strings themselves. It renders a canonical
// source text for the given entries and parses it with Parse, so the result
// always parses and round-trips through Print and Format.

// TextElem is one part of a Text value: a literal chunk or a {{variable}}
// placeholder. Use Lit and Var to build one.
type TextElem struct {
	lit   string
	name  string
	isVar bool
}

// Lit is a literal chunk of a Text value. Its content is escaped for
// whichever context the Text ends up in (URL, key, JSON string, ...); the
// caller passes the decoded value, never pre-escaped source.
func Lit(s string) TextElem { return TextElem{lit: s} }

// Var is a {{name}} placeholder in a Text value.
func Var(name string) TextElem { return TextElem{name: name, isVar: true} }

// Text is a value made of literal parts and {{variable}} placeholders, e.g.
// Text{Lit("Bearer "), Var("token")}.
type Text []TextElem

// PlainText is shorthand for Text{Lit(s)}.
func PlainText(s string) Text { return Text{Lit(s)} }

// atom is one decoded rune or placeholder in a Text, flattened so the
// escaper can look ahead across Lit/Var boundaries.
type atom struct {
	r      rune
	name   string
	isVar  bool
	forced bool // must be written as \u{XXXX} regardless of context safety
}

func atomize(t Text) []atom {
	var atoms []atom
	for _, e := range t {
		if e.isVar {
			atoms = append(atoms, atom{isVar: true, name: e.name})
			continue
		}
		for _, r := range e.lit {
			atoms = append(atoms, atom{r: r})
		}
	}
	return atoms
}

// escaper renders a Text into the source text of one grammar context
// (unquoted value, quoted string, key-string, filename, ...).
type escaper struct {
	// safe reports whether r may appear raw in this context.
	safe func(r rune) bool
	// short maps a rune needing escape to its backslash sequence; runes
	// missing from short fall back to a \u{XXXX} escape.
	short map[rune]string
	// pairBraces additionally forces a literal '{' immediately followed by
	// another '{' (raw or the start of a placeholder) to be escaped, so it
	// can never be misread as opening a {{ }} placeholder. Only needed
	// where '{' is otherwise a safe raw character.
	pairBraces bool
	// forceEdgeSpaces forces a leading or trailing run of literal spaces to
	// be escaped: the unquoted-value grammar (URL, header/query/form/
	// cookie/basic-auth value) has no delimiter of its own, so a raw
	// leading space is swallowed by the mandatory separator before it and a
	// raw trailing space is silently dropped as trailing whitespace.
	forceEdgeSpaces bool
	// unicodeEscape renders a forced or unsafe rune as this context's \u
	// escape; it defaults to the bracketed \u{XXXX} form.
	unicodeEscape func(b *strings.Builder, r rune)
	// noLeadingBracket forces a leading '[' to be escaped: keyString and
	// filename both reject a literal value starting with '[' (it would be
	// ambiguous with a `[Section]` header).
	noLeadingBracket bool
}

// writeBracedUnicode is the \u{XXXX} escape used by every context except
// JSON strings.
func writeBracedUnicode(b *strings.Builder, r rune) { fmt.Fprintf(b, `\u{%X}`, r) }

// writeJSONUnicode is the plain JSON \uXXXX escape (exactly 4 hex digits,
// with a surrogate pair above the BMP).
func writeJSONUnicode(b *strings.Builder, r rune) {
	if r > 0xFFFF {
		r -= 0x10000
		fmt.Fprintf(b, `\u%04X\u%04X`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
		return
	}
	fmt.Fprintf(b, `\u%04X`, r)
}

func (e escaper) write(b *strings.Builder, t Text) {
	atoms := atomize(t)
	if e.pairBraces {
		for i := range atoms {
			if atoms[i].isVar || atoms[i].r != '{' {
				continue
			}
			if next := atoms[i+1:]; len(next) > 0 && (next[0].isVar || next[0].r == '{') {
				atoms[i].forced = true
			}
		}
	}
	if e.forceEdgeSpaces {
		for i := 0; i < len(atoms) && !atoms[i].isVar && atoms[i].r == ' '; i++ {
			atoms[i].forced = true
		}
		for i := len(atoms) - 1; i >= 0 && !atoms[i].isVar && atoms[i].r == ' '; i-- {
			atoms[i].forced = true
		}
	}
	if e.noLeadingBracket && len(atoms) > 0 && !atoms[0].isVar && atoms[0].r == '[' {
		atoms[0].forced = true
	}
	for _, a := range atoms {
		if a.isVar {
			b.WriteString("{{")
			b.WriteString(a.name)
			b.WriteString("}}")
			continue
		}
		e.writeRune(b, a.r, a.forced)
	}
}

func (e escaper) writeRune(b *strings.Builder, r rune, forced bool) {
	if !forced {
		if e.safe(r) {
			b.WriteRune(r)
			return
		}
		if seq, ok := e.short[r]; ok {
			b.WriteString(seq)
			return
		}
	}
	esc := e.unicodeEscape
	if esc == nil {
		esc = writeBracedUnicode
	}
	esc(b, r)
}

// controlSafe returns a safe predicate for the anyChar-based contexts
// (unquoted, quoted, backtick, JSON string): any printable rune except
// backslash, DEL and the context's own delimiter(s) in exceptSet.
func controlSafe(exceptSet string) func(r rune) bool {
	return func(r rune) bool {
		return r >= 0x20 && r != 0x7f && r != '\\' && !strings.ContainsRune(exceptSet, r)
	}
}

func controlShort(extra map[rune]string) map[rune]string {
	m := map[rune]string{'\\': `\\`, '\b': `\b`, '\n': `\n`, '\f': `\f`, '\r': `\r`, '\t': `\t`}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

var (
	// unquotedEscaper is used for the URL and for header/query/form/
	// cookie/basic-auth values, content types and [Options] string values.
	unquotedEscaper = escaper{
		safe: controlSafe("#"), short: controlShort(map[rune]string{'#': `\#`}),
		pairBraces: true, forceEdgeSpaces: true,
	}
	// jsonStringEscaper is used for JSON string values and member names.
	jsonStringEscaper = escaper{
		safe: controlSafe(`"`), short: controlShort(map[rune]string{'"': `\"`}),
		pairBraces: true, unicodeEscape: writeJSONUnicode,
	}
	// backtickEscaper is used for the oneline backtick body fallback.
	backtickEscaper = escaper{
		safe: controlSafe("`"), short: controlShort(map[rune]string{'`': "\\`"}),
		pairBraces: true,
	}
	// keyEscaper is used for header/query/form/cookie keys and capture
	// names: a much smaller safe set (no raw space, no raw '{').
	keyEscaper = escaper{
		safe: func(r rune) bool { return isAlphanumeric(r) || strings.ContainsRune("_-.[]@$", r) },
		short: map[rune]string{
			'#': `\#`, ':': `\:`, '\\': `\\`, '/': `\/`,
			'\b': `\b`, '\f': `\f`, '\n': `\n`, '\r': `\r`, '\t': `\t`,
		},
		noLeadingBracket: true,
	}
	// filenameEscaper is used for file names (body, multipart).
	filenameEscaper = escaper{
		safe: func(r rune) bool { return !strings.ContainsRune("#;{} \n\r\\", r) },
		short: map[rune]string{
			'\\': `\\`, '#': `\#`, ';': `\;`, ' ': `\ `, '{': `\{`, '}': `\}`,
			'\n': `\n`, '\r': `\r`,
		},
		noLeadingBracket: true,
	}
)

// Field is a `key: value` line: a header, a query/form/cookie param, or the
// [BasicAuth] user/password pair.
type Field struct{ Key, Value Text }

// KV is shorthand for Field{PlainText(key), PlainText(value)}.
func KV(key, value string) Field { return Field{PlainText(key), PlainText(value)} }

func writeField(b *strings.Builder, f Field) {
	keyEscaper.write(b, f.Key)
	b.WriteString(": ")
	unquotedEscaper.write(b, f.Value)
	b.WriteByte('\n')
}

func writeSection(b *strings.Builder, name string, fields []Field) {
	if len(fields) == 0 {
		return
	}
	b.WriteString("[" + name + "]\n")
	for _, f := range fields {
		writeField(b, f)
	}
}

// MultipartFile is a `key: file,name; type` line in [Multipart].
type MultipartFile struct {
	Name        Text
	ContentType Text // empty means no content type is written
}

// MultipartField is one line in [Multipart]: a text field (Value set) or a
// file field (File set).
type MultipartField struct {
	Key   Text
	Value Text
	File  *MultipartFile
}

func writeMultipart(b *strings.Builder, m MultipartField) {
	keyEscaper.write(b, m.Key)
	b.WriteString(": ")
	if m.File != nil {
		b.WriteString("file,")
		filenameEscaper.write(b, m.File.Name)
		b.WriteByte(';')
		if len(m.File.ContentType) > 0 {
			b.WriteByte(' ')
			unquotedEscaper.write(b, m.File.ContentType)
		}
	} else {
		unquotedEscaper.write(b, m.Value)
	}
	b.WriteByte('\n')
}

// BasicAuth is the one user:password pair of [BasicAuth].
type BasicAuth struct{ User, Password Text }

// OptionField is one line in [Options]. Name is the option's grammar name
// (e.g. "compressed", "retry", "variable"); RawValue is its already
// formatted literal (e.g. "true", "3", "500ms", `foo="bar"`) since the
// option shapes are too varied to model individually here.
type OptionField struct{ Name, RawValue string }

// BodySpec is a request or response body, already rendered to its literal
// source form.
type BodySpec struct{ src string }

// Members is an ordered list of JSON object members, for JSONBody's v.
type Members []Member

// Member is one ordered "key": value pair of Members.
type Member struct {
	Key   string
	Value any
}

var jsonNumberRE = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// JSONBody builds a JSON body from v, which must be built from nil, bool,
// float64, json.Number, string, Text (a string with placeholders), Members
// or map[string]any (sorted by key) and []any, nested as needed.
func JSONBody(v any) (*BodySpec, error) {
	var b strings.Builder
	if err := writeJSONValue(&b, v); err != nil {
		return nil, err
	}
	return &BodySpec{src: b.String()}, nil
}

func writeJSONValue(b *strings.Builder, v any) error {
	switch val := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(val))
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return fmt.Errorf("syntax: JSON number must be finite, got %v", val)
		}
		b.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
	case json.Number:
		if !jsonNumberRE.MatchString(string(val)) {
			return fmt.Errorf("syntax: invalid JSON number %q", val)
		}
		b.WriteString(string(val))
	case string:
		writeJSONString(b, PlainText(val))
	case Text:
		writeJSONString(b, val)
	case Members:
		return writeJSONObject(b, val)
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		members := make(Members, len(keys))
		for i, k := range keys {
			members[i] = Member{Key: k, Value: val[k]}
		}
		return writeJSONObject(b, members)
	case []any:
		return writeJSONArray(b, val)
	default:
		return fmt.Errorf("syntax: unsupported JSON value type %T", v)
	}
	return nil
}

func writeJSONString(b *strings.Builder, t Text) {
	b.WriteByte('"')
	jsonStringEscaper.write(b, t)
	b.WriteByte('"')
}

func writeJSONObject(b *strings.Builder, members Members) error {
	b.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			b.WriteString(", ")
		}
		writeJSONString(b, PlainText(m.Key))
		b.WriteString(": ")
		if err := writeJSONValue(b, m.Value); err != nil {
			return fmt.Errorf("member %q: %w", m.Key, err)
		}
	}
	b.WriteByte('}')
	return nil
}

func writeJSONArray(b *strings.Builder, elems []any) error {
	b.WriteByte('[')
	for i, e := range elems {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := writeJSONValue(b, e); err != nil {
			return fmt.Errorf("element %d: %w", i, err)
		}
	}
	b.WriteByte(']')
	return nil
}

// RawTextBody builds a multiline body from text tagged with lang (e.g.
// "graphql"); lang == "" selects the untemplated ```raw``` form, which
// accepts any text but never expands {{ }} placeholders. It falls back to a
// oneline backtick string when text contains the closing fence, or (for a
// templated lang) could be misread as a {{ }} placeholder or, for graphql, a
// trailing `variables` block; it falls back to a base64 body when text is
// not valid UTF-8.
func RawTextBody(text, lang string) *BodySpec {
	if !utf8.ValidString(text) {
		return BytesBody([]byte(text))
	}
	templated := lang != ""
	unsafe := strings.Contains(text, "```") ||
		(templated && strings.Contains(text, "{{")) ||
		(lang == "graphql" && (text == "variables" || strings.HasPrefix(text, "variables") &&
			(len(text) == len("variables") || !isNameChar(rune(text[len("variables")]))) ||
			strings.Contains(text, "\nvariables")))
	if !unsafe {
		hint := lang
		if hint == "" {
			hint = "raw"
		}
		var b strings.Builder
		b.WriteString("```")
		b.WriteString(hint)
		b.WriteByte('\n')
		b.WriteString(text)
		b.WriteString("```")
		return &BodySpec{src: b.String()}
	}
	var b strings.Builder
	b.WriteByte('`')
	backtickEscaper.write(&b, PlainText(text))
	b.WriteByte('`')
	return &BodySpec{src: b.String()}
}

// FileBody is a `file,path;` body.
func FileBody(path Text) *BodySpec {
	var b strings.Builder
	b.WriteString("file,")
	filenameEscaper.write(&b, path)
	b.WriteByte(';')
	return &BodySpec{src: b.String()}
}

// BytesBody is a `base64,...;` body holding raw bytes.
func BytesBody(data []byte) *BodySpec {
	return &BodySpec{src: "base64," + base64.StdEncoding.EncodeToString(data) + ";"}
}

// ResponseSpec is an entry's expected response: its status only (as digits,
// or "*" for any status); assertions are not built here.
type ResponseSpec struct{ Status string }

// EntrySpec describes one request, and optionally its expected response, to
// render with BuildFile.
type EntrySpec struct {
	// Comments are written as one or more `#` lines above the request.
	Comments  []string
	Method    string
	URL       Text
	Headers   []Field
	Query     []Field
	Form      []Field
	Cookies   []Field
	Multipart []MultipartField
	BasicAuth *BasicAuth
	Options   []OptionField
	Body      *BodySpec
	Response  *ResponseSpec
}

// BuildFile renders entries to canonical source text and parses it with the
// given dialect, so the result always parses and round-trips through Print
// and Format.
func BuildFile(entries []EntrySpec, d Dialect) (*File, error) {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteByte('\n')
		}
		writeEntry(&b, e)
	}
	src := b.String()
	f, err := Parse("<build>", []byte(src), d)
	if err != nil {
		return nil, fmt.Errorf("syntax: BuildFile produced invalid source: %w\n%s", err, src)
	}
	return f, nil
}

func writeEntry(b *strings.Builder, e EntrySpec) {
	for _, c := range e.Comments {
		for line := range strings.SplitSeq(c, "\n") {
			b.WriteByte('#')
			if line = strings.TrimRight(line, " \t"); line != "" {
				b.WriteByte(' ')
				b.WriteString(line)
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString(e.Method)
	b.WriteByte(' ')
	unquotedEscaper.write(b, e.URL)
	b.WriteByte('\n')
	for _, h := range e.Headers {
		writeField(b, h)
	}
	writeSection(b, "Query", e.Query)
	writeSection(b, "Form", e.Form)
	writeSection(b, "Cookies", e.Cookies)
	if len(e.Multipart) > 0 {
		b.WriteString("[Multipart]\n")
		for _, m := range e.Multipart {
			writeMultipart(b, m)
		}
	}
	if e.BasicAuth != nil {
		b.WriteString("[BasicAuth]\n")
		writeField(b, Field{Key: e.BasicAuth.User, Value: e.BasicAuth.Password})
	}
	if len(e.Options) > 0 {
		b.WriteString("[Options]\n")
		for _, o := range e.Options {
			b.WriteString(o.Name)
			b.WriteString(": ")
			b.WriteString(o.RawValue)
			b.WriteByte('\n')
		}
	}
	if e.Body != nil {
		b.WriteString(e.Body.src)
		b.WriteByte('\n')
	}
	if e.Response != nil {
		b.WriteString("HTTP ")
		b.WriteString(e.Response.Status)
		b.WriteByte('\n')
	}
}
