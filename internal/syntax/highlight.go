// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strconv"
	"strings"
)

// TokenKind classifies a piece of source for syntax highlighting.
type TokenKind int

// Token kinds, in the reference formatter's highlighting categories.
const (
	TokenPlain   TokenKind = iota // whitespace, punctuation, version, status, variable names
	TokenMethod                   // request method
	TokenString                   // URLs, templates, strings, file names, bodies, option names
	TokenSection                  // `[Name]` section headers
	TokenQuery                    // query kinds: status, header, jsonpath, ...
	TokenKeyword                  // filter and predicate kinds, `not`
	TokenNumber                   // numbers, booleans, null, durations
	TokenComment                  // `# ...` comments
)

// Token is a run of source text with its highlighting kind.
type Token struct {
	Kind TokenKind
	Text string
}

// Tokens splits Print(f) into highlighting tokens, following the reference
// formatter's walk over the file. Concatenating every Token.Text gives
// Print(f) back.
func Tokens(f *File) []Token {
	var w tokenWalker
	if f.BOM {
		w.emit(TokenPlain, utf8BOM)
	}
	for _, e := range f.Entries {
		w.request(e.Request)
		if e.Response != nil {
			w.response(e.Response)
		}
	}
	w.lineTerminators(f.LineTerminators)
	return w.tokens
}

type tokenWalker struct {
	tokens []Token
}

// emit appends text, merging it into the previous token when both share a
// kind. Empty plain text is dropped; an empty coloured token is kept, as
// the reference formatter keeps it (an empty value still prints its colour
// codes).
func (w *tokenWalker) emit(kind TokenKind, text string) {
	if text == "" && kind == TokenPlain {
		return
	}
	if n := len(w.tokens); n > 0 && w.tokens[n-1].Kind == kind {
		w.tokens[n-1].Text += text
		return
	}
	w.tokens = append(w.tokens, Token{Kind: kind, Text: text})
}

func (w *tokenWalker) ws(s Whitespace) { w.emit(TokenPlain, s.Value) }

// source prints n with the lossless printer.
func source(n Node) string {
	var p printer
	p.node(n)
	return p.String()
}

func (w *tokenWalker) template(t *Template) {
	if t != nil {
		w.emit(TokenString, source(t))
	}
}

func (w *tokenWalker) request(r *Request) {
	w.lineTerminators(r.LineTerminators)
	w.ws(r.Space0)
	w.emit(TokenMethod, r.Method.Value)
	w.ws(r.Space1)
	w.template(r.URL)
	w.lineTerminator(r.LineTerminator0)
	w.keyValues(r.Headers)
	w.sections(r.Sections)
	w.body(r.Body)
}

func (w *tokenWalker) response(r *Response) {
	w.lineTerminators(r.LineTerminators)
	w.ws(r.Space0)
	w.emit(TokenPlain, r.Version.Value)
	w.ws(r.Space1)
	w.emit(TokenPlain, r.Status.Value)
	w.lineTerminator(r.LineTerminator0)
	w.keyValues(r.Headers)
	w.sections(r.Sections)
	w.body(r.Body)
}

func (w *tokenWalker) body(b *Body) {
	if b == nil {
		return
	}
	w.lineTerminators(b.LineTerminators)
	w.ws(b.Space0)
	switch v := b.Value.(type) {
	case *Base64, *Hex, *FileRef:
		w.value(v)
	default:
		// Any other body, a bare JSON `true` or `1` included, is one string.
		w.emit(TokenString, source(v))
	}
	w.lineTerminator(b.LineTerminator0)
}

func (w *tokenWalker) lineTerminators(lts []*LineTerminator) {
	for _, lt := range lts {
		w.lineTerminator(lt)
	}
}

func (w *tokenWalker) lineTerminator(lt *LineTerminator) {
	if lt == nil {
		return
	}
	w.ws(lt.Space0)
	if lt.Comment != nil {
		w.emit(TokenComment, "#"+lt.Comment.Value)
	}
	w.ws(lt.Newline)
}

func (w *tokenWalker) keyValues(kvs []*KeyValue) {
	for _, kv := range kvs {
		w.keyValue(kv)
	}
}

func (w *tokenWalker) keyValue(kv *KeyValue) {
	w.lineTerminators(kv.LineTerminators)
	w.ws(kv.Space0)
	w.template(kv.Key)
	w.ws(kv.Space1)
	w.emit(TokenPlain, ":")
	w.ws(kv.Space2)
	w.template(kv.Value)
	w.lineTerminator(kv.LineTerminator0)
}

func (w *tokenWalker) sections(sections []*Section) {
	for _, s := range sections {
		w.lineTerminators(s.LineTerminators)
		w.ws(s.Space0)
		w.emit(TokenSection, "["+s.Name+"]")
		w.lineTerminator(s.LineTerminator0)
		w.keyValues(s.KeyValues)
		for _, m := range s.Multipart {
			switch m := m.(type) {
			case *KeyValue:
				w.keyValue(m)
			case *FilenameParam:
				w.filenameParam(m)
			}
		}
		for _, o := range s.Options {
			w.option(o)
		}
		for _, c := range s.Captures {
			w.capture(c)
		}
		for _, a := range s.Asserts {
			w.assert(a)
		}
		for _, m := range s.Messages {
			w.messageStep(m)
		}
	}
}

func (w *tokenWalker) messageStep(s *MessageStep) {
	w.lineTerminators(s.LineTerminators)
	w.ws(s.Space0)
	w.emit(TokenKeyword, s.Kind.String())
	if s.Colon {
		w.ws(s.Space1)
		w.emit(TokenPlain, ":")
		w.ws(s.Space2)
		w.value(s.Value)
	}
	w.lineTerminator(s.LineTerminator0)
}

