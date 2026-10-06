// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxedit

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/syntax"
)

// URLParam is a param of a URL's own query string (`?name=value&…`), as
// the [Query] row MoveURLQuery writes for it: source text, the value
// percent-decoded. A param that cannot move is listed as written in the
// URL.
type URLParam struct{ Key, Value string }

// urlAtom is one decoded rune of a URL, or a {{placeholder}}.
type urlAtom struct {
	r     rune
	name  string
	isVar bool
}

func (a urlAtom) is(r rune) bool { return !a.isVar && a.r == r }

// urlQuery is the query string of a URL, split into params.
type urlQuery struct {
	// base is the URL without its query string.
	base   syntax.Text
	params []URLParam
	// fields are the [Query] rows the params become; nil when kept.
	fields []syntax.Field
	// kept says why the query stays in the URL ("": it can move).
	kept string
}

// urlQueryOf splits the query string off url; nil when it has none (no
// `?`, or nothing after it).
func urlQueryOf(url *syntax.Template) *urlQuery {
	atoms := atomsOf(url)
	at := -1
	for i, a := range atoms {
		if a.is('?') {
			at = i
			break
		}
	}
	if at < 0 || at == len(atoms)-1 {
		return nil
	}
	q := &urlQuery{base: atomText(atoms[:at])}
	query := atoms[at+1:]
	for _, a := range atoms {
		if a.is('#') {
			// The engine appends [Query] to the URL as written: after a
			// fragment it would not be a query any more.
			q.kept = "the URL has a #fragment"
		}
	}
	for _, piece := range splitAtoms(query, '&') {
		if len(piece) == 0 {
			continue // `a=1&&b=2`, a trailing `&`: no param
		}
		eq := -1
		for i, a := range piece {
			if a.is('=') {
				eq = i
				break
			}
		}
		if eq <= 0 {
			// `flag` or `=v`: a row would send `flag=`, not `flag`.
			q.params = append(q.params, URLParam{Key: syntax.URLSource(atomText(piece))})
			if q.kept == "" {
				q.kept = fmt.Sprintf("%q is not name=value", syntax.URLSource(atomText(piece)))
			}
			continue
		}
		// Names are sent as written, values percent-encoded: the name
		// stays as is, the value is decoded.
		name, raw := atomText(piece[:eq]), piece[eq+1:]
		value, err := decodeValue(raw)
		if err != nil {
			q.params = append(q.params, URLParam{Key: syntax.URLSource(name), Value: syntax.URLSource(atomText(raw))})
			if q.kept == "" {
				q.kept = err.Error()
			}
			continue
		}
		f := syntax.Field{Key: name, Value: value}
		k, v := syntax.FieldSource(f)
		q.params = append(q.params, URLParam{Key: k, Value: v})
		q.fields = append(q.fields, f)
	}
	if q.kept != "" {
		q.fields = nil
	}
	return q
}

// atomsOf is url as decoded runes and placeholders.
func atomsOf(url *syntax.Template) []urlAtom {
	var atoms []urlAtom
	for _, el := range url.Elements {
		switch el := el.(type) {
		case *syntax.TemplateString:
			for _, r := range el.Value {
				atoms = append(atoms, urlAtom{r: r})
			}
		case *syntax.Placeholder:
			atoms = append(atoms, urlAtom{name: el.Expr.Name, isVar: true})
		}
	}
	return atoms
}

// queryStart is the byte offset of the `?` that starts the query string
// of url, source text ({{placeholders}} and escapes skipped); -1: none.
func queryStart(url string) int {
	for i := 0; i < len(url); i++ {
		switch {
		case url[i] == '\\' && strings.HasPrefix(url[i+1:], "u{"):
			if j := strings.IndexByte(url[i:], '}'); j > 0 {
				i += j
			}
		case url[i] == '\\':
			i++
		case strings.HasPrefix(url[i:], "{{"):
			if j := strings.Index(url[i:], "}}"); j > 0 {
				i += j + 1
			}
		case url[i] == '?':
			return i
		}
	}
	return -1
}

// splitAtoms splits atoms at every sep rune.
func splitAtoms(atoms []urlAtom, sep rune) [][]urlAtom {
	var out [][]urlAtom
	start := 0
	for i, a := range atoms {
		if a.is(sep) {
			out = append(out, atoms[start:i])
			start = i + 1
		}
	}
	return append(out, atoms[start:])
}

// atomText is atoms as a Text.
func atomText(atoms []urlAtom) syntax.Text {
	var t syntax.Text
	var lit strings.Builder
	for _, a := range atoms {
		if !a.isVar {
			lit.WriteRune(a.r)
			continue
		}
		if lit.Len() > 0 {
			t = append(t, syntax.Lit(lit.String()))
			lit.Reset()
		}
		t = append(t, syntax.Var(a.name))
	}
	if lit.Len() > 0 {
		t = append(t, syntax.Lit(lit.String()))
	}
	return t
}

