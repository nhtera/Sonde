// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strconv"
	"strings"
	"unicode"
)

// Print renders f back to source; for a parsed file the output is identical
// to the input. Literal nodes print their Source text (TemplateString,
// Number, JSONNumber, Regex, Base64, Hex), so code that builds or edits such
// nodes must set Source; a CookiePath prints its parts when Source is empty.
func Print(f *File) []byte {
	var p printer
	p.file(f)
	return []byte(p.String())
}

// Format renders f canonically. Only horizontal whitespace and line endings
// change; blank lines, comments, order and body content are kept:
//   - no indentation; one space after `:` and between assert/capture parts;
//   - trailing whitespace removed, blank lines emptied;
//   - LF line endings outside bodies and a final newline.
func Format(f *File) []byte {
	p := printer{canonical: true}
	p.file(f)
	return []byte(p.String())
}

// PrintNode renders one node back to source, as Print does.
func PrintNode(n Node) string {
	var p printer
	p.node(n)
	return p.String()
}

type printer struct {
	strings.Builder
	canonical bool
	// lint applies Lint's line rules on top of canonical: line endings
	// kept, comment lines unindented.
	lint bool
}

// ws writes w, or its canonical form in canonical mode.
func (p *printer) ws(w Whitespace, canonical string) {
	if p.canonical {
		p.WriteString(canonical)
	} else {
		p.WriteString(w.Value)
	}
}

// spaceBefore is the canonical separator before an optional value: none
// when the value is absent or an empty string.
func spaceBefore(n Node) string {
	switch n := n.(type) {
	case nil:
		return ""
	case *Template:
		if n == nil || (n.Delimiter == 0 && len(n.Elements) == 0) {
			return ""
		}
	}
	return " "
}

func (p *printer) file(f *File) {
	if f.BOM {
		p.WriteString(utf8BOM)
	}
	for _, e := range f.Entries {
		p.lineTerminators(e.Request.LineTerminators)
		p.request(e.Request)
		if e.Response != nil {
			p.response(e.Response)
		}
	}
	p.lineTerminators(f.LineTerminators)
}

func (p *printer) request(r *Request) {
	p.ws(r.Space0, "")
	p.WriteString(r.Method.Value)
	p.ws(r.Space1, " ")
	p.template(r.URL)
	p.lineTerminator(r.LineTerminator0)
	p.keyValues(r.Headers)
	p.sections(r.Sections)
	p.body(r.Body)
}

func (p *printer) response(r *Response) {
	p.lineTerminators(r.LineTerminators)
	p.ws(r.Space0, "")
	p.WriteString(r.Version.Value)
	p.ws(r.Space1, " ")
	p.WriteString(r.Status.Value)
	p.lineTerminator(r.LineTerminator0)
	p.keyValues(r.Headers)
	p.sections(r.Sections)
	p.body(r.Body)
}

func (p *printer) body(b *Body) {
	if b == nil {
		return
	}
	p.lineTerminators(b.LineTerminators)
	p.ws(b.Space0, "")
	p.node(b.Value)
	p.lineTerminator(b.LineTerminator0)
}

// lineTerminators prints the blank or comment-only lines above an item.
func (p *printer) lineTerminators(lts []*LineTerminator) {
	for _, lt := range lts {
		if p.lint {
			p.lintLineTerminator(lt, false)
		} else {
			p.lineTerminator(lt)
		}
	}
}

// lineTerminator prints the end of an item's line.
func (p *printer) lineTerminator(lt *LineTerminator) {
	if lt == nil {
		return
	}
	if p.lint {
		p.lintLineTerminator(lt, true)
		return
	}
	if !p.canonical {
		p.WriteString(lt.Space0.Value)
		if lt.Comment != nil {
			p.WriteByte('#')
			p.WriteString(lt.Comment.Value)
		}
		p.WriteString(lt.Newline.Value)
		return
	}
	if lt.Comment != nil {
		p.WriteString(lt.Space0.Value)
		p.WriteByte('#')
		p.WriteString(strings.TrimRight(lt.Comment.Value, " \t\r"))
	}
	p.WriteByte('\n')
}

// lintLineTerminator prints lt the way Lint does: a trailing comment keeps
// the spaces before it, a comment line does not; the line ending is kept.
func (p *printer) lintLineTerminator(lt *LineTerminator, trailing bool) {
	if lt.Comment != nil {
		if trailing {
			p.WriteString(lt.Space0.Value)
		}
		p.WriteByte('#')
		p.WriteString(strings.TrimRightFunc(lt.Comment.Value, unicode.IsSpace))
	}
	if lt.Newline.Value == "" {
		p.WriteByte('\n')
	} else {
		p.WriteString(lt.Newline.Value)
	}
}