func (w *tokenWalker) filenameParam(fp *FilenameParam) {
	w.lineTerminators(fp.LineTerminators)
	w.ws(fp.Space0)
	w.template(fp.Key)
	w.ws(fp.Space1)
	w.emit(TokenPlain, ":")
	w.ws(fp.Space2)
	v := fp.Value
	w.emit(TokenPlain, "file,")
	w.ws(v.Space0)
	w.template(v.Filename)
	w.ws(v.Space1)
	w.emit(TokenPlain, ";")
	w.ws(v.Space2)
	w.template(v.ContentType)
	w.lineTerminator(fp.LineTerminator0)
}

func (w *tokenWalker) option(o *Option) {
	w.lineTerminators(o.LineTerminators)
	w.ws(o.Space0)
	w.emit(TokenString, o.Name)
	w.ws(o.Space1)
	w.emit(TokenPlain, ":")
	w.ws(o.Space2)
	w.value(o.Value)
	w.lineTerminator(o.LineTerminator0)
}

func (w *tokenWalker) capture(c *Capture) {
	w.lineTerminators(c.LineTerminators)
	w.ws(c.Space0)
	w.template(c.Name)
	w.ws(c.Space1)
	w.emit(TokenPlain, ":")
	w.ws(c.Space2)
	w.query(c.Query)
	w.filters(c.Filters)
	if c.Redact {
		w.ws(c.Space3)
		w.emit(TokenString, "redact")
	}
	w.lineTerminator(c.LineTerminator0)
}

func (w *tokenWalker) assert(a *Assert) {
	w.lineTerminators(a.LineTerminators)
	w.ws(a.Space0)
	w.query(a.Query)
	w.filters(a.Filters)
	w.ws(a.Space1)
	if a.Predicate.Not {
		w.emit(TokenKeyword, "not")
		w.ws(a.Predicate.Space0)
	}
	f := a.Predicate.Func
	w.emit(TokenKeyword, f.Kind.String())
	if f.Value != nil {
		w.ws(f.Space0)
		w.value(f.Value)
	}
	w.lineTerminator(a.LineTerminator0)
}

func (w *tokenWalker) query(q *Query) {
	w.emit(TokenQuery, q.Kind.String())
	if q.Arg != nil {
		w.ws(q.Space0)
		w.value(q.Arg)
	}
}

func (w *tokenWalker) filters(items []*FilterItem) {
	for _, it := range items {
		w.ws(it.Space)
		f := it.Filter
		w.emit(TokenKeyword, f.Kind.String())
		if f.Arg != nil {
			w.ws(f.Space0)
			w.value(f.Arg)
		}
		if f.Arg2 != nil {
			w.ws(f.Space1)
			w.value(f.Arg2)
		}
	}
}

// value emits a polymorphic value: numbers, booleans, null and durations
// are numeric; files, base64 and hex keep their punctuation plain; every
// other value (strings, placeholders, regexes, bodies) is a string.
func (w *tokenWalker) value(n Node) {
	switch n := n.(type) {
	case nil:
	case *Number:
		w.emit(TokenNumber, n.Source)
	case *Boolean:
		w.emit(TokenNumber, strconv.FormatBool(n.Value))
	case *Null:
		w.emit(TokenNumber, "null")
	case *Duration:
		w.emit(TokenNumber, n.Value.Source+n.Unit)
	case *Base64:
		w.emit(TokenPlain, "base64,")
		w.ws(n.Space0)
		w.emit(TokenString, n.Source)
		w.ws(n.Space1)
		w.emit(TokenPlain, ";")
	case *Hex:
		w.emit(TokenPlain, "hex,")
		w.ws(n.Space0)
		w.emit(TokenString, n.Source)
		w.ws(n.Space1)
		w.emit(TokenPlain, ";")
	case *FileRef:
		w.emit(TokenPlain, "file,")
		w.ws(n.Space0)
		w.template(n.Filename)
		w.ws(n.Space1)
		w.emit(TokenPlain, ";")
	case *VariableDefinition:
		w.emit(TokenPlain, n.Name)
		w.ws(n.Space0)
		w.emit(TokenPlain, "=")
		w.ws(n.Space1)
		w.value(n.Value)
	default:
		w.emit(TokenString, source(n))
	}
}

// ANSI escape codes per token kind, as the reference formatter colours
// them; TokenPlain is not coloured.
var ansiColors = map[TokenKind]string{
	TokenMethod:  "\x1b[33m",
	TokenString:  "\x1b[32m",
	TokenSection: "\x1b[35m",
	TokenQuery:   "\x1b[36m",
	TokenKeyword: "\x1b[33m",
	TokenNumber:  "\x1b[36m",
	TokenComment: "\x1b[90m",
}

// HighlightANSI renders f with ANSI colour codes; without them the output
// is Print(f).
func HighlightANSI(f *File) []byte {
	var b strings.Builder
	var prev string
	for _, t := range Tokens(f) {
		color := ansiColors[t.Kind]
		// Kinds that share a colour still form one coloured run.
		if color != "" && color == prev {
			b.WriteString(t.Text)
			continue
		}
		if prev != "" {
			b.WriteString("\x1b[0m")
		}
		b.WriteString(color)
		b.WriteString(t.Text)
		prev = color
	}
	if prev != "" {
		b.WriteString("\x1b[0m")
	}
	return []byte(b.String())
}