// decodeValue percent-decodes a query value (`+` is a space). A value
// that would not be sent the same way from [Query] is refused.
func decodeValue(atoms []urlAtom) (syntax.Text, error) {
	raw := syntax.URLSource(atomText(atoms))
	var lit []byte
	for i := 0; i < len(atoms); i++ {
		a := atoms[i]
		switch {
		case a.isVar:
			// In the URL a variable's value is sent as is; in [Query] it
			// would be percent-encoded (an `&` in it, an encoded value).
			return nil, fmt.Errorf("%q has a {{variable}}, sent as is in the URL", raw)
		case a.r == ';':
			// Some servers split params at `;`: in [Query] it is encoded.
			return nil, fmt.Errorf("%q has a ;", raw)
		case a.r == '+':
			lit = append(lit, ' ')
		case a.r == '%':
			var b []byte
			if i+2 < len(atoms) && !atoms[i+1].isVar && !atoms[i+2].isVar {
				b, _ = hex.DecodeString(string([]rune{atoms[i+1].r, atoms[i+2].r}))
			}
			if len(b) != 1 {
				return nil, fmt.Errorf("%q has a %% that is not an escape", raw)
			}
			lit = append(lit, b[0])
			i += 2
		default:
			lit = utf8.AppendRune(lit, a.r)
		}
	}
	if !utf8.Valid(lit) {
		return nil, fmt.Errorf("%q decodes to bytes that are not text", raw)
	}
	return syntax.PlainText(string(lit)), nil
}

// MoveURLQuery moves the query string of entry n's URL to [Query] rows,
// before the rows it has (the order they are sent in): the URL keeps its
// base, every `name=value` becomes a `name: value` row, the value
// percent-decoded (the engine encodes it again). It is refused when the
// URL has no query string or when a param would not be sent the same way:
// a #fragment, a piece that is not name=value, a value with a
// {{variable}} or a `;`, a bad % escape. The server reads the same params;
// an equivalent encoding may change (`+` is sent as %20, `,` as %2C), and
// empty pieces (`a=1&&b=2`, a trailing `&`) are dropped.
func MoveURLQuery(name string, src []byte, n int) (*Result, error) {
	d, en, err := load(name, src, n)
	if err != nil {
		return nil, err
	}
	q := urlQueryOf(en.e.Request.URL)
	if q == nil {
		return nil, fmt.Errorf("%w: the URL has no query string", ErrInvalid)
	}
	if q.kept != "" {
		return nil, fmt.Errorf("%w: the query stays in the URL: %s", ErrInvalid, q.kept)
	}
	var rows strings.Builder
	for _, f := range q.fields {
		k, v := syntax.FieldSource(f)
		rows.WriteString(rowText(Query, k, v) + "\n")
	}
	count := 0
	var at int
	var lead string
	if s := en.sections[Query]; s != nil && len(s.rows) > 0 {
		count, at = len(s.rows), s.rows[0].lineStart
	} else {
		at, lead = d.rowInsertion(en, Query)
	}
	// One edit from the URL's `?` to the rows: the URL's base and the
	// lines between are kept as written, only the rows are new.
	sp := en.e.Request.URL.Span
	cut := queryStart(d.text(sp))
	if cut < 0 {
		return nil, fmt.Errorf("%w: the URL has no query string", ErrInvalid)
	}
	start := sp.Start.Offset + cut
	text := string(src[sp.End.Offset:at]) + d.eol(lead+rows.String())
	out := make([]byte, 0, len(src)+len(text))
	out = append(append(append(out, src[:start]...), text...), src[at:]...)
	nd, err := parse(name, out)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := sameEntries(d)(nd); err != nil {
		return nil, err
	}
	ne := nd.entries[n-1]
	if got, want := syntax.URLSource(atomText(atomsOf(ne.e.Request.URL))), syntax.URLSource(q.base); got != want {
		return nil, fmt.Errorf("%w: the URL reads as %q, not %q", ErrInvalid, got, want)
	}
	s := ne.sections[Query]
	if s == nil || len(s.rows) != count+len(q.fields) {
		return nil, fmt.Errorf("%w: the query rows do not read back", ErrInvalid)
	}
	for i, f := range q.fields {
		k, v := syntax.FieldSource(f)
		if r := s.rows[i]; r.Key != k || r.Value != v || r.Disabled {
			return nil, fmt.Errorf("%w: the row %q reads as %q", ErrInvalid, rowText(Query, k, v), rowText(Query, r.Key, r.Value))
		}
	}
	return &Result{Source: out, Edits: []TextEdit{{Range: d.off.rng(start, at), NewText: text}}}, nil
}
