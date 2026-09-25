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

func loadGrammarInventory(t *testing.T) map[string]map[string]bool {
	t.Helper()
	byKind := map[string]map[string]bool{}
	for _, cols := range readTSV(t, "testdata/grammar-inventory.tsv", 2) {
		row := grammarRow{kind: cols[0], name: cols[1]}
		if byKind[row.kind] == nil {
			byKind[row.kind] = map[string]bool{}
		}
		byKind[row.kind][row.name] = true
	}
	return byKind
}

func loadCLIInventory(t *testing.T) map[string]map[string]cliRow {
	t.Helper()
	byKind := map[string]map[string]cliRow{}
	for _, cols := range readTSV(t, "testdata/cli-inventory.tsv", 5) {
		usage, err := strconv.Atoi(cols[4])
		if err != nil {
			t.Fatalf("cli-inventory.tsv: bad usage %q: %v", cols[4], err)
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
// missing, no extras.
func TestGrammarInventoryMatches(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	grammar := loadGrammarInventory(t)

	cases := map[string][]Entry{
		"queries":    tbl.Queries,
		"filters":    tbl.Filters,
		"predicates": tbl.Predicates,
		"functions":  tbl.Functions,
		"options":    tbl.Options,
	}
	for kind, entries := range cases {
		want := grammar[kind]
		if len(want) == 0 {
			t.Fatalf("testdata/grammar-inventory.tsv: no rows for kind %q", kind)
		}
		got := map[string]bool{}
		for _, e := range entries {
			got[e.Name] = true
		}
		for name := range want {
			if !got[name] {
				t.Errorf("table.yaml %s: missing %q (present in grammar-inventory.tsv)", kind, name)
			}
		}
		for name := range got {
			if !want[name] {
				t.Errorf("table.yaml %s: extra %q (absent from grammar-inventory.tsv)", kind, name)
			}
		}
	}
}

// TestCLIInventoryMatches asserts table.yaml's flags/env/options contain
// exactly the names extracted from the reference CLI option definitions by
// scripts/gen-cli-inventory.sh, and that each entry's usage field matches
// the extracted occurrence count.
func TestCLIInventoryMatches(t *testing.T) {
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cli := loadCLIInventory(t)

	cases := map[string][]Entry{
		"flags":   tbl.Flags,
		"env":     tbl.Env,
		"options": tbl.Options,
	}
	for kind, entries := range cases {
		want := cli[kind]
		if len(want) == 0 {
			t.Fatalf("testdata/cli-inventory.tsv: no rows for kind %q", kind)
		}
		got := map[string]Entry{}
		for _, e := range entries {
			got[e.Name] = e
		}
		for name, row := range want {
			e, ok := got[name]
			if !ok {
				t.Errorf("table.yaml %s: missing %q (present in cli-inventory.tsv)", kind, name)
				continue
			}
			if e.Usage != row.usage {
				t.Errorf("table.yaml %s %q: usage = %d, want %d (cli-inventory.tsv)", kind, name, e.Usage, row.usage)
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
		for name := range got {
			if _, ok := want[name]; !ok {
				t.Errorf("table.yaml %s: extra %q (absent from cli-inventory.tsv)", kind, name)
			}
		}
	}
}

// TestOptionsInventoriesAgree asserts the "options" kind lists the same
// names in both generated .tsv files (the grammar and CLI scripts extract
// it independently from different reference source files).
func TestOptionsInventoriesAgree(t *testing.T) {
	grammar := loadGrammarInventory(t)["options"]
	cli := loadCLIInventory(t)["options"]
	for name := range grammar {
		if _, ok := cli[name]; !ok {
			t.Errorf("options: %q in grammar-inventory.tsv but not cli-inventory.tsv", name)
		}
	}
	for name := range cli {
		if _, ok := grammar[name]; !ok {
			t.Errorf("options: %q in cli-inventory.tsv but not grammar-inventory.tsv", name)
		}
	}
}
