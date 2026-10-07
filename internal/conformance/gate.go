// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// gateReport is the outcome of comparing one conformance run against the
// manifest's recorded expectations.
type gateReport struct {
	// Regressions lists gating-lane scripts (blocking, hurlfmt, pty) with
	// expect: pass that did not semantically pass this run. Any entry here
	// fails TestConformance.
	Regressions []string
	// NewlyPassing lists scripts (any lane) not marked expect: pass that
	// semantically passed this run. Reported, never fails the build; run
	// `make conformance-update` to promote them.
	NewlyPassing []string
	// Unclassified lists gating-lane scripts the manifest has no entry for
	// at all. Reported, never fails the build.
	Unclassified []string
	// Stale lists manifest entries whose script no longer exists in the
	// discovered corpus (a rename, a removed fixture, or a typo). Any entry
	// here fails TestConformance: a stale entry can never be checked again,
	// which would otherwise let it hide a real problem indefinitely.
	Stale []string
}

// gateConformance scopes the pass/fail gate to the gating lanes (per
// docs/conformance.md: blocking, hurlfmt and pty gate; extended is
// report-only, network is skipped unless opted in, timing is quarantined)
// while still surfacing newly-passing scripts across every lane.
// wireOnly lists the scripts only sonde's own HTTP/1.x layer passes: with
// SONDE_HTTP1_WIRE=legacy (net/http for HTTP/1.x), the fallback kept for
// one minor release, they are not regressions.
var wireOnly = map[string]bool{
	"hurl/tests_ok/http_version/http_version_10.sh":         true,
	"hurl/tests_ok/http_version/http_version_10_env_var.sh": true,
	"hurl/tests_ssl/cacert_to_json.sh":                      true,
	// NTLM needs a connection of the own layer (8.0.1's scripts do not
	// probe Features first).
	"hurl/tests_ok/ntlm/ntlm.sh":        true,
	"hurl/tests_ok/ntlm/ntlm_option.sh": true,
	// next snapshot
	"hurl/tests_ok/http_version/http_version_10_config_file.sh":    true,
	"hurl/tests_ok/html_report_injection/html_report_injection.sh": true,
}

func gateConformance(scripts []Script, results []ScriptResult, manifest Manifest) gateReport {
	legacy := os.Getenv("SONDE_HTTP1_WIRE") == "legacy"
	resultByPath := make(map[string]ScriptResult, len(results))
	discovered := make(map[string]bool, len(scripts))
	for _, r := range results {
		resultByPath[r.Path] = r
	}
	for _, s := range scripts {
		discovered[s.Path] = true
	}

	var report gateReport
	for path := range manifest {
		if !discovered[path] {
			report.Stale = append(report.Stale, path)
		}
	}

	for _, s := range scripts {
		r, ran := resultByPath[s.Path]
		if !ran {
			continue
		}

		entry, known := manifest[s.Path]
		if !known {
			if gatingLanes[s.Lane] {
				report.Unclassified = append(report.Unclassified, s.Path)
			}
			// Absence from the manifest is treated like expect: fail for
			// the "newly passing" check below, so an unclassified script
			// that already passes still gets surfaced for promotion.
			entry = ManifestEntry{Expect: ExpectFail}
		}

		// A gating script recorded as expect: pass that stops passing —
		// including one that now merely gets skipped (e.g. it starts
		// exiting 255, the reference runner's own "unmet prerequisite"
		// signal) — is a regression: the manifest promised a clean pass,
		// and "it didn't even run" is not a lesser claim than "it failed".
		if gatingLanes[s.Lane] && entry.Expect == ExpectPass && !r.SemanticPass && (!legacy || !wireOnly[s.Path]) {
			report.Regressions = append(report.Regressions, fmt.Sprintf("%s: %s", s.Path, semanticOutcome(r)))
		}
		if entry.Expect != ExpectPass && !r.Skipped && r.SemanticPass {
			report.NewlyPassing = append(report.NewlyPassing, s.Path)
		}
	}

	sort.Strings(report.Regressions)
	sort.Strings(report.NewlyPassing)
	sort.Strings(report.Unclassified)
	sort.Strings(report.Stale)
	return report
}

// semanticOutcome describes why a result did not semantically pass, for
// regression messages.
func semanticOutcome(r ScriptResult) string {
	if r.Skipped {
		return fmt.Sprintf("expected pass, got skipped (%s)", r.SkipReason)
	}
	return fmt.Sprintf("expected pass, got exit %d (want %d)%s",
		r.ActualExit, r.ExpectedExit, mismatchSuffix(r.MismatchInfo))
}

func mismatchSuffix(info string) string {
	if info == "" {
		return ""
	}
	return " — " + info
}

