// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// exportFormats are the hurlfmt --out formats each tests_export fixture is
// compared in, with the suffix that replaces ".hurl" in its expected file.
// It mirrors integration/hurlfmt/test_format.py upstream.
var exportFormats = []struct {
	key    string // manifest key suffix after "#"
	out    string // value passed to hurlfmt --out
	suffix string // expected file: foo.hurl -> foo<suffix>
}{
	{"lint", "hurl", ".lint.hurl"},
	{"json", "json", ".json"},
	{"html", "html", ".html"},
}

// numberedHurlRE matches numbered inputs ("foo.1.hurl"), which the upstream
// runner's accept() is meant to refuse; none exist in the pinned trees.
var numberedHurlRE = regexp.MustCompile(`\.\d+\.hurl$`)

// DiscoverHurlfmt lists the formatter tree's entry points, keyed with the
// "hurlfmt/" prefix: the tests_ok (recursive) and tests_failed scripts, and
// one comparison per tests_export fixture and format whose expected file
// exists, as integration/hurlfmt/integration.py does upstream.
func DiscoverHurlfmt(root string) ([]Script, error) {
	var out []Script

	okScripts, err := findScripts(filepath.Join(root, "tests_ok"), true)
	if err != nil {
		return nil, err
	}
	failedScripts, err := findScripts(filepath.Join(root, "tests_failed"), false)
	if err != nil {
		return nil, err
	}
	for _, abs := range append(okScripts, failedScripts...) {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		out = append(out, Script{Path: TreeHurlfmt + "/" + rel, Tree: TreeHurlfmt, Rel: rel, Lane: LaneHurlfmt})
	}

	inputs, err := filepath.Glob(filepath.Join(root, "tests_export", "*.hurl"))
	if err != nil {
		return nil, err
	}
	for _, abs := range inputs {
		if strings.HasSuffix(abs, ".lint.hurl") || numberedHurlRE.MatchString(abs) {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		for _, f := range exportFormats {
			if _, err := os.Stat(exportExpectedPath(abs, f.suffix)); err != nil {
				continue
			}
			out = append(out, Script{
				Path:   TreeHurlfmt + "/" + rel + "#" + f.key,
				Tree:   TreeHurlfmt,
				Rel:    rel,
				Export: f.out,
				Lane:   LaneHurlfmt,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func exportExpectedPath(hurlPath, suffix string) string {
	return strings.TrimSuffix(hurlPath, ".hurl") + suffix
}

// exportSuffix returns the expected-file suffix for an --out format.
func exportSuffix(out string) string {
	for _, f := range exportFormats {
		if f.out == out {
			return f.suffix
		}
	}
	return ""
}

// checkExport scores one export comparison the way test_format.py does:
// only stdout is compared (decoded, against the expected file read with
// universal newlines); the exit code and stderr are not checked.
func checkExport(hurlPath, out string, got processResult) oracleVerdict {
	v := oracleVerdict{ActualExit: got.ExitCode, ExitOK: true, StderrOK: true, StdoutChecked: true}
	expected, err := os.ReadFile(exportExpectedPath(hurlPath, exportSuffix(out))) //nolint:gosec // G304: vendored fixtures.
	if err != nil {
		v.Reason = fmt.Sprintf("reading expected %s output: %v", out, err)
		return v
	}
	v.StdoutOK = decodeString(got.Stdout) == universalNewlines(decodeString(expected))
	if !v.StdoutOK {
		v.Reason = fmt.Sprintf("stdout: --out %s differs (exit %d)", out, got.ExitCode)
	}
	return v
}
