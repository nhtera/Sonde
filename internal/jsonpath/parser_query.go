// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

// tryFilterQuery parses the query embedded in a filter selector's test
// expression or a function's NodesType argument: relative ("@...") first,
// then absolute ("$...").
func tryFilterQuery(r *reader) (filterQuery, bool, error) {
	if fq, ok, err := tryRelativeQuery(r); err != nil || ok {
		return fq, ok, err
	}
	return tryAbsoluteQuery(r)
}

func tryRelativeQuery(r *reader) (filterQuery, bool, error) {
	if !matchStr("@", r) {
		return filterQuery{}, false, nil
	}
	segments, err := parseSegments(r)
	if err != nil {
		return filterQuery{}, false, err
	}
	return filterQuery{absolute: false, segments: segments}, true, nil
}

func tryAbsoluteQuery(r *reader) (filterQuery, bool, error) {
	if !matchStr("$", r) {
		return filterQuery{}, false, nil
	}
	segments, err := parseSegments(r)
	if err != nil {
		return filterQuery{}, false, err
	}
	return filterQuery{absolute: true, segments: segments}, true, nil
}
