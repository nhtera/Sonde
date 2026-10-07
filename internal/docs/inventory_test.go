// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package docs

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

// grammarRow is one line of testdata/grammar-inventory.tsv: kind, name.
type grammarRow struct{ kind, name string }

// cliRow is one line of testdata/cli-inventory.tsv: kind, name, short, arg, usage.
type cliRow struct {
	kind, name, short, arg string
	usage                  int
}

func readTSV(t *testing.T, path string, ncols int) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v (run scripts/gen-grammar-inventory.sh and scripts/gen-cli-inventory.sh)", path, err)
	}
	defer f.Close()

	var rows [][]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != ncols {
			t.Fatalf("%s: line %q: want %d columns, got %d", path, line, ncols, len(cols))
		}
		rows = append(rows, cols)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return rows
}

// Inventory files: the conformance gate's version (8.0.1), and the pinned
// upstream snapshot whose additions table.yaml marks `since: 8.1.0`.
const (
	grammarTSV     = "testdata/grammar-inventory.tsv"
	grammarNextTSV = "testdata/grammar-inventory-next.tsv"
	cliTSV         = "testdata/cli-inventory.tsv"
	cliNextTSV     = "testdata/cli-inventory-next.tsv"
)

func loadGrammarInventory(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	byKind := map[string]map[string]bool{}
	for _, cols := range readTSV(t, path, 2) {
		row := grammarRow{kind: cols[0], name: cols[1]}
		if byKind[row.kind] == nil {
			byKind[row.kind] = map[string]bool{}
		}
		byKind[row.kind][row.name] = true
	}
	return byKind
}

func loadCLIInventory(t *testing.T, path string) map[string]map[string]cliRow {
	t.Helper()
	byKind := map[string]map[string]cliRow{}
	for _, cols := range readTSV(t, path, 5) {
		usage, err := strconv.Atoi(cols[4])
		if err != nil {
			t.Fatalf("%s: bad usage %q: %v", path, cols[4], err)
		}
		row := cliRow{kind: cols[0], name: cols[1], short: cols[2], arg: cols[3], usage: usage}
		if byKind[row.kind] == nil {
			byKind[row.kind] = map[string]cliRow{}
		}
		byKind[row.kind][row.name] = row
	}
	return byKind
}

// TestGrammarInventoryMatches asserts table.yaml's queries/filters/
// predicates/functions/options contain exactly the names extracted from
// the reference parser source by scripts/gen-grammar-inventory.sh — no
// missing, no extras. Rows marked `since: 8.1.0` are checked against the
// pinned snapshot's inventory instead.
func TestGrammarInventoryMatches(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	grammar := loadGrammarInventory(t, grammarTSV)
	next := loadGrammarInventory(t, grammarNextTSV)

	cases := map[string][]Entry{
		"queries":    tbl.Queries,
		"filters":    tbl.Filters,
		"predicates": tbl.Predicates,
		"functions":  tbl.Functions,
		"options":    tbl.Options,
	}
	for kind, entries := range cases {
		if len(grammar[kind]) == 0 || len(next[kind]) == 0 {
			t.Fatalf("grammar inventories: no rows for kind %q", kind)
		}
		got := map[string]Entry{}
		for _, e := range entries {
			got[e.Name] = e
		}
		for _, inv := range []struct {
			path  string
			names map[string]bool
		}{{grammarTSV, grammar[kind]}, {grammarNextTSV, next[kind]}} {
			for name := range inv.names {
				if _, ok := got[name]; !ok {
					t.Errorf("table.yaml %s: missing %q (present in %s)", kind, name, inv.path)
				}
			}
		}
		for name, e := range got {
			want, path := grammar[kind], grammarTSV
			if e.Since != "" {
				want, path = next[kind], grammarNextTSV
				if grammar[kind][name] {
					t.Errorf("table.yaml %s %q: since %s, but %s has it", kind, name, e.Since, grammarTSV)
				}
			}
			if !want[name] {
				t.Errorf("table.yaml %s: extra %q (absent from %s)", kind, name, path)
			}
		}
	}
}

// TestCLIInventoryMatches asserts table.yaml's flags/env/options contain
// exactly the names extracted from the reference CLI option definitions by
// scripts/gen-cli-inventory.sh, and that each entry's usage field matches
// the extracted occurrence count. Rows marked `since: 8.1.0` are checked
// against the pinned snapshot's inventory, and its usage counts.
func TestCLIInventoryMatches(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cli := loadCLIInventory(t, cliTSV)
	next := loadCLIInventory(t, cliNextTSV)

	cases := map[string][]Entry{
		"flags":   tbl.Flags,
		"env":     tbl.Env,
		"options": tbl.Options,
	}
	for kind, entries := range cases {
		if len(cli[kind]) == 0 || len(next[kind]) == 0 {
			t.Fatalf("CLI inventories: no rows for kind %q", kind)
		}
		got := map[string]Entry{}
		for _, e := range entries {
			got[e.Name] = e
		}
		for _, inv := range []struct {
			path string
			rows map[string]cliRow
		}{{cliTSV, cli[kind]}, {cliNextTSV, next[kind]}} {
			for name := range inv.rows {
				if _, ok := got[name]; !ok {
					t.Errorf("table.yaml %s: missing %q (present in %s)", kind, name, inv.path)
				}
			}
		}
		for name, e := range got {
			want, path := cli[kind], cliTSV
			if e.Since != "" {
				want, path = next[kind], cliNextTSV
				if _, old := cli[kind][name]; old {
					t.Errorf("table.yaml %s %q: since %s, but %s has it", kind, name, e.Since, cliTSV)
				}
			}
			row, ok := want[name]
			if !ok {
				t.Errorf("table.yaml %s: extra %q (absent from %s)", kind, name, path)
				continue
			}
			if e.Usage != row.usage {
				t.Errorf("table.yaml %s %q: usage = %d, want %d (%s)", kind, name, e.Usage, row.usage, path)
			}
			if kind == "flags" {
				if e.Short != row.short {
					t.Errorf("table.yaml %s %q: short = %q, want %q", kind, name, e.Short, row.short)
				}
				if e.Arg != row.arg {
					t.Errorf("table.yaml %s %q: arg = %q, want %q", kind, name, e.Arg, row.arg)
				}
			}
		}
	}
}

// TestOptionsInventoriesAgree asserts the "options" kind lists the same
// names in the grammar and CLI inventories of each version (the two scripts
// extract it independently from different reference source files).
func TestOptionsInventoriesAgree(t *testing.T) {
	for _, pair := range [][2]string{{grammarTSV, cliTSV}, {grammarNextTSV, cliNextTSV}} {
		grammar := loadGrammarInventory(t, pair[0])["options"]
		cli := loadCLIInventory(t, pair[1])["options"]
		for name := range grammar {
			if _, ok := cli[name]; !ok {
				t.Errorf("options: %q in %s but not %s", name, pair[0], pair[1])
			}
		}
		for name := range cli {
			if _, ok := grammar[name]; !ok {
				t.Errorf("options: %q in %s but not %s", name, pair[1], pair[0])
			}
		}
	}
}
