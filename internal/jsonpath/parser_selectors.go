// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

// parseSelectors parses a comma-separated selector-list inside brackets.
func parseSelectors(r *reader) ([]selector, error) {
	var selectors []selector
	for {
		sel, err := parseSelector(r)
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, sel)
		skipWhitespace(r)
		if !matchStr(",", r) {
			break
		}
		skipWhitespace(r)
	}
	return selectors, nil
}

func parseSelector(r *reader) (selector, error) {
	start := r.cursor()

	if name, ok, err := tryNameSelector(r); err != nil {
		return selector{}, err
	} else if ok {
		return selector{kind: selName, name: name}, nil
	}
	if matchStr("*", r) {
		return selector{kind: selWildcard}, nil
	}
	if sel, ok, err := tryArraySliceSelector(r); err != nil {
		return selector{}, err
	} else if ok {
		return sel, nil
	}
	if idx, ok, err := tryIndexSelector(r); err != nil {
		return selector{}, err
	} else if ok {
		return selector{kind: selIndex, index: idx}, nil
	}
	if sel, ok, err := tryFilterSelector(r); err != nil {
		return selector{}, err
	} else if ok {
		return sel, nil
	}

	return selector{}, newParseError(start, "expecting a selector")
}

func tryNameSelector(r *reader) (string, bool, error) {
	return tryStringLiteral(r)
}

func tryIndexSelector(r *reader) (int64, bool, error) {
	return tryInteger(r)
}

func tryArraySliceSelector(r *reader) (selector, bool, error) {
	save := r.cursor()

	var start *int64
	if v, ok, err := tryInteger(r); err != nil {
		return selector{}, false, err
	} else if ok {
		skipWhitespace(r)
		start = &v
	}

	if !matchStr(":", r) {
		// Not a slice selector; may still be a valid index or name selector.
		r.seek(save)
		return selector{}, false, nil
	}
	skipWhitespace(r)

	var end *int64
	if v, ok, err := tryInteger(r); err != nil {
		return selector{}, false, err
	} else if ok {
		skipWhitespace(r)
		end = &v
	}

	step := int64(1)
	if matchStr(":", r) {
		skipWhitespace(r)
		if v, ok, err := tryInteger(r); err != nil {
			return selector{}, false, err
		} else if ok {
			step = v
		}
	}

	return selector{kind: selSlice, sliceStart: start, sliceEnd: end, sliceStep: step}, true, nil
}

func tryFilterSelector(r *reader) (selector, bool, error) {
	if !matchStr("?", r) {
		return selector{}, false, nil
	}
	skipWhitespace(r)
	expr, err := parseLogicalOrExpr(r)
	if err != nil {
		return selector{}, false, err
	}
	return selector{kind: selFilter, filter: expr}, true, nil
}
