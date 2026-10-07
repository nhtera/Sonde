// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxexport

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/nhtera/sonde/internal/syntax"
)

// JSON renders f's AST as the reference formatter's JSON document: one
// line, keys in a fixed order, no whitespace.
func JSON(f *syntax.File) string {
	entries := make(jList, 0, len(f.Entries))
	for _, e := range f.Entries {
		entries = append(entries, entryJSON(e))
	}
	var b strings.Builder
	jObject{{"entries", entries}}.write(&b)
	return b.String()
}

// A small ordered JSON model: the reference output keeps insertion order,
// which encoding/json maps cannot.
type (
	jValue  interface{ write(b *strings.Builder) }
	jString string
	jNumber string // written as is
	jBool   bool
	jNull   struct{}
	jList   []jValue
	jObject []jMember
	jMember struct {
		name  string
		value jValue
	}
)

func (s jString) write(b *strings.Builder) { writeJSONString(b, string(s)) }
func (n jNumber) write(b *strings.Builder) { b.WriteString(string(n)) }
func (v jBool) write(b *strings.Builder)   { b.WriteString(strconv.FormatBool(bool(v))) }
func (jNull) write(b *strings.Builder)     { b.WriteString("null") }

func (l jList) write(b *strings.Builder) {
	b.WriteByte('[')
	for i, v := range l {
		if i > 0 {
			b.WriteByte(',')
		}
		v.write(b)
	}
	b.WriteByte(']')
}

func (o jObject) write(b *strings.Builder) {
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSONString(b, m.name)
		b.WriteByte(':')
		m.value.write(b)
	}
	b.WriteByte('}')
}

