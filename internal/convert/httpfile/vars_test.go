// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
)

func TestResolveVarsChain(t *testing.T) {
	order, raw := fileVarOrder([]fileVarDef{
		{name: "host", rawValue: "example.com"},
		{name: "base", rawValue: "https://{{host}}/api"},
		{name: "list", rawValue: "{{base}}/items"},
	})
	resolved, warns := resolveVars(order, raw, "file variable")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if resolved["list"] != "https://example.com/api/items" {
		t.Errorf("list = %q", resolved["list"])
	}
}

func TestResolveVarsForwardReference(t *testing.T) {
	order, raw := fileVarOrder([]fileVarDef{
		{name: "a", rawValue: "{{b}}/x"},
		{name: "b", rawValue: "y"},
	})
	resolved, warns := resolveVars(order, raw, "file variable")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if resolved["a"] != "y/x" {
		t.Errorf("a = %q", resolved["a"])
	}
}

func TestResolveVarsCycle(t *testing.T) {
	order, raw := fileVarOrder([]fileVarDef{
		{name: "a", rawValue: "{{b}}"},
		{name: "b", rawValue: "{{a}}"},
	})
	_, warns := resolveVars(order, raw, "file variable")
	if len(warns) == 0 {
		t.Fatal("expected a circular-reference warning")
	}
	found := false
	for _, w := range warns {
		if w.Kind == convert.WarnUnsupported && strings.Contains(w.Message, "circular") {
			found = true
		}
	}
	if !found {
		t.Errorf("warns = %v", warns)
	}
}

func TestResolveVarsUnknownReferenceStaysLiteral(t *testing.T) {
	order, raw := fileVarOrder([]fileVarDef{{name: "a", rawValue: "{{undefined}}/x"}})
	resolved, warns := resolveVars(order, raw, "file variable")
	if resolved["a"] != "{{undefined}}/x" {
		t.Errorf("a = %q", resolved["a"])
	}
	if len(warns) != 1 {
		t.Fatalf("warns = %v", warns)
	}
}

func TestResolveVarsDynamicStaysLiteral(t *testing.T) {
	order, raw := fileVarOrder([]fileVarDef{{name: "id", rawValue: "{{$uuid}}"}})
	resolved, warns := resolveVars(order, raw, "file variable")
	if resolved["id"] != "{{$uuid}}" {
		t.Errorf("id = %q", resolved["id"])
	}
	if len(warns) != 1 {
		t.Fatalf("warns = %v", warns)
	}
}

func TestFileVarOrderLastWins(t *testing.T) {
	order, raw := fileVarOrder([]fileVarDef{
		{name: "a", rawValue: "1"},
		{name: "b", rawValue: "2"},
		{name: "a", rawValue: "3"},
	})
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("order = %v", order)
	}
	if raw["a"] != "3" {
		t.Errorf("a = %q", raw["a"])
	}
}

func TestMergeVarNames(t *testing.T) {
	m, warns := mergeVarNames(nil, []string{"a.b", "newDate"}, map[string]string{"a.b": "1", "newDate": "2"})
	if m["a_b"] != "1" || m["var_newDate"] != "2" {
		t.Errorf("m = %v", m)
	}
	if len(warns) != 0 {
		t.Errorf("warns = %v", warns)
	}
}

// TestMergeVarNamesCollisionWarns checks M13: two distinct names that
// sanitize to the same VariableName collide deterministically (later in
// order wins) and warn, rather than resolving nondeterministically as a Go
// map range would.
func TestMergeVarNamesCollisionWarns(t *testing.T) {
	m, warns := mergeVarNames(nil, []string{"a.b", "a_b"}, map[string]string{"a.b": "one", "a_b": "two"})
	if m["a_b"] != "two" {
		t.Errorf("a_b = %q, want the later name's value", m["a_b"])
	}
	if len(warns) != 1 || warns[0].Kind != convert.WarnUnsupported {
		t.Fatalf("warns = %v", warns)
	}
	// Reversed order: the other name now wins, still deterministic.
	m2, _ := mergeVarNames(nil, []string{"a_b", "a.b"}, map[string]string{"a.b": "one", "a_b": "two"})
	if m2["a_b"] != "one" {
		t.Errorf("a_b = %q, want the later name's value", m2["a_b"])
	}
}

// TestMergeVarNamesSeedThenOverride checks the M7 pattern: seeding with one
// layer, then merging a second layer on top, lets the second layer's values
// win without a spurious collision warning (that's expected layering, not a
// same-layer collision).
func TestMergeVarNamesSeedThenOverride(t *testing.T) {
	m, w1 := mergeVarNames(nil, []string{"host"}, map[string]string{"host": "default.test"})
	if len(w1) != 0 {
		t.Fatalf("seed warns = %v", w1)
	}
	m, w2 := mergeVarNames(m, []string{"host"}, map[string]string{"host": "prod.test"})
	if len(w2) != 0 {
		t.Fatalf("override warns = %v", w2)
	}
	if m["host"] != "prod.test" {
		t.Errorf("host = %q", m["host"])
	}
}
