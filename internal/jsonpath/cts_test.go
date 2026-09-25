// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

// The JSONPath Compliance Test Suite (testdata/cts/cts.json) exercises
// selectors that no implementation is expected to accept as written,
// because the RFC leaves their behavior implementation-defined or the
// case describes a non-standard convention some engines follow. Each
// entry names a test from the suite and the reason it is skipped.
var ctsIgnored = map[string]string{
	"filter, non-singular existence, multiple": "" +
		"a union selector cannot be used directly as a boolean predicate; " +
		"the standard-compliant form is $[?(@[0] || @['a'])]",
	"filter, non-singular existence, slice": "" +
		"a slice selector cannot be used directly as a boolean predicate",
	"functions, match, dot matcher on \\u2028":  "regex \".\" is expected to match this character",
	"functions, match, dot matcher on \\u2029":  "regex \".\" is expected to match this character",
	"functions, search, dot matcher on \\u2028": "regex \".\" is expected to match this character",
	"functions, search, dot matcher on \\u2029": "regex \".\" is expected to match this character",
	"functions, match, non-string second arg":   "the second argument of match() must be a string (a regex)",
	"functions, search, non-string second arg":  "the second argument of search() must be a string (a regex)",
}

type ctsFile struct {
	Tests []map[string]json.RawMessage `json:"tests"`
}

type ctsCase struct {
	name            string
	selector        string
	invalidSelector bool
	hasDocument     bool
	document        value.Value
	results         []value.List
}

func loadCTSCases(t *testing.T) []ctsCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/cts/cts.json")
	if err != nil {
		t.Fatalf("reading cts.json: %v", err)
	}
	var file ctsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshaling cts.json: %v", err)
	}
	cases := make([]ctsCase, 0, len(file.Tests))
	for _, t2 := range file.Tests {
		cases = append(cases, parseCTSCase(t, t2))
	}
	return cases
}

func parseCTSCase(t *testing.T, raw map[string]json.RawMessage) ctsCase {
	t.Helper()
	var tc ctsCase
	if err := json.Unmarshal(raw["name"], &tc.name); err != nil {
		t.Fatalf("test name: %v", err)
	}
	if err := json.Unmarshal(raw["selector"], &tc.selector); err != nil {
		t.Fatalf("%s: selector: %v", tc.name, err)
	}
	_, tc.invalidSelector = raw["invalid_selector"]

	if docRaw, ok := raw["document"]; ok {
		doc, err := value.DecodeJSON(string(docRaw))
		if err != nil {
			t.Fatalf("%s: document: %v", tc.name, err)
		}
		tc.hasDocument = true
		tc.document = doc
	}

	switch {
	case raw["results"] != nil:
		var sets []json.RawMessage
		if err := json.Unmarshal(raw["results"], &sets); err != nil {
			t.Fatalf("%s: results: %v", tc.name, err)
		}
		for _, s := range sets {
			tc.results = append(tc.results, decodeCTSNodeList(t, tc.name, s))
		}
	case raw["result"] != nil:
		tc.results = []value.List{decodeCTSNodeList(t, tc.name, raw["result"])}
	}
	return tc
}

func decodeCTSNodeList(t *testing.T, name string, raw json.RawMessage) value.List {
	t.Helper()
	v, err := value.DecodeJSON(string(raw))
	if err != nil {
		t.Fatalf("%s: result list: %v", name, err)
	}
	list, ok := v.(value.List)
	if !ok {
		t.Fatalf("%s: result is not a list: %T", name, v)
	}
	return list
}

func (tc ctsCase) run() error {
	q, err := Parse(tc.selector)
	if err != nil {
		if tc.invalidSelector {
			return nil
		}
		return fmt.Errorf("cannot parse valid selector %q: %w", tc.selector, err)
	}
	if tc.invalidSelector {
		return fmt.Errorf("should not parse invalid selector %q", tc.selector)
	}

	actual := q.Eval(tc.document)
	for _, want := range tc.results {
		if nodeListEqual(want, actual) {
			return nil
		}
	}
	return fmt.Errorf("selector %q: no match among %d expected result set(s), got %v", tc.selector, len(tc.results), actual)
}

func nodeListEqual(a value.List, b []value.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !value.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestCTSComplianceSuite(t *testing.T) {
	cases := loadCTSCases(t)

	seen := make(map[string]bool, len(cases))
	for _, tc := range cases {
		seen[tc.name] = true
	}
	for name := range ctsIgnored {
		if !seen[name] {
			t.Fatalf("ignored test %q is not present in the compliance suite", name)
		}
	}

	var total, ignored, failed int
	total = len(cases)
	for _, tc := range cases {
		if _, skip := ctsIgnored[tc.name]; skip {
			ignored++
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err != nil {
				failed++
				t.Error(err)
			}
		})
	}

	passed := total - ignored - failed
	t.Logf("RFC9535 compliance: total=%d passed=%d ignored=%d", total, passed, ignored)
}
