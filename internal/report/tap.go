// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nhtera/sonde/engine"
)

// tapVersionMarker is the TAP 13 header line; see
// https://testanything.org/tap-version-13-specification.html.
const tapVersionMarker = "TAP version 13"

var tapPlanLine = regexp.MustCompile(`^1\.\.\d+`)

// tapCase is one line of a TAP report.
type tapCase struct {
	description string
	success     bool
}

// WriteTAP appends one line per result, in order, to the TAP report at
// path, renumbering the whole file: repeated calls with the same path
// read every line already there, then rewrite the file with the old
// lines followed by the new ones, all under a single "1..N" plan —
// matching the reference CLI's cumulative behavior. redact masks secret
// values in the file name each line reports. The file is written
// atomically (temp file + rename), so a run killed mid-write never
// leaves a truncated report for the next invocation to choke on.
func WriteTAP(path string, results []*engine.UnitResult, redact func(string) string) error {
	existing, err := readTAPCases(path)
	if err != nil {
		return err
	}
	cases := existing
	for _, res := range results {
		cases = append(cases, tapCase{
			// A result's own Success already covers a parse error or an
			// interrupted unit (both leave it false), so neither needs
			// special-casing here the way JUnit's empty-testcase shape
			// does.
			description: escapeTAPDescription(forResult(res, redact)(res.Label())),
			success:     res.Success,
		})
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s\n", tapVersionMarker)
	fmt.Fprintf(&buf, "1..%d\n", len(cases))
	for i, c := range cases {
		state := "ok"
		if !c.success {
			state = "not ok"
		}
		fmt.Fprintf(&buf, "%s %d - %s\n", state, i+1, c.description)
	}
	return atomicWriteFile(path, buf.Bytes(), 0o644) //nolint:gosec // G306: report files are not secret
}

// escapeTAPDescription makes s safe to use as a TAP line's description,
// per the TAP 13 spec (https://testanything.org/tap-version-13-specification.html):
// a bare '#' would otherwise start a directive (e.g. "# SKIP", "# TODO"),
// silently changing how a consumer interprets the line, so it is escaped
// as "\#"; a line break would otherwise split one test into two lines, so
// it is replaced with a space.
func escapeTAPDescription(s string) string {
	s = strings.ReplaceAll(s, "#", `\#`)
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

// readTAPCases parses the TAP report already at path, or returns nil when
// it does not exist yet. Error text (a "report cannot be written" runtime
// error per architecture.md's exit code table) matches the reference
// CLI's own reporting exactly, since it is the one report-format error a
// conformance oracle asserts on (tests_failed/parse_error_tap: a
// pre-existing report file that is not a TAP file at all).
func readTAPCases(path string) ([]tapCase, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path is a CLI-trusted --report-tap flag
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("Issue reading TAP report %s (%s)", path, err) //nolint:staticcheck // ST1005: matches the reference CLI's message verbatim
	}
	defer f.Close() //nolint:errcheck // read-only handle

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("Issue reading TAP report %s (%s)", path, err) //nolint:staticcheck // ST1005: matches the reference CLI's message verbatim
	}
	if len(lines) == 0 {
		return nil, nil
	}

	header, rest := lines[0], lines[1:]
	if strings.EqualFold(header, tapVersionMarker) {
		if len(rest) == 0 {
			return nil, fmt.Errorf("Invalid TAP Header <>") //nolint:staticcheck // ST1005: matches the reference CLI's message verbatim
		}
		header, rest = rest[0], rest[1:]
	}
	if !tapPlanLine.MatchString(header) {
		return nil, fmt.Errorf("Invalid TAP Header <%s>", header) //nolint:staticcheck // ST1005: matches the reference CLI's message verbatim
	}

	var cases []tapCase
	for _, line := range rest {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		c, err := parseTAPCase(line)
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, nil
}

// parseTAPCase parses one "ok N - description" / "not ok N - description"
// line. Error text matches the reference CLI's own TAP-line parser.
func parseTAPCase(line string) (tapCase, error) {
	success := true
	rest, ok := strings.CutPrefix(line, "not ok")
	if ok {
		success = false
	} else {
		rest, ok = strings.CutPrefix(line, "ok")
	}
	if !ok {
		return tapCase{}, fmt.Errorf("Invalid TAP line <%s> - must start with ok or nok", line) //nolint:staticcheck // ST1005: matches the reference CLI's message verbatim
	}
	idx := strings.Index(rest, "-")
	if idx < 0 {
		return tapCase{}, fmt.Errorf("Invalid TAP line <%s> - missing '-' separator", line) //nolint:staticcheck // ST1005: matches the reference CLI's message verbatim
	}
	description := strings.TrimSpace(rest[idx+1:])
	return tapCase{description: description, success: success}, nil
}
