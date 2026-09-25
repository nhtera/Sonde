// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// bodyBytes parses a body value.
func bodyBytes(r *reader) (Bytes, *Error) {
	return prefixChoice(r, []alt[Bytes]{
		{[]string{"```"}, func(r *reader) (Bytes, *Error) { return asBytes(multilineString(r)) }},
		{[]string{"`"}, func(r *reader) (Bytes, *Error) { return asBytes(backtickTemplate(r)) }},
		{nil, func(r *reader) (Bytes, *Error) {
			v, err := jsonValue(r)
			if err != nil {
				return nil, err
			}
			return v.(Bytes), nil
		}},
		{[]string{"<"}, func(r *reader) (Bytes, *Error) { return asBytes(xmlBytes(r)) }},
		{[]string{"base64"}, func(r *reader) (Bytes, *Error) { return asBytes(base64Bytes(r)) }},
		{[]string{"hex"}, func(r *reader) (Bytes, *Error) { return asBytes(hexBytes(r)) }},
		{[]string{"file"}, func(r *reader) (Bytes, *Error) { return asBytes(fileRef(r)) }},
	})
}

func asBytes[T Bytes](v T, err *Error) (Bytes, *Error) {
	if err != nil {
		return nil, err
	}
	return v, nil
}

// advanceTo consumes input up to the file offset off.
func (r *reader) advanceTo(off int) {
	for r.pos.Offset < off && !r.isEOF() {
		r.read()
	}
}

// xmlBytes reads one well-formed XML document (prolog, then a root element)
// and stops right after the root element closes.
func xmlBytes(r *reader) (*XML, *Error) {
	if !r.peekIs('<') {
		return nil, errAt(r.pos, true, ErrXML, "")
	}
	start := r.pos
	d := xml.NewDecoder(strings.NewReader(normalizeXMLVersion(r.src[start.Offset:r.end])))
	// Only well-formedness is checked: bytes pass through whatever the
	// declared encoding, and the body is kept verbatim.
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	depth := 0
	for {
		tok, err := d.Token()
		if err != nil {
			// Report the last character read before the failure.
			off := start.Offset + int(d.InputOffset())
			if off > start.Offset {
				_, size := utf8.DecodeLastRuneInString(r.src[start.Offset:off])
				off -= size
			}
			bad := *r
			bad.advanceTo(off)
			return nil, errAt(bad.pos, false, ErrXML, "")
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				r.advanceTo(start.Offset + int(d.InputOffset()))
				return &XML{Value: r.slice(start)}, nil
			}
		}
	}
}

var xmlVersionAttr = regexp.MustCompile(`version\s*=\s*["']([0-9]\.[0-9])["']`)

// normalizeXMLVersion rewrites a declared XML version such as 1.1 to 1.0
// (same length, so offsets are unchanged): the decoder only accepts 1.0 and
// the syntax check does not depend on the version.
func normalizeXMLVersion(s string) string {
	if !strings.HasPrefix(s, "<?xml") {
		return s
	}
	end := strings.Index(s, "?>")
	if end < 0 {
		return s
	}
	m := xmlVersionAttr.FindStringSubmatchIndex(s[:end])
	if m == nil || s[m[2]:m[3]] == "1.0" {
		return s
	}
	return s[:m[2]] + "1.0" + s[m[3]:]
}

// multilineString parses ```lang? ... ```.
func multilineString(r *reader) (*MultilineString, *Error) {
	if err := tryLiteral(r, "```"); err != nil {
		return nil, err
	}
	return choice(r,
		func(r *reader) (*MultilineString, *Error) { return multilineText(r, MultilineJSON, "json") },
		func(r *reader) (*MultilineString, *Error) { return multilineText(r, MultilineXML, "xml") },
		multilineGraphQL,
		func(r *reader) (*MultilineString, *Error) { return multilineText(r, MultilineRaw, "raw") },
		func(r *reader) (*MultilineString, *Error) { return multilineText(r, MultilineText, "") },
	)
}

