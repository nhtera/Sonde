// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import "slices"

// Sonde-only constructs (docs/decisions/0004-streaming-protocols.md). Each
// parses only in the .sonde dialect; in a .hurl file it is an ErrSondeOnly
// error naming the construct.

var sondeRequestSectionKinds = map[string]SectionKind{"SondeMessages": SectionMessages, "SondeGrpc": SectionGrpc}

var sondeOptionShapes = map[string]int{
	"sonde-stream-count": optNatural, "sonde-stream-max-bytes": optNatural,
	"sonde-stream-timeout": optDuration,
}

// sondeOnly is the error for a Sonde construct at pos in a .hurl file, or
// nil in a .sonde one.
func sondeOnly(r *reader, pos Pos, what string) *Error {
	if r.sonde {
		return nil
	}
	return errAt(pos, false, ErrSondeOnly, what)
}

var stepKinds = map[string]StepKind{"send": StepSend, "receive": StepReceive, "close": StepClose}

// messageStep parses one [SondeMessages] step. A line that does not start
// with a lowercase word followed by `:` or the end of the line is not a step
// (a body, the response line, the next entry), so it fails recoverably.
func messageStep(r *reader) (*MessageStep, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	s := &MessageStep{LineTerminators: lts}
	s.Space0, _ = zeroOrMoreSpaces(r)
	start := r.pos
	name := r.readWhile(func(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || isDigit(c) || c == '-' })
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return nil, errAt(start, true, ErrMessageStep, name)
	}
	afterName := r.pos
	space1, _ := zeroOrMoreSpaces(r)
	colon := r.consume(":")
	kind, ok := stepKinds[name]
	if !colon {
		r.pos = afterName
		end := atLineEnd(r)
		switch {
		case r.peekIs(','): // a `hex,…;`, `base64,…;` or `file,…;` body
			return nil, errAt(start, true, ErrMessageStep, name)
		case end && (name == "true" || name == "false" || name == "null"): // a JSON body
			return nil, errAt(start, true, ErrMessageStep, name)
		case !end && ok: // `receive 2`
			return nil, expecting(afterName, false, ":")
		}
	}
	if !ok {
		return nil, errAt(start, false, ErrMessageStep, name)
	}
	s.Kind = kind
	switch {
	case colon:
		s.Space1, s.Colon = space1, true
		s.Space2, _ = zeroOrMoreSpaces(r)
		if kind == StepSend {
			valuePos := r.pos
			if s.Value, err = asNode(bodyBytes(r)); err != nil && err.recoverable {
				return nil, errAt(valuePos, false, ErrMessageValue, "")
			}
		} else {
			s.Value, err = stepNumber(r, kind)
		}
		if err != nil {
			return nil, asRecoverable(err, false)
		}
	case kind == StepSend:
		return nil, expecting(r.pos, false, ":")
	}
	s.Span = Span{start, r.pos}
	if s.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	return s, nil
}

// stepNumber is the value of a receive step (a count, at least 1) or a
// close step (a status code a close frame may carry), or a placeholder.
func stepNumber(r *reader, kind StepKind) (Node, *Error) {
	what := "integer >= 1"
	valid := func(n int64) bool { return n >= 1 }
	if kind == StepClose {
		what = "close code 1000-4999 (not 1004-1006 or 1015)"
		valid = func(n int64) bool { return ValidCloseCode(int(n)) }
	}
	start := r.pos
	v, err := literalOrPlaceholder(r, what, func(r *reader) (Node, *Error) { return asNode(natural(r)) })
	if err != nil {
		return nil, err
	}
	if n, ok := v.(*Number); ok && (n.Kind != NumberInteger || !valid(n.Int)) {
		return nil, expecting(start, false, what)
	}
	return v, nil
}

// ValidCloseCode reports whether a WebSocket close frame may carry code
// (RFC 6455 §7.4): 1000-4999 except the codes reserved for local use.
func ValidCloseCode(code int) bool {
	return code >= 1000 && code <= 4999 && (code < 1004 || code > 1006) && code != 1015
}

// atLineEnd reports whether a line terminator follows, without consuming it.
func atLineEnd(r *reader) bool {
	save := r.pos
	_, err := lineTerminator(r)
	r.pos = save
	return err == nil
}

// sondeStreamQuery is `sondeStream` with an optional quoted field.
func sondeStreamQuery(r *reader) (*Query, *Error) {
	return sondeFieldQuery(r, QuerySondeStream, validStreamFields, ErrStreamField)
}

// sondeGrpcQuery is `sondeGrpc` with an optional quoted field.
func sondeGrpcQuery(r *reader) (*Query, *Error) {
	return sondeFieldQuery(r, QuerySondeGrpc, validGrpcFields, ErrGrpcField)
}

// sondeFieldQuery is a Sonde query keyword with an optional quoted field,
// one of fields.
func sondeFieldQuery(r *reader, kind QueryKind, fields []string, errKind ErrorKind) (*Query, *Error) {
	start := r.pos
	if err := tryLiteral(r, kind.String()); err != nil {
		return nil, err
	}
	if err := sondeOnly(r, start, "query `"+kind.String()+"`"); err != nil {
		return nil, err
	}
	q := &Query{Kind: kind}
	save := r.pos
	space0, _ := zeroOrMoreSpaces(r)
	if space0.Value == "" || !r.peekIs('"') {
		r.pos = save
		return q, nil
	}
	r.read() // '"'
	from := r.pos
	name := r.readWhile(isLetter)
	if !slices.Contains(fields, name) {
		e := errAt(from, false, errKind, name)
		e.sonde = true
		return nil, e
	}
	if err := literal(r, `"`); err != nil {
		return nil, err
	}
	q.Space0, q.Arg = space0, &StreamField{Name: name}
	return q, nil
}

// grpcKeyValue is a `key: value` line of [SondeGrpc]; the key is one of
// validGrpcKeys, written without templates.
func grpcKeyValue(r *reader) (*KeyValue, *Error) {
	kv, err := keyValue(r)
	if err != nil {
		return nil, err
	}
	key := ""
	for _, el := range kv.Key.Elements {
		ts, ok := el.(*TemplateString)
		if !ok {
			key = "{{"
			break
		}
		key += ts.Value
	}
	if !slices.Contains(validGrpcKeys, key) {
		e := errAt(kv.Key.Span.Start, false, ErrGrpcKey, key)
		e.sonde = true
		return nil, e
	}
	return kv, nil
}
