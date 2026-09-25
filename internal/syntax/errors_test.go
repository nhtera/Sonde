// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// validInErrorSuite are files of the parser-error suite that are valid on
// purpose (they accompany an invalid file in a multi-file run).
var validInErrorSuite = map[string]bool{
	"tests_error_parser/parallel_parsing_error_a.hurl": true,
	"tests_error_parser/parallel_parsing_error_b.hurl": true,
	"tests_error_parser/parallel_parsing_error_d.hurl": true,
}

var errLocation = regexp.MustCompile(`--> (\S+):(\d+):(\d+)`)

// TestParserErrorConformance checks every file of the parser-error suite is
// rejected, and compares the reported position and rendering with the
// oracle's .err file. Known differences live in testdata/syntax/error_diffs.txt.
func TestParserErrorConformance(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(conformanceDir, "tests_error_parser", "*.hurl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no parser-error files: %v", err)
	}
	known := readKnownDiffs(t)
	var lineMatch, exact, compared int
	var diffs []string
	for _, path := range files {
		name := conformanceName(path)
		src := readFile(t, path)
		_, err := Parse(name, src, DialectHurl)
		if validInErrorSuite[name] {
			if err != nil {
				t.Errorf("%s: %v, want success", name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: parsed, want a parse error", name)
			continue
		}
		perr := err.(*Error)
		oracle, ok := oracleFor(path, name)
		if !ok {
			continue
		}
		compared++
		rendered := "error: " + perr.Render(name, src)
		if strings.Contains(oracle, rendered) {
			exact++
		}
		m := errLocation.FindStringSubmatch(oracle)
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		switch {
		case perr.Pos.Line != line:
			diffs = append(diffs, name+" line")
		case perr.Pos.Col != col:
			lineMatch++
			diffs = append(diffs, name+" column")
		default:
			lineMatch++
		}
	}
	sort.Strings(diffs)
	for _, d := range diffs {
		if !known[d] {
			t.Errorf("unexpected position difference: %s (add to error_diffs.txt only with a reason)", d)
		}
	}
	for d := range known {
		if !contains(diffs, d) {
			t.Errorf("stale entry in error_diffs.txt: %s", d)
		}
	}
	if compared > 0 && lineMatch*10 < compared*9 {
		t.Errorf("error line matches %d/%d, want >= 90%%", lineMatch, compared)
	}
	t.Logf("parser errors: %d files, %d compared, line match %d, exact render %d", len(files), compared, lineMatch, exact)
}

// oracleFor returns the .err text that reports an error for name.
func oracleFor(path, name string) (string, bool) {
	errs, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.err"))
	for _, e := range errs {
		b, err := os.ReadFile(e)
		if err == nil && strings.Contains(string(b), "--> "+name+":") {
			return extractFor(string(b), name), true
		}
	}
	return "", false
}

// extractFor returns the error block of an .err file that mentions name.
func extractFor(oracle, name string) string {
	blocks := strings.Split(oracle, "error: ")
	for _, b := range blocks {
		if strings.Contains(b, "--> "+name+":") {
			return "error: " + b
		}
	}
	return oracle
}

func readKnownDiffs(t *testing.T) map[string]bool {
	t.Helper()
	known := map[string]bool{}
	b, err := os.ReadFile("../../testdata/syntax/error_diffs.txt")
	if os.IsNotExist(err) {
		return known
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line, _, _ = strings.Cut(line, "#")
		if line = strings.TrimSpace(line); line != "" {
			known[line] = true
		}
	}
	return known
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
