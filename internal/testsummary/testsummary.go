// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package testsummary classifies run results and renders the per-file
// status lines and the final block of `sonde run --test`.
package testsummary

import (
	"fmt"
	"time"

	"github.com/nhtera/sonde/engine"
)

// Outcome is how a run ended, in increasing severity.
type Outcome int

// Outcomes.
const (
	OK      Outcome = iota // success
	Assert                 // every decisive error is an assert failure
	Runtime                // at least one decisive error is not an assert failure
	Parse                  // the file does not parse
)

// Classify returns the outcome of a finished run.
func Classify(res *engine.UnitResult) Outcome {
	if res.ParseError != nil {
		return Parse
	}
	if res.Success {
		return OK
	}
	for _, e := range res.Errors() {
		if !e.Assert() {
			return Runtime
		}
	}
	return Assert
}

// Worst returns the more severe of two outcomes, a when they are equal.
func Worst(a, b Outcome) Outcome {
	if b > a {
		return b
	}
	return a
}

// ANSI styles of Line.
const (
	ansiReset     = "\x1b[0m"
	ansiBold      = "\x1b[1m"
	ansiGreenBold = "\x1b[1;32m"
	ansiRedBold   = "\x1b[1;31m"
)

// Line is the one-line status of a file in --test mode, with its line
// terminator, colored like the reference CLI's own
// ParProgress::print_completed when color is on: a bold green "Success" or
// bold red "Failure", the filename bold.
func Line(res *engine.UnitResult, color bool) string {
	status, style := "Success", ansiGreenBold
	if !res.Success {
		status, style = "Failure", ansiRedBold
	}
	n := 0
	for _, e := range res.Entries {
		n += len(e.Calls)
	}
	if !color {
		return fmt.Sprintf("%s %s (%d request(s) in %d ms)\n", status, res.Label(), n, res.Duration.Milliseconds())
	}
	return fmt.Sprintf("%s%s%s %s%s%s (%d request(s) in %d ms)\n",
		style, status, ansiReset, ansiBold, res.Label(), ansiReset, n, res.Duration.Milliseconds())
}

// Summary is --test's final block: the documented wording and number
// formatting, reproduced exactly.
func Summary(totalFiles, succeededFiles, totalRequests int, duration time.Duration) string {
	failed := totalFiles - succeededFiles
	var successPct, failedPct float64
	if totalFiles > 0 {
		successPct = 100 * float64(succeededFiles) / float64(totalFiles)
		failedPct = 100 * float64(failed) / float64(totalFiles)
	}
	ms := duration.Milliseconds()
	var rate float64
	if ms > 0 {
		rate = 1000 * float64(totalRequests) / float64(ms)
	}
	return fmt.Sprintf(
		"--------------------------------------------------------------------------------\n"+
			"Executed files:    %d\n"+
			"Executed requests: %d (%.1f/s)\n"+
			"Succeeded files:   %d (%.1f%%)\n"+
			"Failed files:      %d (%.1f%%)\n"+
			"Duration:          %d ms (%s)\n\n",
		totalFiles, totalRequests, rate, succeededFiles, successPct, failed, failedPct, ms, formatDurationHMS(duration))
}

func formatDurationHMS(d time.Duration) string {
	total := d.Milliseconds()
	hours := total / 3600000
	minutes := (total % 3600000) / 60000
	seconds := (total % 60000) / 1000
	millis := total % 1000
	return fmt.Sprintf("%dh:%dm:%ds:%dms", hours, minutes, seconds, millis)
}