// printGateReport writes the regressions/newly-passing/unclassified/stale
// lists a gated run found, in that order, each grouped under a header so CI
// logs are scannable without reading results.json.
func printGateReport(w io.Writer, report gateReport) {
	if len(report.Regressions) > 0 {
		fmt.Fprintln(w, "conformance: GATING regressions (manifest says expect: pass, run disagrees):")
		for _, r := range report.Regressions {
			fmt.Fprintln(w, "  -", r)
		}
	}
	if len(report.Stale) > 0 {
		fmt.Fprintln(w, "conformance: BLOCKING manifest entries whose script no longer exists (fix or remove them):")
		for _, p := range report.Stale {
			fmt.Fprintln(w, "  -", p)
		}
	}
	if len(report.NewlyPassing) > 0 {
		fmt.Fprintln(w, "conformance: newly passing, not yet marked pass in the manifest (run `make conformance-update` to promote):")
		for _, p := range report.NewlyPassing {
			fmt.Fprintln(w, "  -", p)
		}
	}
	if len(report.Unclassified) > 0 {
		fmt.Fprintln(w, "conformance: gating-lane scripts with no manifest entry at all:")
		for _, p := range report.Unclassified {
			fmt.Fprintln(w, "  -", p)
		}
	}
}

// laneRates is the pair of headline percentages printed and compared
// against the manifest for one lane.
type laneRates struct {
	Lane             Lane
	SemanticPct      float64
	FullPct          float64
	ManifestBaseline float64 // % of the lane's manifest entries marked expect: pass
	Delta            float64 // SemanticPct - ManifestBaseline
}

// manifestBaselineRate is the semantic pass rate the manifest implies for a
// lane: the fraction of that lane's manifest entries marked expect: pass.
// Comparing it against the measured rate turns "did anything regress" into
// a single printed number even before the gate's Regressions list is read.
func manifestBaselineRate(scripts []Script, manifest Manifest, lane Lane) float64 {
	var total, pass int
	for _, s := range scripts {
		if s.Lane != lane {
			continue
		}
		e, ok := manifest[s.Path]
		if !ok {
			continue
		}
		total++
		if e.Expect == ExpectPass {
			pass++
		}
	}
	if total == 0 {
		return 0
	}
	return 100 * float64(pass) / float64(total)
}

// computeLaneRates joins this run's measured summary with the manifest
// baseline for every lane that has results.
func computeLaneRates(scripts []Script, results []ScriptResult, manifest Manifest) []laneRates {
	var out []laneRates
	for _, s := range summarize(results) {
		if s.Total == 0 {
			continue
		}
		baseline := manifestBaselineRate(scripts, manifest, s.Lane)
		out = append(out, laneRates{
			Lane:             s.Lane,
			SemanticPct:      s.semanticPct(),
			FullPct:          s.fullPct(),
			ManifestBaseline: baseline,
			Delta:            s.semanticPct() - baseline,
		})
	}
	return out
}

// printLaneRates prints semantic/full-oracle rates per lane alongside the
// manifest's baseline and the delta between them.
func printLaneRates(w io.Writer, rates []laneRates) {
	fmt.Fprintf(w, "%-12s %10s %10s %18s %10s\n", "lane", "semantic%", "full%", "manifest-pass%", "delta(pp)")
	for _, r := range rates {
		fmt.Fprintf(w, "%-12s %9.1f%% %9.1f%% %17.1f%% %+9.1f\n",
			r.Lane, r.SemanticPct, r.FullPct, r.ManifestBaseline, r.Delta)
	}
}

// updateReport summarizes what `make conformance-update` changed.
type updateReport struct {
	Promoted           []string // moved from fail/skip/unclassified to pass
	Demoted            []string // moved from pass to fail/skip (only where the demotePolicy allows it)
	KeptDespiteFailure []string // still expect: pass in the manifest but failed this run (demotion not allowed)
}

// demotePolicy says which expect: pass entries a manifest update may
// demote. Only names paths (CONFORMANCE_DEMOTE_ONLY) and must come with a
// Reason (CONFORMANCE_DEMOTE_REASON), which is recorded on each demoted
// entry. All (CONFORMANCE_ALLOW_DEMOTE) is the emergency blanket switch.
type demotePolicy struct {
	All    bool
	Only   map[string]bool
	Reason string
}

func (p demotePolicy) allows(path string) bool {
	return p.All || p.Only[path]
}

// parseDemotePolicy builds the policy from the CONFORMANCE_DEMOTE_ONLY
// value (comma-separated manifest keys), its required reason, and the
// blanket switch. Every named path must be a discovered script, so a typo
// cannot quietly demote nothing.
func parseDemotePolicy(only, reason string, all bool, scripts []Script) (demotePolicy, error) {
	p := demotePolicy{All: all, Reason: strings.TrimSpace(reason)}
	if strings.TrimSpace(only) == "" {
		return p, nil
	}
	if p.Reason == "" {
		return p, fmt.Errorf("CONFORMANCE_DEMOTE_ONLY needs CONFORMANCE_DEMOTE_REASON")
	}
	known := make(map[string]bool, len(scripts))
	for _, s := range scripts {
		known[s.Path] = true
	}
	p.Only = map[string]bool{}
	var unknown []string
	for _, path := range strings.Split(only, ",") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !known[path] {
			unknown = append(unknown, path)
		}
		p.Only[path] = true
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return p, fmt.Errorf("CONFORMANCE_DEMOTE_ONLY names unknown scripts: %s", strings.Join(unknown, ", "))
	}
	return p, nil
}