// writeJSONString escapes s as the reference serializer does: short
// escapes for quote, backslash, \b, \f, \n, \r and \t, \u00XX (lower-case)
// for any other control character, everything else as is.
func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if unicode.IsControl(r) {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

// text is a template's value: literal parts decoded, placeholders as
// `{{name}}`.
func text(t *syntax.Template) string {
	if t == nil {
		return ""
	}
	var b strings.Builder
	for _, e := range t.Elements {
		switch e := e.(type) {
		case *syntax.TemplateString:
			b.WriteString(e.Value)
		case *syntax.Placeholder:
			b.WriteString("{{" + e.Expr.Name + "}}")
		}
	}
	return b.String()
}

// source prints n back as written.
func source(n syntax.Node) string { return syntax.PrintNode(n) }

func entryJSON(e *syntax.Entry) jValue {
	o := jObject{{"request", requestJSON(e.Request)}}
	if e.Response != nil {
		o = append(o, jMember{"response", responseJSON(e.Response)})
	}
	return o
}

// firstSection returns the first section of kind, as the reference
// exporter reads only that one.
func firstSection(sections []*syntax.Section, kind syntax.SectionKind) *syntax.Section {
	for _, s := range sections {
		if s.Kind == kind {
			return s
		}
	}
	return nil
}

func keyValuesJSON(kvs []*syntax.KeyValue) jList {
	l := make(jList, 0, len(kvs))
	for _, kv := range kvs {
		l = append(l, jObject{{"name", jString(text(kv.Key))}, {"value", jString(text(kv.Value))}})
	}
	return l
}

func requestJSON(r *syntax.Request) jValue {
	o := jObject{{"method", jString(r.Method.Value)}, {"url", jString(text(r.URL))}}
	if len(r.Headers) > 0 {
		o = append(o, jMember{"headers", keyValuesJSON(r.Headers)})
	}
	if s := firstSection(r.Sections, syntax.SectionQueryParams); s != nil && len(s.KeyValues) > 0 {
		o = append(o, jMember{"query_string_params", keyValuesJSON(s.KeyValues)})
	}
	if s := firstSection(r.Sections, syntax.SectionFormParams); s != nil && len(s.KeyValues) > 0 {
		o = append(o, jMember{"form_params", keyValuesJSON(s.KeyValues)})
	}
	if s := firstSection(r.Sections, syntax.SectionMultipart); s != nil && len(s.Multipart) > 0 {
		var l jList
		for _, m := range s.Multipart {
			switch m := m.(type) {
			case *syntax.KeyValue:
				l = append(l, jObject{{"name", jString(text(m.Key))}, {"value", jString(text(m.Value))}})
			case *syntax.FilenameParam:
				p := jObject{{"name", jString(text(m.Key))}, {"filename", jString(text(m.Value.Filename))}}
				if m.Value.ContentType != nil {
					p = append(p, jMember{"content_type", jString(text(m.Value.ContentType))})
				}
				l = append(l, p)
			}
		}
		o = append(o, jMember{"multipart_form_data", l})
	}
	if s := firstSection(r.Sections, syntax.SectionCookies); s != nil && len(s.KeyValues) > 0 {
		o = append(o, jMember{"cookies", keyValuesJSON(s.KeyValues)})
	}
	if s := firstSection(r.Sections, syntax.SectionOptions); s != nil && len(s.Options) > 0 {
		l := make(jList, 0, len(s.Options))
		for _, opt := range s.Options {
			l = append(l, optionJSON(opt))
		}
		o = append(o, jMember{"options", l})
	}
	if r.Body != nil {
		o = append(o, jMember{"body", bodyJSON(r.Body.Value)})
	}
	var comments jList
	for _, lt := range r.LineTerminators {
		if lt.Comment != nil {
			comments = append(comments, jString(lt.Comment.Value))
		}
	}
	if len(comments) > 0 {
		o = append(o, jMember{"comments", comments})
	}
	return o
}

func responseJSON(r *syntax.Response) jValue {
	var o jObject
	if v := r.Version.Value; v != "HTTP" {
		o = append(o, jMember{"version", jString(v)})
	}
	if s := r.Status.Value; s != "*" {
		// A number, normalized as the reference does: `00` is 0.
		if n, err := strconv.ParseUint(s, 10, 64); err == nil {
			s = strconv.FormatUint(n, 10)
		}
		o = append(o, jMember{"status", jNumber(s)})
	}
	if len(r.Headers) > 0 {
		o = append(o, jMember{"headers", keyValuesJSON(r.Headers)})
	}
	if s := firstSection(r.Sections, syntax.SectionCaptures); s != nil && len(s.Captures) > 0 {
		l := make(jList, 0, len(s.Captures))
		for _, c := range s.Captures {
			l = append(l, captureJSON(c))
		}
		o = append(o, jMember{"captures", l})
	}
	if s := firstSection(r.Sections, syntax.SectionAsserts); s != nil && len(s.Asserts) > 0 {
		l := make(jList, 0, len(s.Asserts))
		for _, a := range s.Asserts {
			l = append(l, assertJSON(a))
		}
		o = append(o, jMember{"asserts", l})
	}
	if r.Body != nil {
		o = append(o, jMember{"body", bodyJSON(r.Body.Value)})
	}
	return o
}

func bytesBase64(v []byte) jValue {
	return jObject{{"encoding", jString("base64")}, {"value", jString(base64.StdEncoding.EncodeToString(v))}}
}

func fileJSON(f *syntax.FileRef) jValue {
	return jObject{{"type", jString("file")}, {"filename", jString(text(f.Filename))}}
}

var multilineTypes = map[syntax.MultilineKind]string{
	syntax.MultilineText: "text", syntax.MultilineRaw: "raw", syntax.MultilineJSON: "json",
	syntax.MultilineXML: "xml", syntax.MultilineGraphQL: "graphql",
}

// multilineText is a multiline string's value; a GraphQL query keeps its
// `variables {...}` block as written.
func multilineText(m *syntax.MultilineString) string {
	s := text(m.Value)
	if v := m.Variables; v != nil {
		s += "variables" + v.Space.Value + source(v.Value) + v.Whitespace.Value
	}
	return s
}

func bodyJSON(v syntax.Bytes) jValue {
	switch v := v.(type) {
	case *syntax.Base64:
		return bytesBase64(v.Value)
	case *syntax.Hex:
		return bytesBase64(v.Value)
	case *syntax.FileRef:
		return fileJSON(v)
	case *syntax.XML:
		return jObject{{"type", jString("xml")}, {"value", jString(v.Value)}}
	case *syntax.Template:
		if v.Delimiter == '"' { // a JSON string body
			return jObject{{"type", jString("json")}, {"value", jString(text(v))}}
		}
		return jObject{{"type", jString("text")}, {"value", jString(text(v))}}
	case *syntax.MultilineString:
		return jObject{{"type", jString(multilineTypes[v.Kind])}, {"value", jString(multilineText(v))}}
	default:
		return jObject{{"type", jString("json")}, {"value", jsonValue(v)}}
	}
}

func jsonValue(n syntax.Node) jValue {
	switch n := n.(type) {
	case *syntax.Null:
		return jNull{}
	case *syntax.Boolean:
		return jBool(n.Value)
	case *syntax.JSONNumber:
		// As written, like the reference; a form JSON itself rejects
		// (e.g. `0e`) is written as a string so the output stays valid.
		return jNumberOrString(n.Source)
	case *syntax.Template:
		return jString(text(n))
	case *syntax.Placeholder:
		return jString("{{" + n.Expr.Name + "}}")
	case *syntax.JSONList:
		l := make(jList, 0, len(n.Elements))
		for _, el := range n.Elements {
			l = append(l, jsonValue(el.Value))
		}
		return l
	case *syntax.JSONObject:
		o := make(jObject, 0, len(n.Elements))
		for _, el := range n.Elements {
			o = append(o, jMember{text(el.Name), jsonValue(el.Value)})
		}
		return o
	}
	return jNull{}
}

// numberText is a number as the reference displays it: integers and
// floats normalized (no exponent), big integers as written.
func numberText(n *syntax.Number) string {
	switch n.Kind {
	case syntax.NumberInteger:
		return strconv.FormatInt(n.Int, 10)
	case syntax.NumberFloat:
		if math.IsInf(n.Float, 0) {
			return n.Source // too large for a float; see jNumberOrString
		}
		return strconv.FormatFloat(n.Float, 'f', -1, 64)
	}
	return n.Source
}

// jNumberOrString writes s as a number when it is a valid JSON number,
// else as a string, so the document stays valid JSON (the reference can
// write `inf` or other non-JSON forms there).
func jNumberOrString(s string) jValue {
	if json.Valid([]byte(s)) {
		return jNumber(s)
	}
	return jString(s)
}

func optionJSON(o *syntax.Option) jValue {
	var value jValue
	switch v := o.Value.(type) {
	case *syntax.Template:
		value = jString(text(v))
	case *syntax.Boolean:
		value = jBool(v.Value)
	case *syntax.Placeholder:
		value = jString("{{" + v.Expr.Name + "}}")
	case *syntax.Number:
		value = jNumberOrString(numberText(v))
	case *syntax.Duration:
		if v.Unit == "" {
			value = jNumberOrString(numberText(v.Value))
		} else {
			// The unit object gets the option's name as its last member.
			return jObject{{"value", jNumberOrString(numberText(v.Value))}, {"unit", jString(v.Unit)}, {"name", jString(o.Name)}}
		}
	case *syntax.VariableDefinition:
		value = jString(v.Name + "=" + source(v.Value))
	default:
		value = jString(source(v))
	}
	return jObject{{"name", jString(o.Name)}, {"value", value}}
}

func captureJSON(c *syntax.Capture) jValue {
	o := jObject{{"name", jString(text(c.Name))}, {"query", queryJSON(c.Query)}}
	if len(c.Filters) > 0 {
		o = append(o, jMember{"filters", filtersJSON(c.Filters)})
	}
	if c.Redact {
		o = append(o, jMember{"redact", jBool(true)})
	}
	return o
}

func assertJSON(a *syntax.Assert) jValue {
	o := jObject{{"query", queryJSON(a.Query)}}
	if len(a.Filters) > 0 {
		o = append(o, jMember{"filters", filtersJSON(a.Filters)})
	}
	return append(o, jMember{"predicate", predicateJSON(a.Predicate)})
}

// regexValueJSON is a regex argument: a string for a template, an object
// for a /regex/ literal.
func regexValueJSON(n syntax.Node) jValue {
	if r, ok := n.(*syntax.Regex); ok {
		return jObject{{"type", jString("regex")}, {"value", jString(r.Pattern)}}
	}
	return jString(argText(n))
}

// argText is a template argument's value.
func argText(n syntax.Node) string {
	if t, ok := n.(*syntax.Template); ok {
		return text(t)
	}
	return source(n)
}

// cookieAttributes are the canonical names of cookie attributes.
var cookieAttributes = map[string]string{
	"value": "Value", "expires": "Expires", "max-age": "Max-Age", "domain": "Domain",
	"path": "Path", "secure": "Secure", "httponly": "HttpOnly", "samesite": "SameSite",
}

func queryJSON(q *syntax.Query) jValue {
	o := jObject{{"type", jString(q.Kind.String())}}
	switch q.Kind {
	case syntax.QueryJSONPath, syntax.QueryXPath:
		o = append(o, jMember{"expr", jString(argText(q.Arg))})
	case syntax.QueryHeader, syntax.QueryVariable:
		o = append(o, jMember{"name", jString(argText(q.Arg))})
	case syntax.QueryCookie:
		if c, ok := q.Arg.(*syntax.CookiePath); ok {
			expr := text(c.Name)
			if c.Attribute != nil {
				expr += "[" + cookieAttributes[strings.ToLower(c.Attribute.Name)] + "]"
			}
			o = append(o, jMember{"expr", jString(expr)})
		}
	case syntax.QueryRegex:
		o = append(o, jMember{"expr", regexValueJSON(q.Arg)})
	case syntax.QueryCertificate:
		if c, ok := q.Arg.(*syntax.CertificateAttribute); ok {
			o = append(o, jMember{"expr", jString(c.Name)})
		}
	}
	return o
}

func filtersJSON(items []*syntax.FilterItem) jList {
	l := make(jList, 0, len(items))
	for _, it := range items {
		f := it.Filter
		o := jObject{{"type", jString(f.Kind.String())}}
		switch f.Kind {
		case syntax.FilterDecode:
			o = append(o, jMember{"encoding", jString(argText(f.Arg))})
		case syntax.FilterFormat, syntax.FilterDateFormat, syntax.FilterToDate:
			o = append(o, jMember{"fmt", jString(argText(f.Arg))})
		case syntax.FilterJSONPath, syntax.FilterXPath:
			o = append(o, jMember{"expr", jString(argText(f.Arg))})
		case syntax.FilterNth:
			switch n := f.Arg.(type) {
			case *syntax.Number:
				o = append(o, jMember{"n", jNumberOrString(numberText(n))})
			case *syntax.Placeholder:
				// The reference writes the bare name as a number, which is
				// not valid JSON; sonde writes the placeholder as a string.
				o = append(o, jMember{"n", jString("{{" + n.Expr.Name + "}}")})
			}
		case syntax.FilterRegex:
			o = append(o, jMember{"expr", regexValueJSON(f.Arg)})
		case syntax.FilterReplace:
			o = append(o, jMember{"old_value", jString(argText(f.Arg))}, jMember{"new_value", jString(argText(f.Arg2))})
		case syntax.FilterReplaceRegex:
			o = append(o, jMember{"pattern", regexValueJSON(f.Arg)}, jMember{"new_value", jString(argText(f.Arg2))})
		case syntax.FilterSplit:
			o = append(o, jMember{"sep", jString(argText(f.Arg))})
		case syntax.FilterURLQueryParam:
			o = append(o, jMember{"param", jString(argText(f.Arg))})
		}
		// charsetDecode's encoding is not exported, as in the reference.
		l = append(l, o)
	}
	return l
}

func predicateJSON(p *syntax.Predicate) jValue {
	var o jObject
	if p.Not {
		o = append(o, jMember{"not", jBool(true)})
	}
	o = append(o, jMember{"type", jString(p.Func.Kind.String())})
	if p.Func.Value == nil {
		return o
	}
	var value jValue
	encoding := ""
	switch v := p.Func.Value.(type) {
	case *syntax.Template:
		value = jString(text(v))
	case *syntax.MultilineString:
		value = jString(text(v.Value))
	case *syntax.Boolean:
		value = jBool(v.Value)
	case *syntax.Null:
		value = jNull{}
	case *syntax.Number:
		value = jNumberOrString(numberText(v))
	case *syntax.FileRef:
		value = fileJSON(v)
	case *syntax.Hex:
		value, encoding = jString(base64.StdEncoding.EncodeToString(v.Value)), "base64"
	case *syntax.Base64:
		value, encoding = jString(base64.StdEncoding.EncodeToString(v.Value)), "base64"
	case *syntax.Placeholder:
		// The reference writes the expression without its braces here.
		value = jString(v.Expr.Name)
	case *syntax.Regex:
		value, encoding = jString(v.Pattern), "regex"
	default:
		value = jString(source(v))
	}
	o = append(o, jMember{"value", value})
	if encoding != "" {
		o = append(o, jMember{"encoding", jString(encoding)})
	}
	return o
}
