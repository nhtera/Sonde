// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runsvc

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/report"
	"github.com/nhtera/sonde/internal/testsummary"
)

// testText is a test run's summary as `sonde --test` prints it: a line
// per file (as they finished), then the totals.
func testText(results []*engine.UnitResult, duration time.Duration) string {
	var b strings.Builder
	succeeded, requests := 0, 0
	for _, res := range results {
		b.WriteString(testsummary.Line(res, false))
		if res.Success {
			succeeded++
		}
		for _, e := range res.Entries {
			requests += len(e.Calls)
		}
	}
	b.WriteString(testsummary.Summary(len(results), succeeded, requests, duration))
	return b.String()
}

// shown is the results as the command shows them: each file named by its
// project path (the command runs with --file-root .), as they finished.
func (rn *run) shown() []*engine.UnitResult {
	out := make([]*engine.UnitResult, len(rn.results))
	for i, res := range rn.results {
		c := *res
		if rel, ok := rn.files[res.File]; ok {
			c.File = rel
		}
		out[i] = &c
	}
	return out
}

// keepTest keeps a test run's results for its reports (the last few), in
// the order of the files given, as the command's reports list them.
func (r *Runs) keepTest(id string, rn *run) {
	seqs := make([]int, len(rn.results))
	for i, res := range rn.results {
		seqs[i] = rn.seqs[res]
	}
	results := rn.shown()
	order := make([]int, len(results))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(seqs[a], seqs[b]) })
	sorted := make([]*engine.UnitResult, len(results))
	for i, j := range order {
		sorted[i] = results[j]
	}
	// Errors name their file by its path on disk: by its project path in
	// the reports, as with --file-root . (the file's own name).
	prefix := rn.root.Dir() + string(filepath.Separator)
	redact := func(s string) string { return strings.ReplaceAll(rn.redact(s), prefix, "") }
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tests = append(r.tests, testRun{id: id, results: sorted, redact: redact})
	if len(r.tests) > keptTests {
		r.tests = r.tests[len(r.tests)-keptTests:]
	}
}

// Report formats and the file each writes in the folder picked (html and
// json write a folder of files, as their command flags do).
var reports = map[string]string{"html": "", "json": "", "junit": "junit.xml", "tap": "report.tap"}

// Export writes test run id's report in format (html, json, junit, tap)
// into a new folder in dir (sonde-<format>-<time>…: a report written
// twice would append to or overwrite the first), redacted as the run was;
// it returns what it wrote.
func (r *Runs) Export(id, format, dir string) (string, error) {
	file, ok := reports[format]
	if !ok {
		return "", apperr.New(apperr.Invalid, "unknown report format "+format)
	}
	r.mu.Lock()
	i := slices.IndexFunc(r.tests, func(t testRun) bool { return t.id == id })
	var t testRun
	if i >= 0 {
		t = r.tests[i]
	}
	r.mu.Unlock()
	if i < 0 {
		return "", apperr.New(apperr.NotFound, "this test run is no longer kept: run it again")
	}
	dir, err := os.MkdirTemp(dir, "sonde-"+format+"-"+time.Now().Format("20060102-150405")+"-")
	if err != nil {
		return "", err
	}
	switch format {
	case "html":
		err = report.WriteHTML(dir, t.results, t.redact)
	case "json":
		err = report.WriteJSON(dir, t.results, t.redact)
	case "junit":
		err = report.WriteJUnit(filepath.Join(dir, file), t.results, t.redact)
	case "tap":
		err = report.WriteTAP(filepath.Join(dir, file), t.results, t.redact)
	}
	if err != nil {
		return "", err
	}
	if file == "" {
		return dir, nil
	}
	return filepath.Join(dir, file), nil
}