func multilineText(r *reader, kind MultilineKind, lang string) (*MultilineString, *Error) {
	if lang != "" {
		if err := tryLiteral(r, lang); err != nil {
			return nil, err
		}
	}
	space, _ := zeroOrMoreSpaces(r)
	start := r.pos
	if hint := r.readWhile(func(c rune) bool { return c != '\n' && c != '\r' && c != '`' }); hint != "" {
		return nil, errAt(start, false, ErrMultilineLanguageHint, hint)
	}
	nl, err := newline(r)
	if err != nil {
		return nil, err
	}
	value, err := multilineValue(r, kind != MultilineRaw)
	if err != nil {
		return nil, err
	}
	return &MultilineString{Kind: kind, Lang: lang, Space: space, Newline: nl, Value: value}, nil
}

func multilineValue(r *reader, templated bool) (*Template, *Error) {
	start := r.pos
	chars := r.scratch[:0]
	for !r.peekPrefix("```") && !r.isEOF() {
		pos := r.pos
		c, _ := r.read()
		if templated {
			chars = append(chars, encodedChar{c, r.slice(pos), pos})
		}
	}
	r.scratch = chars
	end := r.pos
	if err := literal(r, "```"); err != nil {
		return nil, err
	}
	return multilineTemplate(r, chars, start, end, templated)
}

func multilineTemplate(r *reader, chars []encodedChar, start, end Pos, templated bool) (*Template, *Error) {
	t := &Template{Span: Span{start, end}}
	if !templated {
		if raw := r.src[start.Offset:end.Offset]; raw != "" {
			t.Elements = []TemplateElement{&TemplateString{Value: raw, Source: raw}}
		}
		return t, nil
	}
	elements, err := templatize(r, chars, end)
	if err != nil {
		return nil, err
	}
	t.Elements = elements
	return t, nil
}

func multilineGraphQL(r *reader) (*MultilineString, *Error) {
	if err := tryLiteral(r, "graphql"); err != nil {
		return nil, err
	}
	comma := r.consume(",")
	space, _ := zeroOrMoreSpaces(r)
	nl, err := newline(r)
	if err != nil {
		return nil, err
	}
	ms := &MultilineString{Kind: MultilineGraphQL, Lang: "graphql", Comma: comma, Space: space, Newline: nl}
	start := r.pos
	var chars []encodedChar
	for !r.peekPrefix("```") && !r.isEOF() {
		pos := r.pos
		c, _ := r.read()
		chars = append(chars, encodedChar{c, r.slice(pos), pos})
		if c != '\n' {
			continue
		}
		end := r.pos
		vars, ok, err := optional(r, graphQLVariables)
		if err != nil {
			return nil, err
		}
		if ok {
			if err := literal(r, "```"); err != nil {
				return nil, err
			}
			if ms.Value, err = multilineTemplate(r, chars, start, end, true); err != nil {
				return nil, err
			}
			ms.Variables = vars
			return ms, nil
		}
	}
	end := r.pos
	if err := literal(r, "```"); err != nil {
		return nil, err
	}
	if ms.Value, err = multilineTemplate(r, chars, start, end, true); err != nil {
		return nil, err
	}
	return ms, nil
}

func graphQLVariables(r *reader) (*GraphQLVariables, *Error) {
	if err := tryLiteral(r, "variables"); err != nil {
		return nil, err
	}
	space, _ := zeroOrMoreSpaces(r)
	start := r.pos
	obj, err := jsonObject(r)
	if err != nil {
		return nil, errAt(start, false, ErrGraphQLVariables, "")
	}
	wsStart := r.pos
	ws := r.readWhile(isJSONWhitespace)
	return &GraphQLVariables{Space: space, Value: obj, Whitespace: Whitespace{Value: ws, Span: Span{wsStart, r.pos}}}, nil
}