func (p *printer) keyValues(kvs []*KeyValue) {
	for _, kv := range kvs {
		p.keyValue(kv, false)
	}
}

// keyValue prints kv. In lint mode an empty value prints as `key:`, except
// for a cookie (cookie), which keeps the space: `name: `.
func (p *printer) keyValue(kv *KeyValue, cookie bool) {
	p.lineTerminators(kv.LineTerminators)
	p.ws(kv.Space0, "")
	p.template(kv.Key)
	p.ws(kv.Space1, "")
	p.WriteByte(':')
	if p.lint && kv.Value != nil && len(kv.Value.Elements) == 0 {
		if cookie {
			p.WriteByte(' ')
		}
		p.lineTerminator(kv.LineTerminator0)
		return
	}
	p.ws(kv.Space2, spaceBefore(kv.Value))
	p.template(kv.Value)
	p.lineTerminator(kv.LineTerminator0)
}

func (p *printer) sections(sections []*Section) {
	for _, s := range sections {
		p.lineTerminators(s.LineTerminators)
		p.ws(s.Space0, "")
		p.WriteString("[" + s.Name + "]")
		p.lineTerminator(s.LineTerminator0)
		for _, kv := range s.KeyValues {
			p.keyValue(kv, s.Kind == SectionCookies)
		}
		for _, m := range s.Multipart {
			switch m := m.(type) {
			case *KeyValue:
				p.keyValue(m, false)
			case *FilenameParam:
				p.filenameParam(m)
			}
		}
		for _, o := range s.Options {
			p.option(o)
		}
		for _, c := range s.Captures {
			p.capture(c)
		}
		for _, a := range s.Asserts {
			p.assert(a)
		}
		for _, m := range s.Messages {
			p.messageStep(m)
		}
	}
}

func (p *printer) messageStep(s *MessageStep) {
	p.lineTerminators(s.LineTerminators)
	p.ws(s.Space0, "")
	p.WriteString(s.Kind.String())
	if s.Colon {
		p.ws(s.Space1, "")
		p.WriteByte(':')
		p.ws(s.Space2, " ")
		p.node(s.Value)
	}
	p.lineTerminator(s.LineTerminator0)
}

func (p *printer) filenameParam(fp *FilenameParam) {
	p.lineTerminators(fp.LineTerminators)
	p.ws(fp.Space0, "")
	p.template(fp.Key)
	p.ws(fp.Space1, "")
	p.WriteByte(':')
	p.ws(fp.Space2, " ")
	v := fp.Value
	p.WriteString("file,")
	p.ws(v.Space0, "")
	p.template(v.Filename)
	p.ws(v.Space1, "")
	p.WriteByte(';')
	p.ws(v.Space2, spaceBefore(v.ContentType))
	if v.ContentType != nil {
		p.template(v.ContentType)
	}
	p.lineTerminator(fp.LineTerminator0)
}

func (p *printer) option(o *Option) {
	p.lineTerminators(o.LineTerminators)
	p.ws(o.Space0, "")
	p.WriteString(o.Name)
	p.ws(o.Space1, "")
	p.WriteByte(':')
	if p.lint {
		p.WriteByte(' ') // even before an empty value
	} else {
		p.ws(o.Space2, spaceBefore(o.Value))
	}
	p.node(o.Value)
	p.lineTerminator(o.LineTerminator0)
}

func (p *printer) capture(c *Capture) {
	p.lineTerminators(c.LineTerminators)
	p.ws(c.Space0, "")
	p.template(c.Name)
	p.ws(c.Space1, "")
	p.WriteByte(':')
	p.ws(c.Space2, " ")
	p.query(c.Query)
	p.filters(c.Filters)
	if c.Redact {
		p.ws(c.Space3, " ")
		p.WriteString("redact")
	}
	p.lineTerminator(c.LineTerminator0)
}

func (p *printer) assert(a *Assert) {
	p.lineTerminators(a.LineTerminators)
	p.ws(a.Space0, "")
	p.query(a.Query)
	p.filters(a.Filters)
	p.ws(a.Space1, " ")
	if a.Predicate.Not {
		p.WriteString("not")
		p.ws(a.Predicate.Space0, " ")
	}
	f := a.Predicate.Func
	p.WriteString(f.Kind.String())
	if f.Value != nil {
		p.ws(f.Space0, " ")
		p.node(f.Value)
	}
	p.lineTerminator(a.LineTerminator0)
}

func (p *printer) query(q *Query) {
	p.WriteString(q.Kind.String())
	if q.Arg != nil {
		p.ws(q.Space0, " ")
		p.node(q.Arg)
	}
}

