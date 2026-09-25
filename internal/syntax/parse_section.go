// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

func requestSection(r *reader) (*Section, *Error) {
	return section(r, true)
}

func responseSection(r *reader) (*Section, *Error) {
	return section(r, false)
}

var requestSectionKinds = map[string]SectionKind{
	"Query": SectionQueryParams, "QueryStringParams": SectionQueryParams,
	"Form": SectionFormParams, "FormParams": SectionFormParams,
	"Multipart": SectionMultipart, "MultipartFormData": SectionMultipart,
	"BasicAuth": SectionBasicAuth, "Cookies": SectionCookies, "Options": SectionOptions,
}

var responseSectionKinds = map[string]SectionKind{
	"Captures": SectionCaptures, "Asserts": SectionAsserts,
}

func section(r *reader, request bool) (*Section, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	start := r.pos
	name, err := sectionName(r)
	if err != nil {
		return nil, err
	}
	s := &Section{LineTerminators: lts, Space0: space0, Name: name, Span: Span{start, r.pos}}
	if s.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	kinds, errKind := requestSectionKinds, ErrRequestSectionName
	if !request {
		kinds, errKind = responseSectionKinds, ErrResponseSectionName
	}
	kind, ok := kinds[name]
	if !ok {
		return nil, errAt(Pos{Offset: start.Offset + 1, Line: start.Line, Col: start.Col + 1}, false, errKind, name)
	}
	s.Kind = kind
	switch kind {
	case SectionQueryParams, SectionFormParams, SectionCookies:
		s.KeyValues, err = zeroOrMore(r, keyValue)
	case SectionBasicAuth:
		var kv *KeyValue
		var found bool
		if kv, found, err = optional(r, keyValue); found {
			s.KeyValues = []*KeyValue{kv}
		}
	case SectionMultipart:
		s.Multipart, err = zeroOrMore(r, multipartParam)
	case SectionOptions:
		s.Options, err = zeroOrMore(r, option)
	case SectionCaptures:
		s.Captures, err = zeroOrMore(r, capture)
	case SectionAsserts:
		s.Asserts, err = zeroOrMore(r, assert)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func sectionName(r *reader) (string, *Error) {
	pos := r.pos
	if err := tryLiteral(r, "["); err != nil {
		return "", err
	}
	name := r.readWhile(isAlphanumeric)
	if name == "" {
		return "", expecting(pos, true, "a valid section name")
	}
	if err := tryLiteral(r, "]"); err != nil {
		return "", err
	}
	return name, nil
}

func multipartParam(r *reader) (MultipartParam, *Error) {
	save := r.pos
	fp, err := filenameParam(r)
	if err == nil {
		return fp, nil
	}
	if !err.recoverable {
		return nil, err
	}
	r.pos = save
	kv, err := keyValue(r)
	if err != nil {
		return nil, err
	}
	return kv, nil
}

func filenameParam(r *reader) (*FilenameParam, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	key, err := recovering(keyString)(r)
	if err != nil {
		return nil, err
	}
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, ":"); err != nil {
		return nil, asRecoverable(err, true)
	}
	space2, _ := zeroOrMoreSpaces(r)
	value, err := filenameValue(r)
	if err != nil {
		return nil, err
	}
	lt0, err := lineTerminator(r)
	if err != nil {
		return nil, err
	}
	return &FilenameParam{LineTerminators: lts, Space0: space0, Key: key, Space1: space1,
		Space2: space2, Value: value, LineTerminator0: lt0}, nil
}

func filenameValue(r *reader) (*FilenameValue, *Error) {
	if err := tryLiteral(r, "file,"); err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	name, err := filename(r)
	if err != nil {
		return nil, err
	}
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, ";"); err != nil {
		return nil, err
	}
	v := &FilenameValue{Space0: space0, Filename: name, Space1: space1}
	save := r.pos
	if _, err := lineTerminator(r); err == nil {
		r.pos = save
		v.Space2 = emptyWhitespace(save)
		return v, nil
	}
	r.pos = save
	v.Space2, _ = zeroOrMoreSpaces(r)
	start := r.pos
	if v.ContentType, err = unquotedTemplate(r); err != nil {
		return nil, errAt(start, false, ErrFileContentType, "")
	}
	return v, nil
}

func capture(r *reader) (*Capture, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	c := &Capture{LineTerminators: lts}
	c.Space0, _ = zeroOrMoreSpaces(r)
	start := r.pos
	if c.Name, err = recovering(keyString)(r); err != nil {
		return nil, err
	}
	c.Space1, _ = zeroOrMoreSpaces(r)
	if err := literal(r, ":"); err != nil {
		return nil, asRecoverable(err, true)
	}
	c.Space2, _ = zeroOrMoreSpaces(r)
	if c.Query, err = query(r); err != nil {
		return nil, err
	}
	if c.Filters, err = filters(r); err != nil {
		return nil, err
	}
	space, redact, err := optional(r, func(r *reader) (Whitespace, *Error) {
		sp, _ := zeroOrMoreSpaces(r)
		if err := tryLiteral(r, "redact"); err != nil {
			return Whitespace{}, err
		}
		return sp, nil
	})
	if err != nil {
		return nil, err
	}
	if redact {
		c.Space3, c.Redact = space, true
	} else {
		c.Space3 = emptyWhitespace(r.pos)
	}
	c.Span = Span{start, r.pos}
	if c.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	return c, nil
}

func assert(r *reader) (*Assert, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	a := &Assert{LineTerminators: lts}
	a.Space0, _ = zeroOrMoreSpaces(r)
	start := r.pos
	if a.Query, err = query(r); err != nil {
		return nil, err
	}
	if a.Filters, err = filters(r); err != nil {
		return nil, err
	}
	if a.Space1, err = oneOrMoreSpaces(r); err != nil {
		return nil, err
	}
	if a.Predicate, err = predicate(r); err != nil {
		return nil, err
	}
	a.Span = Span{start, r.pos}
	if a.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	return a, nil
}
