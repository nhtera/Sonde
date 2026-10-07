// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package docs

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

func TestLoad(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Queries) == 0 || len(tbl.Filters) == 0 || len(tbl.Predicates) == 0 ||
		len(tbl.Functions) == 0 || len(tbl.Options) == 0 || len(tbl.Flags) == 0 ||
		len(tbl.Env) == 0 || len(tbl.Config) == 0 || len(tbl.Differences) == 0 {
		t.Fatal("Load: expected every kind to have at least one entry")
	}
}

// TestLoadCached checks Load returns the same *Table instance on repeat
// calls (sync.OnceValues caching, no package-level mutable state re-parsed).
func TestLoadCached(t *testing.T) {
	a, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("Load: expected the same cached *Table on repeat calls")
	}
}

func TestLookup(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := tbl.Lookup("queries", "jsonpath")
	if !ok {
		t.Fatal(`Lookup("queries", "jsonpath"): not found`)
	}
	if e.Status != StatusSupported {
		t.Errorf("jsonpath query status = %q, want %q", e.Status, StatusSupported)
	}

	if _, ok := tbl.Lookup("queries", "does-not-exist"); ok {
		t.Error(`Lookup("queries", "does-not-exist"): expected not found`)
	}
	if _, ok := tbl.Lookup("no-such-kind", "status"); ok {
		t.Error(`Lookup("no-such-kind", "status"): expected not found`)
	}
}

func TestSortedNamesUnique(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for kind, entries := range tbl.kinds() {
		names := sortedNames(entries)
		if !sort.StringsAreSorted(names) {
			t.Errorf("%s: sortedNames not sorted: %v", kind, names)
		}
		for i := 1; i < len(names); i++ {
			if names[i] == names[i-1] {
				t.Errorf("%s: duplicate name %q", kind, names[i])
			}
		}
	}
}

func TestValidateRejectsUnknownStatus(t *testing.T) {
	tbl := &Table{Queries: []Entry{{Name: "x", Status: "bogus", Doc: "d"}}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for an unknown status")
	}
}

func TestValidateRejectsDuplicateName(t *testing.T) {
	tbl := &Table{Queries: []Entry{
		{Name: "x", Status: StatusSupported, Doc: "d"},
		{Name: "x", Status: StatusSupported, Doc: "d"},
	}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for a duplicate name")
	}
}

func TestValidateRejectsEmptyName(t *testing.T) {
	tbl := &Table{Queries: []Entry{{Name: "", Status: StatusSupported, Doc: "d"}}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for an empty name")
	}
}

func TestValidateRejectsMissingDoc(t *testing.T) {
	tbl := &Table{Queries: []Entry{{Name: "x", Status: StatusSupported}}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for a missing doc")
	}
}

func TestValidateRequiresReasonForUnsupported(t *testing.T) {
	tbl := &Table{Flags: []Entry{{Name: "--x", Status: StatusUnsupported, Doc: "d"}}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for unsupported without a reason")
	}
}

func TestValidateRequiresReasonForPartial(t *testing.T) {
	tbl := &Table{Flags: []Entry{{Name: "--x", Status: StatusPartial, Doc: "d"}}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for partial without a reason")
	}
}

func TestValidateRequiresPhaseForPlanned(t *testing.T) {
	tbl := &Table{Flags: []Entry{{Name: "--x", Status: StatusPlanned, Doc: "d"}}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for planned without a phase")
	}
}

func TestValidateAcceptsSupportedWithoutPhaseOrReason(t *testing.T) {
	tbl := &Table{Queries: []Entry{{Name: "x", Status: StatusSupported, Doc: "d"}}}
	if err := validate(tbl); err != nil {
		t.Fatalf("validate: unexpected error: %v", err)
	}
}

func TestValidateRejectsIncompleteDifference(t *testing.T) {
	tbl := &Table{Differences: []Difference{{Area: "Syntax", Input: "x"}}}
	if err := validate(tbl); err == nil {
		t.Fatal("validate: expected an error for an incomplete difference row")
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	bad := []byte("queries:\n  - name: x\n    status: supported\n    doc: d\n    bogus: 1\n")
	dec := yaml.NewDecoder(bytes.NewReader(bad))
	dec.KnownFields(true)
	var tbl Table
	if err := dec.Decode(&tbl); err == nil {
		t.Fatal("decode: expected an error for an unknown field")
	}
}

// TestDifferencesGiveAReason checks that every difference row says why
// sonde differs, ending its sonde text with "(why: …)".
func TestDifferencesGiveAReason(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range tbl.Differences {
		if !strings.Contains(d.Sonde, "(why: ") || !strings.HasSuffix(d.Sonde, ")") {
			t.Errorf("%s / %s: no (why: …) reason", d.Area, d.Input)
		}
	}
}
