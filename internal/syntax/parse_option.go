// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import "fmt"

// Option value shapes.
const (
	optString = iota
	optFilename
	optFilenamePassword
	optBoolean
	optNatural
	optCount
	optDuration
	optVariable
	optVerbosity
)

var optionShapes = map[string]int{
	"aws-sigv4": optString, "cacert": optFilename, "cert": optFilenamePassword,
	"compressed": optBoolean, "connect-to": optString, "connect-timeout": optDuration,
	"delay": optDuration, "digest": optBoolean, "insecure": optBoolean, "header": optString,
	"http1.0": optBoolean, "http1.1": optBoolean, "http2": optBoolean, "http3": optBoolean,
	"ipv4": optBoolean, "ipv6": optBoolean, "key": optFilename, "limit-rate": optNatural,
	"location": optBoolean, "location-trusted": optBoolean, "max-redirs": optCount,
	"max-time": optDuration, "negotiate": optBoolean, "netrc": optBoolean,
	"netrc-file": optString, "netrc-optional": optBoolean, "ntlm": optBoolean,
	"output": optFilename, "path-as-is": optBoolean, "pinnedpubkey": optString,
	"proxy": optString, "repeat": optCount, "resolve": optString, "retry": optCount,
	"retry-interval": optDuration, "skip": optBoolean, "unix-socket": optString,
	"user": optString, "variable": optVariable, "verbose": optBoolean,
	"verbosity": optVerbosity, "very-verbose": optBoolean,
}

func isOptionNameChar(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || isDigit(c) || c == '-' || c == '.' || c == '_'
}

func option(r *reader) (*Option, *Error) {
	lts, err := optionalLineTerminators(r)
	if err != nil {
		return nil, err
	}
	o := &Option{LineTerminators: lts}
	o.Space0, _ = zeroOrMoreSpaces(r)
	start := r.pos
	o.Name = r.readWhile(isOptionNameChar)
	o.Space1, _ = zeroOrMoreSpaces(r)
	if err := tryLiteral(r, ":"); err != nil {
		return nil, err
	}
	o.Space2, _ = zeroOrMoreSpaces(r)
	shape, ok := optionShapes[o.Name]
	if !ok {
		return nil, errAt(start, false, ErrInvalidOption, o.Name)
	}
	if o.Value, err = optionValue(r, shape); err != nil {
		return nil, err
	}
	if o.LineTerminator0, err = lineTerminator(r); err != nil {
		return nil, err
	}
	return o, nil
}

func optionValue(r *reader, shape int) (Node, *Error) {
	switch shape {
	case optString:
		return asNode(unquotedTemplate(r))
	case optFilename:
		return asNode(filename(r))
	case optFilenamePassword:
		return asNode(filenamePassword(r))
	case optBoolean:
		return literalOrPlaceholder(r, "true|false", func(r *reader) (Node, *Error) { return asNode(boolean(r)) })
	case optNatural:
		return literalOrPlaceholder(r, "integer >= 0", func(r *reader) (Node, *Error) { return asNode(natural(r)) })
	case optCount:
		return literalOrPlaceholder(r, "integer >= -1", func(r *reader) (Node, *Error) { return asNode(count(r)) })
	case optDuration:
		start := r.pos
		d, err := duration(r)
		if err == nil {
			return d, nil
		}
		if !err.recoverable {
			return nil, err
		}
		r.pos = start
		ph, err := placeholder(r)
		if err != nil {
			return nil, expecting(err.Pos, false, "integer")
		}
		return ph, nil
	case optVariable:
		return asNode(variableDefinition(r))
	default: // optVerbosity
		start := r.pos
		switch v := r.readWhile(func(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }); v {
		case "brief", "verbose", "debug":
			return &Identifier{Value: v}, nil
		}
		r.pos = start
		return nil, expecting(start, false, "brief|verbose|debug")
	}
}

// literalOrPlaceholder tries lit, then a placeholder; any failure of the
// placeholder is reported as expecting what.
func literalOrPlaceholder(r *reader, what string, lit parseFunc[Node]) (Node, *Error) {
	start := r.pos
	if v, err := lit(r); err == nil {
		return v, nil
	}
	r.pos = start
	ph, err := placeholder(r)
	if err != nil {
		return nil, expecting(err.Pos, false, what)
	}
	return ph, nil
}

// count is an integer >= -1 (-1 means infinite).
func count(r *reader) (*Number, *Error) {
	start := r.pos
	n, err := committing(integer)(r)
	if err != nil {
		return nil, err
	}
	if n.Int < -1 {
		return nil, expecting(start, false, "Expecting a count value")
	}
	return n, nil
}

var reservedNames = map[string]bool{"getEnv": true, "newDate": true, "newUuid": true}

func variableDefinition(r *reader) (*VariableDefinition, *Error) {
	start := r.pos
	name := r.readWhile(isNameChar)
	if name == "" {
		return nil, expecting(start, false, "variable name")
	}
	if reservedNames[name] {
		return nil, errAt(start, false, ErrVariable,
			fmt.Sprintf("conflicts with the %s function, use a different name", name))
	}
	space0, _ := zeroOrMoreSpaces(r)
	if err := literal(r, "="); err != nil {
		return nil, err
	}
	space1, _ := zeroOrMoreSpaces(r)
	value, err := choice(r,
		func(r *reader) (Node, *Error) { return asNode(null(r)) },
		func(r *reader) (Node, *Error) { return asNode(boolean(r)) },
		func(r *reader) (Node, *Error) { return asNode(number(r)) },
		func(r *reader) (Node, *Error) { return asNode(quotedTemplate(r)) },
		func(r *reader) (Node, *Error) { return asNode(unquotedTemplate(r)) },
	)
	if err != nil {
		return nil, expecting(err.Pos, false, "variable value")
	}
	return &VariableDefinition{Span: Span{start, r.pos}, Name: name, Space0: space0,
		Space1: space1, Value: value}, nil
}
