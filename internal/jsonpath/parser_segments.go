// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import "strings"

// parseSegments parses zero or more child/descendant segments.
func parseSegments(r *reader) ([]segment, error) {
	var segments []segment
	for {
		seg, ok, err := trySegment(r)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

func trySegment(r *reader) (segment, bool, error) {
	save := r.cursor()
	skipWhitespace(r)

	if seg, ok, err := tryDescendantSegment(r); err != nil {
		return segment{}, false, err
	} else if ok {
		return seg, true, nil
	}
	if seg, ok, err := tryChildSegment(r); err != nil {
		return segment{}, false, err
	} else if ok {
		return seg, true, nil
	}

	r.seek(save)
	return segment{}, false, nil
}

func tryChildSegment(r *reader) (segment, bool, error) {
	if selectors, ok, err := tryBracketedSelection(r); err != nil {
		return segment{}, false, err
	} else if ok {
		return segment{kind: segChild, selectors: selectors}, true, nil
	}
	if !matchStr(".", r) {
		return segment{}, false, nil
	}
	save := r.cursor()
	if matchStr("*", r) {
		return segment{kind: segChild, selectors: []selector{{kind: selWildcard}}}, true, nil
	}
	if name, err := memberNameShorthand(r); err == nil {
		return segment{kind: segChild, selectors: []selector{{kind: selName, name: name}}}, true, nil
	}
	return segment{}, false, newParseError(save, "expecting a wildcard-selector or member-name shorthand")
}

func tryDescendantSegment(r *reader) (segment, bool, error) {
	if !matchStr("..", r) {
		return segment{}, false, nil
	}
	save := r.cursor()
	if selectors, ok, err := tryBracketedSelection(r); err != nil {
		return segment{}, false, err
	} else if ok {
		return segment{kind: segDescendant, selectors: selectors}, true, nil
	}
	if matchStr("*", r) {
		return segment{kind: segDescendant, selectors: []selector{{kind: selWildcard}}}, true, nil
	}
	if name, err := memberNameShorthand(r); err == nil {
		return segment{kind: segDescendant, selectors: []selector{{kind: selName, name: name}}}, true, nil
	}
	return segment{}, false, newParseError(save,
		"expecting a bracketed-selection, wildcard-selector or member-name shorthand")
}

func tryBracketedSelection(r *reader) ([]selector, bool, error) {
	if !matchStr("[", r) {
		return nil, false, nil
	}
	skipWhitespace(r)
	selectors, err := parseSelectors(r)
	if err != nil {
		return nil, false, err
	}
	skipWhitespace(r)
	if err := expectStr("]", r); err != nil {
		return nil, false, err
	}
	return selectors, true, nil
}

// memberNameShorthand parses the name in a ".name" segment: name-first
// followed by zero or more name-char (RFC 9535 §2.5.1.1).
func memberNameShorthand(r *reader) (string, error) {
	c, ok := nameFirst(r)
	if !ok {
		return "", newParseError(r.cursor(), "expecting a member name")
	}
	var b strings.Builder
	b.WriteRune(c)
	for {
		c, ok := nameChar(r)
		if !ok {
			break
		}
		b.WriteRune(c)
	}
	return b.String(), nil
}

// nameFirst matches an ASCII letter, underscore, or any non-ASCII
// character outside the UTF-16 surrogate range.
func nameFirst(r *reader) (rune, bool) {
	save := r.cursor()
	c, ok := r.read()
	if !ok {
		return 0, false
	}
	if isASCIIAlpha(c) || c == '_' || (c >= 0x80 && c <= 0xD7FF) || (c >= 0xE000 && c <= 0x10FFFF) {
		return c, true
	}
	r.seek(save)
	return 0, false
}

func nameChar(r *reader) (rune, bool) {
	if c, ok := nameFirst(r); ok {
		return c, true
	}
	return asciiDigit(r)
}

func asciiDigit(r *reader) (rune, bool) {
	save := r.cursor()
	c, ok := r.read()
	if !ok {
		return 0, false
	}
	if c >= '0' && c <= '9' {
		return c, true
	}
	r.seek(save)
	return 0, false
}

func isASCIIAlpha(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
