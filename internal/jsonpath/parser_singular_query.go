// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

// trySingularQuery parses a query guaranteed to select at most one node:
// a chain of "['name']"/".name" and "[index]" segments, absolute or
// relative to the current node.
func trySingularQuery(r *reader) (singularQuery, bool, error) {
	if matchStr("$", r) {
		segments, err := parseSingularQuerySegments(r)
		if err != nil {
			return singularQuery{}, false, err
		}
		return singularQuery{absolute: true, segments: segments}, true, nil
	}
	if matchStr("@", r) {
		segments, err := parseSingularQuerySegments(r)
		if err != nil {
			return singularQuery{}, false, err
		}
		return singularQuery{absolute: false, segments: segments}, true, nil
	}
	return singularQuery{}, false, nil
}

func parseSingularQuerySegments(r *reader) ([]singularQuerySegment, error) {
	var segments []singularQuerySegment
	skipWhitespace(r)
	for {
		seg, ok, err := trySingularQuerySegment(r)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		segments = append(segments, seg)
		skipWhitespace(r)
	}
	return segments, nil
}

func trySingularQuerySegment(r *reader) (singularQuerySegment, bool, error) {
	save := r.cursor()

	if matchStr("[", r) {
		if name, ok, err := tryNameSelector(r); err != nil {
			return singularQuerySegment{}, false, err
		} else if ok {
			if err := expectStr("]", r); err != nil {
				return singularQuerySegment{}, false, err
			}
			return singularQuerySegment{isName: true, name: name}, true, nil
		}
		if idx, ok, err := tryInteger(r); err != nil {
			return singularQuerySegment{}, false, err
		} else if ok {
			if err := expectStr("]", r); err != nil {
				return singularQuerySegment{}, false, err
			}
			return singularQuerySegment{isName: false, index: idx}, true, nil
		}
		return singularQuerySegment{}, false, nil
	}

	if matchStr(".", r) {
		name, err := memberNameShorthand(r)
		if err != nil {
			r.seek(save)
			return singularQuerySegment{}, false, nil
		}
		return singularQuerySegment{isName: true, name: name}, true, nil
	}

	return singularQuerySegment{}, false, nil
}