func (p *printer) filters(items []*FilterItem) {
	for _, it := range items {
		p.ws(it.Space, " ")
		f := it.Filter
		p.WriteString(f.Kind.String())
		if f.Arg != nil {
			p.ws(f.Space0, " ")
			p.node(f.Arg)
		}
		if f.Arg2 != nil {
			p.ws(f.Space1, " ")
			p.node(f.Arg2)
		}
	}
}

func (p *printer) template(t *Template) {
	if t == nil {
		return
	}
	if t.Delimiter != 0 {
		p.WriteRune(t.Delimiter)
	}
	for _, e := range t.Elements {
		switch e := e.(type) {
		case *TemplateString:
			p.WriteString(e.Source)
		case *Placeholder:
			p.placeholder(e)
		}
	}
	if t.Delimiter != 0 {
		p.WriteRune(t.Delimiter)
	}
}

func (p *printer) placeholder(ph *Placeholder) {
	p.WriteString("{{")
	p.WriteString(ph.Space0.Value)
	p.WriteString(ph.Expr.Name)
	p.WriteString(ph.Space1.Value)
	p.WriteString(ph.Trailing)
	p.WriteString("}}")
}

// node prints any polymorphic value.
func (p *printer) node(n Node) {
	switch n := n.(type) {
	case *Template:
		p.template(n)
	case *Placeholder:
		p.placeholder(n)
	case *Number:
		p.WriteString(n.Source)
	case *Boolean:
		p.WriteString(strconv.FormatBool(n.Value))
	case *Null:
		p.WriteString("null")
	case *Regex:
		p.WriteString(n.Source)
	case *Base64:
		p.WriteString("base64,")
		p.ws(n.Space0, "")
		p.WriteString(n.Source)
		p.ws(n.Space1, "")
		p.WriteByte(';')
	case *Hex:
		p.WriteString("hex,")
		p.ws(n.Space0, "")
		p.WriteString(n.Source)
		p.ws(n.Space1, "")
		p.WriteByte(';')
	case *FileRef:
		p.WriteString("file,")
		p.ws(n.Space0, "")
		p.template(n.Filename)
		p.ws(n.Space1, "")
		p.WriteByte(';')
	case *MultilineString:
		p.multiline(n)
	case *XML:
		p.WriteString(n.Value)
	case *JSONNumber:
		p.WriteString(n.Source)
	case *JSONList:
		p.WriteByte('[')
		p.WriteString(n.Space0)
		for i, el := range n.Elements {
			if i > 0 {
				p.WriteByte(',')
			}
			p.WriteString(el.Space0)
			p.node(el.Value)
			p.WriteString(el.Space1)
		}
		p.WriteByte(']')
	case *JSONObject:
		p.WriteByte('{')
		p.WriteString(n.Space0)
		for i, el := range n.Elements {
			if i > 0 {
				p.WriteByte(',')
			}
			p.WriteString(el.Space0)
			p.template(el.Name)
			p.WriteString(el.Space1)
			p.WriteByte(':')
			p.WriteString(el.Space2)
			p.node(el.Value)
			p.WriteString(el.Space3)
		}
		p.WriteByte('}')
	case *Duration:
		p.WriteString(n.Value.Source)
		p.WriteString(n.Unit)
	case *VariableDefinition:
		p.WriteString(n.Name)
		p.ws(n.Space0, "")
		p.WriteByte('=')
		p.ws(n.Space1, "")
		p.node(n.Value)
	case *Identifier:
		p.WriteString(n.Value)
	case *CookiePath:
		p.cookiePath(n)
	case *CertificateAttribute:
		p.WriteString(`"` + n.Name + `"`)
	case *StreamField:
		p.WriteString(`"` + n.Name + `"`)
	}
}

func (p *printer) cookiePath(c *CookiePath) {
	p.WriteByte('"')
	if c.Source != "" {
		p.WriteString(c.Source)
	} else {
		p.template(c.Name)
		if a := c.Attribute; a != nil {
			p.WriteString("[" + a.Space0.Value + a.Name + a.Space1.Value + "]")
		}
	}
	p.WriteByte('"')
}

func (p *printer) multiline(m *MultilineString) {
	p.WriteString("```")
	p.WriteString(m.Lang)
	if m.Comma {
		p.WriteByte(',')
	}
	if p.lint {
		p.WriteString(m.Space.Value)
	} else {
		p.ws(m.Space, "")
	}
	p.WriteString(m.Newline.Value)
	p.template(m.Value)
	if v := m.Variables; v != nil {
		p.WriteString("variables")
		p.WriteString(v.Space.Value)
		p.node(v.Value)
		p.WriteString(v.Whitespace.Value)
	}
	p.WriteString("```")
}
