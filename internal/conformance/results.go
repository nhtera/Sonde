// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// ScriptResult is one script's outcome, serialized into results.json.
type ScriptResult struct {
	Path         string        `json:"path"`
	Lane         Lane          `json:"lane"`
	Skipped      bool          `json:"skipped"`
	SkipReason   string        `json:"skip_reason,omitempty"`
	ExpectedExit int           `json:"expected_exit"`
	ActualExit   int           `json:"actual_exit"`
	SemanticPass bool          `json:"semantic_pass"`
	FullPass     bool          `json:"full_pass"`
	MismatchInfo string        `json:"mismatch,omitempty"`
	Duration     time.Duration `json:"duration_ns"`
}

// laneSummary aggregates one lane's ScriptResults.
type laneSummary struct {
	Lane         Lane
	Total        int
	Ran          int
	Skipped      int
	SemanticPass int
	FullPass     int
}

func (s laneSummary) semanticPct() float64 {
	if s.Ran == 0 {
		return 0
	}
	return 100 * float64(s.SemanticPass) / float64(s.Ran)
}

func (s laneSummary) fullPct() float64 {
	if s.Ran == 0 {
		return 0
	}
	return 100 * float64(s.FullPass) / float64(s.Ran)
}

// summarize buckets results by lane in a stable, human-friendly order.
func summarize(results []ScriptResult) []laneSummary {
	byLane := map[Lane]*laneSummary{}
	order := []Lane{LaneBlocking, LaneExtended, LaneNetwork, LaneTiming, LaneUnsupported}
	for _, l := range order {
		byLane[l] = &laneSummary{Lane: l}
	}
	for _, r := range results {
		s, ok := byLane[r.Lane]
		if !ok {
			s = &laneSummary{Lane: r.Lane}
			byLane[r.Lane] = s
			order = append(order, r.Lane)
		}
		s.Total++
		if r.Skipped {
			s.Skipped++
			continue
		}
		s.Ran++
		if r.SemanticPass {
			s.SemanticPass++
		}
		if r.FullPass {
			s.FullPass++
		}
	}
	out := make([]laneSummary, 0, len(order))
	for _, l := range order {
		out = append(out, *byLane[l])
	}
	return out
}

// writeResultsJSON writes the full per-script results as JSON to path.
func writeResultsJSON(path string, results []ScriptResult) error {
	sorted := make([]ScriptResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	f, err := os.Create(path) //nolint:gosec // G304: path is operator-controlled (SONDE_CONFORMANCE_RESULTS or $TMPDIR).
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // best-effort close on the write path; encoding errors surface first.

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(sorted)
}

// printSummaryTable writes a lane-by-lane pass-rate table to w.
func printSummaryTable(w io.Writer, results []ScriptResult) {
	fmt.Fprintf(w, "%-12s %6s %6s %8s %10s %10s\n", "lane", "total", "ran", "skipped", "semantic%", "full%")
	for _, s := range summarize(results) {
		if s.Total == 0 {
			continue
		}
		fmt.Fprintf(w, "%-12s %6d %6d %8d %9.1f%% %9.1f%%\n",
			s.Lane, s.Total, s.Ran, s.Skipped, s.semanticPct(), s.fullPct())
	}
}
