// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nhtera/sonde/engine"
)

// WriteJUnit appends one <testsuite> summarizing results, in the given
// order, to the JUnit report at path: one <testcase> per result, holding
// a <failure> per assert error and an <error> per runtime error (grouped,
// failures first). A run that produced no error is a self-closed, empty
// <testcase> — except a file that never parsed (ParseError set) or a unit
// stopped before its last entry (Interrupted, with no error of its own to
// show for it): both would otherwise look like a pass, so each gets a
// synthetic <error> instead.
//
// Repeated calls with the same path grow the root <testsuites> element by
// one more <testsuite>, leaving every earlier one untouched — the report
// is cumulative across invocations, matching the reference CLI. redact
// masks secret values in every rendered failure/error message. The file
// is written atomically (temp file + rename), so a run killed mid-write
// never leaves a truncated report for the next invocation to choke on.
func WriteJUnit(path string, results []*engine.UnitResult, redact func(string) string) error {
	root, err := readJUnitRoot(path)
	if err != nil {
		return err
	}
	root.addChild(buildJUnitTestsuite(results, redact))

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := writeXMLDocument(&buf, root); err != nil {
		return err
	}
	return atomicWriteFile(path, buf.Bytes(), 0o644) //nolint:gosec // G306: report files are not secret
}

// readJUnitRoot returns the <testsuites> root of the JUnit report already
// at path, or a fresh empty one when path does not exist yet.
func readJUnitRoot(path string) (*xmlNode, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is a CLI-trusted --report-junit flag
	if err != nil {
		if os.IsNotExist(err) {
			return newXMLElement("testsuites"), nil
		}
		return nil, err
	}
	root, err := parseXMLDocument(data)
	if err != nil {
		return nil, fmt.Errorf("report: reading JUnit report %s: %w", path, err)
	}
	return root, nil
}

// buildJUnitTestsuite renders one <testsuite> from results.
func buildJUnitTestsuite(results []*engine.UnitResult, redact func(string) string) *xmlNode {
	suite := newXMLElement("testsuite")
	var errors, failures int
	for _, res := range results {
		tc, errCount, failCount := buildJUnitTestcase(res, redact)
		errors += errCount
		failures += failCount
		suite.addChild(tc)
	}
	suite.Attrs = []xmlAttr{
		{Name: "tests", Value: strconv.Itoa(len(results))},
		{Name: "errors", Value: strconv.Itoa(errors)},
		{Name: "failures", Value: strconv.Itoa(failures)},
	}
	return suite
}

// buildJUnitTestcase renders one <testcase> from res's outcome: id and
// name are both res.File (its path as given on the command line, the
// upstream identifier), time is its duration in seconds with millisecond
// precision.
func buildJUnitTestcase(res *engine.UnitResult, redact func(string) string) (tc *xmlNode, errCount, failCount int) {
	tc = newXMLElement("testcase",
		xmlAttr{Name: "id", Value: res.File},
		xmlAttr{Name: "name", Value: res.File},
		xmlAttr{Name: "time", Value: fmt.Sprintf("%.3f", res.Duration.Seconds())},
	)

	// A file that never parsed has no entries for renderErrors to walk:
	// without this, it would render as an empty, passing <testcase/>.
	if res.ParseError != nil {
		tc.addChild(newXMLElement("error").addText(redact(res.ParseError.Render(res.File, res.Source))))
		return tc, 1, 0
	}

	failures, errs := renderErrors(res, redact)
	// A unit stopped before its last entry (Ctrl-C, a sibling's fatal
	// error) can likewise have no error of its own — its last attempt
	// simply never ran — and would otherwise render as a pass too.
	if len(failures) == 0 && len(errs) == 0 && res.Interrupted {
		errs = append(errs, "interrupted: the run was stopped before this file's last entry")
	}
	for _, m := range failures {
		tc.addChild(newXMLElement("failure").addText(m))
	}
	for _, m := range errs {
		tc.addChild(newXMLElement("error").addText(m))
	}
	return tc, len(errs), len(failures)
}
