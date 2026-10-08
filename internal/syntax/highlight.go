// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strconv"
	"strings"
)

// TokenKind classifies a piece of source for syntax highlighting, in the
// reference formatter's categories. Renderers map kinds to colours (ANSI)
// or classes (HTML).
type TokenKind int

// Token kinds.
const (
	TokenWhitespace   TokenKind = iota // spaces and line endings
	TokenLiteral                       // punctuation: `:`, `=`, `file,`, `;`, ...
	TokenMethod                        // request method
	TokenURL                           // request URL
	TokenString                        // templates, option names, `redact`, cookie paths
	TokenFilename                      // file names
	TokenSection                       // `[Name]` section headers
	TokenQuery                         // query kinds: status, header, jsonpath, ...
	TokenFilter                        // filter kinds
	TokenPredicate                     // predicate kinds
	TokenNot                           // `not`
	TokenNumber                        // numbers and counts
	TokenBoolean                       // true, false
	TokenNull                          // null
	TokenComment                       // `# ...`
	TokenVersion                       // HTTP version of a response
	TokenStatus                        // status of a response
	TokenJSON                          // JSON body
	TokenXML                           // XML body
	TokenMultiline                     // ``` multiline strings
	TokenBase64                        // base64 value
	TokenHex                           // hex value
	TokenRegex                         // /regex/ literal
	TokenPlaceholder                   // a {{ }} placeholder standing alone as a value
	TokenUnit                          // duration unit
	TokenVariableName                  // name of a `variable` option
	// TokenOpen and TokenClose bracket an entry, request or response
	// (Token.Text is "entry", "request" or "response"); they carry no
	// source text.
	TokenOpen
	TokenClose
)

// Token is a run of source text with its highlighting kind.
type Token struct {
	Kind TokenKind
	Text string
}

// Tokens splits Print(f) into highlighting tokens, following the reference
// formatter's walk over the file. Concatenating the Text of every token but
// TokenOpen and TokenClose gives Print(f) back.
func Tokens(f *File) []Token {
	var w tokenWalker
	if f.BOM {
		w.emit(TokenWhitespace, utf8BOM)
	}
	for _, e := range f.Entries {
		w.mark(TokenOpen, "entry")
		w.request(e.Request)
		if e.Response != nil {
			w.response(e.Response)
		}
		w.mark(TokenClose, "entry")
	}
	w.lineTerminators(f.LineTerminators)
	return w.tokens
}

type tokenWalker struct {
	tokens []Token
}

// emit appends a token. Empty whitespace and punctuation are dropped; an
// empty value is kept, since the reference formatter still renders it (an
// empty coloured run, an empty HTML span).
func (w *tokenWalker) emit(kind TokenKind, text string) {
	if text == "" && (kind == TokenWhitespace || kind == TokenLiteral) {
		return
	}
	w.tokens = append(w.tokens, Token{Kind: kind, Text: text})
}

func (w *tokenWalker) mark(kind TokenKind, name string) {
	w.tokens = append(w.tokens, Token{Kind: kind, Text: name})
}

func (w *tokenWalker) ws(s Whitespace) { w.emit(TokenWhitespace, s.Value) }

// source prints n with the lossless printer.
func source(n Node) string {
	var p printer
	p.node(n)
	return p.String()
}

func (w *tokenWalker) template(kind TokenKind, t *Template) {
	if t != nil {
		w.emit(kind, source(t))
	}
}

func (w *tokenWalker) request(r *Request) {
	w.mark(TokenOpen, "request")
	w.lineTerminators(r.LineTerminators)
	w.ws(r.Space0)
	w.emit(TokenMethod, r.Method.Value)
	w.ws(r.Space1)
	w.template(TokenURL, r.URL)
	w.lineTerminator(r.LineTerminator0)
	w.keyValues(r.Headers)
	w.sections(r.Sections)
	w.body(r.Body)
	w.mark(TokenClose, "request")
}

func (w *tokenWalker) response(r *Response) {
	w.mark(TokenOpen, "response")
	w.lineTerminators(r.LineTerminators)
	w.ws(r.Space0)
	w.emit(TokenVersion, r.Version.Value)
	w.ws(r.Space1)
	w.emit(TokenStatus, r.Status.Value)
	w.lineTerminator(r.LineTerminator0)
	w.keyValues(r.Headers)
	w.sections(r.Sections)
	w.body(r.Body)
	w.mark(TokenClose, "response")
}

