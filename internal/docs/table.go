// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package docs holds Sonde's single compatibility doc-data source
// (table.yaml) and its typed accessors. docs/compat.md and the LSP's
// hover text are both rendered from this table so they can never drift from
// each other.
package docs

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"
	"sync"

	yaml "go.yaml.in/yaml/v3"
)

//go:embed table.yaml
var tableYAML []byte

// Status is how far along Sonde's support for an entry is.
type Status string

// Statuses.
const (
	StatusSupported   Status = "supported"
	StatusPartial     Status = "partial"
	StatusPlanned     Status = "planned"
	StatusUnsupported Status = "unsupported"
)

func (s Status) valid() bool {
	switch s {
	case StatusSupported, StatusPartial, StatusPlanned, StatusUnsupported:
		return true
	default:
		return false
	}
}

// Entry is one query, filter, predicate, template function, request
// [Options] key, CLI flag, environment variable or config-file key.
type Entry struct {
	Name   string `yaml:"name"`
	Status Status `yaml:"status"`
	Phase  int    `yaml:"phase,omitempty"`
	Doc    string `yaml:"doc"`
	Reason string `yaml:"reason,omitempty"`
	Usage  int    `yaml:"usage,omitempty"`
	Short  string `yaml:"short,omitempty"`
	Arg    string `yaml:"arg,omitempty"`
	// Deprecated names the replacement of a deprecated entry.
	Deprecated string `yaml:"deprecated,omitempty"`
}

// Difference is one row of documented behavior that differs from the
// reference implementation for inputs outside the conformance test tree.
type Difference struct {
	Area  string `yaml:"area"`
	Input string `yaml:"input"`
	Hurl  string `yaml:"hurl"`
	Sonde string `yaml:"sonde"`
}

// Table is the decoded, validated content of table.yaml.
type Table struct {
	Queries     []Entry      `yaml:"queries"`
	Filters     []Entry      `yaml:"filters"`
	Predicates  []Entry      `yaml:"predicates"`
	Functions   []Entry      `yaml:"functions"`
	Options     []Entry      `yaml:"options"`
	Flags       []Entry      `yaml:"flags"`
	Env         []Entry      `yaml:"env"`
	Config      []Entry      `yaml:"config"`
	Sonde       SondeTable   `yaml:"sonde"`
	Differences []Difference `yaml:"differences"`
}

// SondeTable lists the Sonde extensions of the file format, valid only in
// .sonde files.
type SondeTable struct {
	Sections []Entry `yaml:"sections"`
	Steps    []Entry `yaml:"steps"`
	Options  []Entry `yaml:"options"`
	Queries  []Entry `yaml:"queries"`
}

// kinds lists every Table field usable with Lookup, in table.yaml's own key
// order.
func (t *Table) kinds() map[string][]Entry {
	return map[string][]Entry{
		"queries":    t.Queries,
		"filters":    t.Filters,
		"predicates": t.Predicates,
		"functions":  t.Functions,
		"options":    t.Options,
		"flags":      t.Flags,
		"env":        t.Env,
		"config":     t.Config,

		"sonde-sections": t.Sonde.Sections,
		"sonde-steps":    t.Sonde.Steps,
		"sonde-options":  t.Sonde.Options,
		"sonde-queries":  t.Sonde.Queries,
	}
}

// Lookup returns the entry named name within kind ("queries", "filters",
// "predicates", "functions", "options", "flags", "env", "config", or a
// Sonde extension kind: "sonde-sections", "sonde-steps", "sonde-options",
// "sonde-queries"), used by the LSP for hover text.
func (t *Table) Lookup(kind, name string) (Entry, bool) {
	for _, e := range t.kinds()[kind] {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

func validate(t *Table) error {
	for kind, entries := range t.kinds() {
		seen := make(map[string]bool, len(entries))
		for _, e := range entries {
			if e.Name == "" {
				return fmt.Errorf("docs: %s: entry with empty name", kind)
			}
			if seen[e.Name] {
				return fmt.Errorf("docs: %s: duplicate name %q", kind, e.Name)
			}
			seen[e.Name] = true
			if e.Deprecated != "" {
				repl, ok := t.Lookup(kind, e.Deprecated)
				if !ok || repl.Deprecated != "" || repl.Name == e.Name {
					return fmt.Errorf("docs: %s %q: deprecated in favor of %q, which must be another, current entry", kind, e.Name, e.Deprecated)
				}
			}
			if !e.Status.valid() {
				return fmt.Errorf("docs: %s %q: invalid status %q", kind, e.Name, e.Status)
			}
			if e.Doc == "" {
				return fmt.Errorf("docs: %s %q: missing doc", kind, e.Name)
			}
			switch e.Status {
			case StatusPartial, StatusUnsupported:
				if e.Reason == "" {
					return fmt.Errorf("docs: %s %q: status %q requires a reason", kind, e.Name, e.Status)
				}
			case StatusPlanned:
				if e.Phase == 0 {
					return fmt.Errorf("docs: %s %q: status %q requires a phase", kind, e.Name, e.Status)
				}
			}
		}
	}
	for i, d := range t.Differences {
		if d.Area == "" || d.Input == "" || d.Hurl == "" || d.Sonde == "" {
			return fmt.Errorf("docs: differences[%d]: all of area/input/hurl/sonde are required", i)
		}
	}
	return nil
}

func decode() (*Table, error) {
	dec := yaml.NewDecoder(bytes.NewReader(tableYAML))
	dec.KnownFields(true)
	var t Table
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("docs: decode table.yaml: %w", err)
	}
	if err := validate(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

var loadOnce = sync.OnceValues(decode)

// Load returns the parsed, validated table.yaml. The result is cached; the
// returned *Table must be treated as read-only.
func Load() (*Table, error) {
	return loadOnce()
}

// sortedNames is a small test helper kept here (not test-only) so both
// table_test.go and inventory_test.go can use it without exporting mutable
// state.
func sortedNames(entries []Entry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	sort.Strings(names)
	return names
}
