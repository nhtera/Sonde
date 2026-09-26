// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// maxJSONDepth bounds the nesting JSONBody rebuilds.
const maxJSONDepth = 512

var errNotRebuildable = errors.New("not rebuildable")

// JSONBody rebuilds text, one JSON value whose strings may hold {{name}}
// placeholders, as a syntax JSON body: members keep their order (and
// duplicates), numbers their spelling, and strings go through ParseText.
// ok is false when text is not exactly one JSON value, nests deeper than
// 512 levels, or has a placeholder in an object key; callers then keep the
// text as is with syntax.TextBody(ParseText(text), "json").
func JSONBody(text string) (body *syntax.BodySpec, warns []Warning, ok bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	v, err := jsonValue(dec, &warns, 0)
	if err != nil {
		return nil, nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, nil, false
	}
	b, err := syntax.JSONBody(v)
	if err != nil {
		return nil, nil, false
	}
	return b, warns, true
}

func jsonValue(dec *json.Decoder, warns *[]Warning, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, errNotRebuildable
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '[' {
			elems := []any{}
			for dec.More() {
				e, err := jsonValue(dec, warns, depth+1)
				if err != nil {
					return nil, err
				}
				elems = append(elems, e)
			}
			_, err := dec.Token()
			return elems, err
		}
		members := syntax.Members{}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := k.(string)
			if strings.Contains(key, "{{") {
				return nil, errNotRebuildable
			}
			v, err := jsonValue(dec, warns, depth+1)
			if err != nil {
				return nil, err
			}
			members = append(members, syntax.Member{Key: key, Value: v})
		}
		_, err := dec.Token()
		return members, err
	case string:
		text, w := ParseText(t)
		*warns = append(*warns, w...)
		return text, nil
	default: // nil, bool, json.Number
		return t, nil
	}
}