func (r updateReport) empty() bool {
	return len(r.Promoted) == 0 && len(r.Demoted) == 0 && len(r.KeptDespiteFailure) == 0
}

// updateManifestReasonFallback is recorded for a script the updater sees
// for the first time and cannot classify as pass: it still needs a human
// to triage and replace this placeholder with a real reason.
const updateManifestReasonFallback = "conformance-update: newly discovered, not yet triaged"

// updateManifest computes the manifest `make conformance-update` should
// write from a fresh run:
//   - a script that now semantically passes is promoted to expect: pass;
//   - a script that is skipped is recorded as expect: skip, keeping any
//     existing hand-written reason over the run's generic skip reason;
//   - a script that fails keeps its existing reason if it had one expect:
//     fail entry already, otherwise gets a placeholder reason to triage;
//   - a script that regresses from expect: pass to either failing or being
//     skipped keeps its pass expectation (and is reported in
//     KeptDespiteFailure) unless the demote policy allows that path, so a
//     regression is never silently absorbed into the manifest as fail or
//     skip. A targeted demotion records the policy's reason.
func updateManifest(existing Manifest, scripts []Script, results []ScriptResult, demote demotePolicy) (Manifest, updateReport) {
	resultByPath := make(map[string]ScriptResult, len(results))
	for _, r := range results {
		resultByPath[r.Path] = r
	}

	updated := make(Manifest, len(scripts))
	var report updateReport
	for _, s := range scripts {
		r, ran := resultByPath[s.Path]
		prev, known := existing[s.Path]
		if !ran {
			if known {
				updated[s.Path] = prev
			}
			continue
		}

		wasPass := known && prev.Expect == ExpectPass
		allowDemote := demote.allows(s.Path)
		demoteReason := ""
		if wasPass && allowDemote && demote.Only[s.Path] {
			demoteReason = demote.Reason
		}

		switch {
		case r.SemanticPass:
			if !known || prev.Expect != ExpectPass {
				report.Promoted = append(report.Promoted, s.Path)
			}
			updated[s.Path] = ManifestEntry{Lane: s.Lane, Expect: ExpectPass}

		case r.Skipped:
			// A pass -> skip move is a demotion exactly like pass -> fail:
			// the manifest promised a clean pass, and "it didn't even run"
			// must not be absorbed silently either.
			if wasPass && !allowDemote {
				updated[s.Path] = prev
				report.KeptDespiteFailure = append(report.KeptDespiteFailure, s.Path)
				continue
			}
			if wasPass {
				report.Demoted = append(report.Demoted, s.Path)
			}
			reason := r.SkipReason
			if known && prev.Expect == ExpectSkip && prev.Reason != "" {
				reason = prev.Reason
			}
			if demoteReason != "" {
				reason = demoteReason
			}
			updated[s.Path] = ManifestEntry{Lane: s.Lane, Expect: ExpectSkip, Reason: reason}

		default: // failed semantically
			if wasPass && !allowDemote {
				updated[s.Path] = prev
				report.KeptDespiteFailure = append(report.KeptDespiteFailure, s.Path)
				continue
			}
			if wasPass {
				report.Demoted = append(report.Demoted, s.Path)
			}
			reason := updateManifestReasonFallback
			if known && prev.Reason != "" {
				reason = prev.Reason
			}
			if demoteReason != "" {
				reason = demoteReason
			}
			updated[s.Path] = ManifestEntry{Lane: s.Lane, Expect: ExpectFail, Reason: reason}
		}
	}

	sort.Strings(report.Promoted)
	sort.Strings(report.Demoted)
	sort.Strings(report.KeptDespiteFailure)
	return updated, report
}

// printUpdateReport writes what `make conformance-update` changed.
func printUpdateReport(w io.Writer, report updateReport) {
	if report.empty() {
		fmt.Fprintln(w, "conformance-update: no changes (manifest already matches this run)")
		return
	}
	if len(report.Promoted) > 0 {
		fmt.Fprintln(w, "conformance-update: promoted to expect: pass:")
		for _, p := range report.Promoted {
			fmt.Fprintln(w, "  -", p)
		}
	}
	if len(report.Demoted) > 0 {
		fmt.Fprintln(w, "conformance-update: demoted from expect: pass (allowed by CONFORMANCE_DEMOTE_ONLY or CONFORMANCE_ALLOW_DEMOTE):")
		for _, p := range report.Demoted {
			fmt.Fprintln(w, "  -", p)
		}
	}
	if len(report.KeptDespiteFailure) > 0 {
		fmt.Fprintln(w, "conformance-update: kept expect: pass despite a failing run (name it in CONFORMANCE_DEMOTE_ONLY with a CONFORMANCE_DEMOTE_REASON to demote):")
		for _, p := range report.KeptDespiteFailure {
			fmt.Fprintln(w, "  -", p)
		}
	}
}