func (w *tokenWalker) body(b *Body) {
	if b == nil {
		return
	}
	w.lineTerminators(b.LineTerminators)
	w.ws(b.Space0)
	switch v := b.Value.(type) {
	case *Base64, *Hex, *FileRef, *MultilineString:
		w.value(v)
	case *XML:
		w.emit(TokenXML, v.Value)
	case *Template:
		if v.Delimiter == '"' { // a JSON string body
			w.template(TokenJSON, v)
		} else { // a `one-line` string
			w.template(TokenString, v)
		}
	default:
		// Any JSON body, a bare `true` or `1` included.
		w.emit(TokenJSON, source(v))
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
	w.template(TokenString, kv.Key)
	w.ws(kv.Space1)
	w.emit(TokenLiteral, ":")
	w.ws(kv.Space2)
	w.template(TokenString, kv.Value)
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
	w.emit(TokenQuery, s.Kind.String())
	if s.Colon {
		w.ws(s.Space1)
		w.emit(TokenLiteral, ":")
		w.ws(s.Space2)
		w.value(s.Value)
	}
	w.lineTerminator(s.LineTerminator0)
}

func (w *tokenWalker) filenameParam(fp *FilenameParam) {
	w.lineTerminators(fp.LineTerminators)
	w.ws(fp.Space0)
	w.template(TokenString, fp.Key)
	w.ws(fp.Space1)
	w.emit(TokenLiteral, ":")
	w.ws(fp.Space2)
	v := fp.Value
	w.emit(TokenLiteral, "file,")
	w.ws(v.Space0)
	w.template(TokenFilename, v.Filename)
	w.ws(v.Space1)
	w.emit(TokenLiteral, ";")
	w.ws(v.Space2)
	w.template(TokenString, v.ContentType)
	w.lineTerminator(fp.LineTerminator0)
}

// filenameOptions are the options whose value is a file name.
var filenameOptions = map[string]bool{
	"cacert": true, "cert": true, "key": true, "netrc-file": true, "output": true, "unix-socket": true,
}

func (w *tokenWalker) option(o *Option) {
	w.lineTerminators(o.LineTerminators)
	w.ws(o.Space0)
	w.emit(TokenString, o.Name)
	w.ws(o.Space1)
	w.emit(TokenLiteral, ":")
	w.ws(o.Space2)
	if t, ok := o.Value.(*Template); ok && filenameOptions[o.Name] {
		w.template(TokenFilename, t)
	} else {
		w.value(o.Value)
	}
	w.lineTerminator(o.LineTerminator0)
}

func (w *tokenWalker) capture(c *Capture) {
	w.lineTerminators(c.LineTerminators)
	w.ws(c.Space0)
	w.template(TokenString, c.Name)
	w.ws(c.Space1)
	w.emit(TokenLiteral, ":")
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
		w.emit(TokenNot, "not")
		w.ws(a.Predicate.Space0)
	}
	f := a.Predicate.Func
	w.emit(TokenPredicate, f.Kind.String())
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
		w.emit(TokenFilter, f.Kind.String())
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

// value emits a polymorphic value.
func (w *tokenWalker) value(n Node) {
	switch n := n.(type) {
	case nil:
	case *Template:
		w.template(TokenString, n)
	case *Number:
		w.emit(TokenNumber, n.Source)
	case *Boolean:
		w.emit(TokenBoolean, strconv.FormatBool(n.Value))
	case *Null:
		w.emit(TokenNull, "null")
	case *Duration:
		w.emit(TokenNumber, n.Value.Source)
		if n.Unit != "" {
			w.emit(TokenUnit, n.Unit)
		}
	case *Placeholder:
		w.emit(TokenPlaceholder, source(n))
	case *Regex:
		w.emit(TokenRegex, n.Source)
	case *MultilineString:
		w.emit(TokenMultiline, source(n))
	case *Base64:
		w.emit(TokenLiteral, "base64,")
		w.ws(n.Space0)
		w.emit(TokenBase64, n.Source)
		w.ws(n.Space1)
		w.emit(TokenLiteral, ";")
	case *Hex:
		w.emit(TokenLiteral, "hex,")
		w.ws(n.Space0)
		w.emit(TokenHex, n.Source)
		w.ws(n.Space1)
		w.emit(TokenLiteral, ";")
	case *FileRef:
		w.emit(TokenLiteral, "file,")
		w.ws(n.Space0)
		w.template(TokenFilename, n.Filename)
		w.ws(n.Space1)
		w.emit(TokenLiteral, ";")
	case *VariableDefinition:
		w.emit(TokenVariableName, n.Name)
		w.ws(n.Space0)
		w.emit(TokenLiteral, "=")
		w.ws(n.Space1)
		w.value(n.Value)
	default:
		// Cookie paths, certificate attributes, stream fields, verbosity.
		w.emit(TokenString, source(n))
	}
}

// ANSI escape codes per token kind, as the reference formatter colours
// them; kinds not listed are not coloured.
var ansiColors = map[TokenKind]string{
	TokenMethod:      "\x1b[33m",
	TokenURL:         "\x1b[32m",
	TokenString:      "\x1b[32m",
	TokenFilename:    "\x1b[32m",
	TokenSection:     "\x1b[35m",
	TokenQuery:       "\x1b[36m",
	TokenFilter:      "\x1b[33m",
	TokenPredicate:   "\x1b[33m",
	TokenNot:         "\x1b[33m",
	TokenNumber:      "\x1b[36m",
	TokenBoolean:     "\x1b[36m",
	TokenNull:        "\x1b[36m",
	TokenComment:     "\x1b[90m",
	TokenJSON:        "\x1b[32m",
	TokenXML:         "\x1b[32m",
	TokenMultiline:   "\x1b[32m",
	TokenBase64:      "\x1b[32m",
	TokenHex:         "\x1b[32m",
	TokenRegex:       "\x1b[32m",
	TokenPlaceholder: "\x1b[32m",
	TokenUnit:        "\x1b[36m",
}

// HighlightANSI renders f with ANSI colour codes; without them the output
// is Print(f). Neighbouring tokens of one colour form a single coloured
// run, as in the reference formatter.
func HighlightANSI(f *File) []byte {
	var b strings.Builder
	var prev string
	for _, t := range Tokens(f) {
		if t.Kind == TokenOpen || t.Kind == TokenClose {
			continue
		}
		color := ansiColors[t.Kind]
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
