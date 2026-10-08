// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"testing"
)

func TestGateConformanceFlagsBlockingRegression(t *testing.T) {
	scripts := []Script{{Path: "tests_ok/hello/hello.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "tests_ok/hello/hello.sh", Lane: LaneBlocking, SemanticPass: false, ActualExit: 1, ExpectedExit: 0}}
	manifest := Manifest{"tests_ok/hello/hello.sh": {Lane: LaneBlocking, Expect: ExpectPass}}

	got := gateConformance(scripts, results, manifest)
	if len(got.Regressions) != 1 {
		t.Fatalf("Regressions = %v, want exactly one", got.Regressions)
	}
}

func TestGateConformanceIgnoresNonBlockingLaneFailure(t *testing.T) {
	scripts := []Script{{Path: "tests_ssl/cacert.sh", Lane: LaneExtended}}
	results := []ScriptResult{{Path: "tests_ssl/cacert.sh", Lane: LaneExtended, SemanticPass: false}}
	manifest := Manifest{"tests_ssl/cacert.sh": {Lane: LaneExtended, Expect: ExpectPass}}

	got := gateConformance(scripts, results, manifest)
	if len(got.Regressions) != 0 {
		t.Errorf("Regressions = %v, want none (extended lane is not blocking)", got.Regressions)
	}
}

// TestGateConformanceSkippedExpectPassIsRegression covers review finding
// #10(a): a blocking script the manifest marks expect: pass that starts
// exiting 255 (or is otherwise skipped) must be treated as a regression,
// not quietly excused — "it didn't even run" is not a lesser claim than
// "it failed".
func TestGateConformanceSkippedExpectPassIsRegression(t *testing.T) {
	scripts := []Script{{Path: "tests_ok/x/x.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "tests_ok/x/x.sh", Lane: LaneBlocking, Skipped: true, SkipReason: "script exited 255"}}
	manifest := Manifest{"tests_ok/x/x.sh": {Lane: LaneBlocking, Expect: ExpectPass}}

	got := gateConformance(scripts, results, manifest)
	if len(got.Regressions) != 1 || got.Regressions[0] != "tests_ok/x/x.sh: expected pass, got skipped (script exited 255)" {
		t.Errorf("Regressions = %v, want exactly one regression naming the skip reason", got.Regressions)
	}
}

// TestGateConformanceSkippedNonPassScriptNotRegression is the counterpart:
// a script the manifest never claimed passes (expect: skip or expect:
// fail) being skipped is expected behavior, not a regression.
func TestGateConformanceSkippedNonPassScriptNotRegression(t *testing.T) {
	scripts := []Script{{Path: "tests_ok/x/x.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "tests_ok/x/x.sh", Lane: LaneBlocking, Skipped: true, SkipReason: "exit 255"}}
	manifest := Manifest{"tests_ok/x/x.sh": {Lane: LaneBlocking, Expect: ExpectSkip, Reason: "known unmet prerequisite"}}

	got := gateConformance(scripts, results, manifest)
	if len(got.Regressions) != 0 {
		t.Errorf("Regressions = %v, want none (manifest already expects a skip)", got.Regressions)
	}
}

func TestGateConformanceReportsNewlyPassing(t *testing.T) {
	scripts := []Script{{Path: "tests_ok/y/y.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "tests_ok/y/y.sh", Lane: LaneBlocking, SemanticPass: true}}
	manifest := Manifest{"tests_ok/y/y.sh": {Lane: LaneBlocking, Expect: ExpectFail, Reason: "reports: Phase 5 in progress"}}

	got := gateConformance(scripts, results, manifest)
	if len(got.Regressions) != 0 {
		t.Errorf("Regressions = %v, want none", got.Regressions)
	}
	if len(got.NewlyPassing) != 1 || got.NewlyPassing[0] != "tests_ok/y/y.sh" {
		t.Errorf("NewlyPassing = %v, want [tests_ok/y/y.sh]", got.NewlyPassing)
	}
}

func TestGateConformanceReportsUnclassifiedBlockingScript(t *testing.T) {
	scripts := []Script{{Path: "tests_ok/new/new.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "tests_ok/new/new.sh", Lane: LaneBlocking, SemanticPass: false}}
	manifest := Manifest{} // no entry at all

	got := gateConformance(scripts, results, manifest)
	if len(got.Regressions) != 0 {
		t.Errorf("Regressions = %v, want none (no expect: pass was ever recorded)", got.Regressions)
	}
	if len(got.Unclassified) != 1 || got.Unclassified[0] != "tests_ok/new/new.sh" {
		t.Errorf("Unclassified = %v, want [tests_ok/new/new.sh]", got.Unclassified)
	}
}

func TestGateConformanceUnclassifiedNonBlockingScriptNotReported(t *testing.T) {
	scripts := []Script{{Path: "tests_ssl/new.sh", Lane: LaneExtended}}
	results := []ScriptResult{{Path: "tests_ssl/new.sh", Lane: LaneExtended, SemanticPass: false}}
	manifest := Manifest{}

	got := gateConformance(scripts, results, manifest)
	if len(got.Unclassified) != 0 {
		t.Errorf("Unclassified = %v, want none (only blocking-lane gaps are reported)", got.Unclassified)
	}
}

// TestGateConformanceFlagsStaleManifestEntry covers review finding #10(c):
// a manifest entry whose script the current corpus no longer discovers at
// all (renamed, removed, or a typo) must be flagged, not silently ignored
// forever.
func TestGateConformanceFlagsStaleManifestEntry(t *testing.T) {
	scripts := []Script{{Path: "tests_ok/hello/hello.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "tests_ok/hello/hello.sh", Lane: LaneBlocking, SemanticPass: true}}
	manifest := Manifest{
		"tests_ok/hello/hello.sh":    {Lane: LaneBlocking, Expect: ExpectPass},
		"tests_ok/renamed/gone.sh":   {Lane: LaneBlocking, Expect: ExpectPass},
		"tests_ok/removed/vanish.sh": {Lane: LaneBlocking, Expect: ExpectFail, Reason: "wip"},
	}

	got := gateConformance(scripts, results, manifest)
	if len(got.Stale) != 2 {
		t.Fatalf("Stale = %v, want exactly the two entries with no matching script", got.Stale)
	}
	want := map[string]bool{"tests_ok/renamed/gone.sh": true, "tests_ok/removed/vanish.sh": true}
	for _, p := range got.Stale {
		if !want[p] {
			t.Errorf("Stale contains unexpected path %s", p)
		}
	}
	if len(got.Regressions) != 0 {
		t.Errorf("Regressions = %v, want none (the only expect: pass script that still exists passed)", got.Regressions)
	}
}

// TestGateConformanceNoStaleEntriesWhenManifestMatchesCorpus is the
// negative case: every manifest entry has a matching discovered script, so
// Stale must be empty.
func TestGateConformanceNoStaleEntriesWhenManifestMatchesCorpus(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneBlocking, SemanticPass: true}}
	manifest := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectPass}}

	got := gateConformance(scripts, results, manifest)
	if len(got.Stale) != 0 {
		t.Errorf("Stale = %v, want none", got.Stale)
	}
}

func TestManifestBaselineRate(t *testing.T) {
	scripts := []Script{
		{Path: "a.sh", Lane: LaneBlocking},
		{Path: "b.sh", Lane: LaneBlocking},
		{Path: "c.sh", Lane: LaneExtended},
	}
	manifest := Manifest{
		"a.sh": {Lane: LaneBlocking, Expect: ExpectPass},
		"b.sh": {Lane: LaneBlocking, Expect: ExpectFail, Reason: "wip"},
		"c.sh": {Lane: LaneExtended, Expect: ExpectPass},
	}

	if got := manifestBaselineRate(scripts, manifest, LaneBlocking); got != 50 {
		t.Errorf("manifestBaselineRate(blocking) = %v, want 50", got)
	}
	if got := manifestBaselineRate(scripts, manifest, LaneExtended); got != 100 {
		t.Errorf("manifestBaselineRate(extended) = %v, want 100", got)
	}
}

func TestUpdateManifestPromotesNewlyPassing(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneBlocking, SemanticPass: true}}
	existing := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectFail, Reason: "wip"}}

	updated, report := updateManifest(existing, scripts, results, demotePolicy{})
	if updated["a.sh"].Expect != ExpectPass {
		t.Errorf("updated[a.sh].Expect = %v, want pass", updated["a.sh"].Expect)
	}
	if len(report.Promoted) != 1 || report.Promoted[0] != "a.sh" {
		t.Errorf("Promoted = %v, want [a.sh]", report.Promoted)
	}
}

func TestUpdateManifestKeepsReasonForStillFailing(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneBlocking, SemanticPass: false}}
	existing := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectFail, Reason: "reports: Phase 5 in progress"}}

	updated, report := updateManifest(existing, scripts, results, demotePolicy{})
	if updated["a.sh"].Reason != "reports: Phase 5 in progress" {
		t.Errorf("updated[a.sh].Reason = %q, want the original reason kept", updated["a.sh"].Reason)
	}
	if !report.empty() {
		t.Errorf("report = %+v, want empty (no promotion, demotion or kept-despite-failure)", report)
	}
}

func TestUpdateManifestNeverDemotesSilently(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneBlocking, SemanticPass: false}}
	existing := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectPass}}

	updated, report := updateManifest(existing, scripts, results, demotePolicy{})
	if updated["a.sh"].Expect != ExpectPass {
		t.Errorf("updated[a.sh].Expect = %v, want pass kept (allowDemote=false)", updated["a.sh"].Expect)
	}
	if len(report.KeptDespiteFailure) != 1 || report.KeptDespiteFailure[0] != "a.sh" {
		t.Errorf("KeptDespiteFailure = %v, want [a.sh]", report.KeptDespiteFailure)
	}
	if len(report.Demoted) != 0 {
		t.Errorf("Demoted = %v, want none", report.Demoted)
	}
}

func TestUpdateManifestDemotesWhenAllowed(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneBlocking, SemanticPass: false}}
	existing := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectPass}}

	updated, report := updateManifest(existing, scripts, results, demotePolicy{All: true})
	if updated["a.sh"].Expect != ExpectFail {
		t.Errorf("updated[a.sh].Expect = %v, want fail (allowDemote=true)", updated["a.sh"].Expect)
	}
	if updated["a.sh"].Reason == "" {
		t.Error("updated[a.sh].Reason is empty; a demoted fail entry must still carry a reason")
	}
	if len(report.Demoted) != 1 || report.Demoted[0] != "a.sh" {
		t.Errorf("Demoted = %v, want [a.sh]", report.Demoted)
	}
}

// TestUpdateManifestNeverDemotesPassToSkipSilently covers review finding
// #10(b): a pass -> skip move is a demotion exactly like pass -> fail, and
// must be guarded by allowDemote the same way.
func TestUpdateManifestNeverDemotesPassToSkipSilently(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneBlocking, Skipped: true, SkipReason: "script exited 255"}}
	existing := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectPass}}

	updated, report := updateManifest(existing, scripts, results, demotePolicy{})
	if updated["a.sh"].Expect != ExpectPass {
		t.Errorf("updated[a.sh].Expect = %v, want pass kept (allowDemote=false)", updated["a.sh"].Expect)
	}
	if len(report.KeptDespiteFailure) != 1 || report.KeptDespiteFailure[0] != "a.sh" {
		t.Errorf("KeptDespiteFailure = %v, want [a.sh]", report.KeptDespiteFailure)
	}
	if len(report.Demoted) != 0 {
		t.Errorf("Demoted = %v, want none", report.Demoted)
	}
}

// TestUpdateManifestDemotesPassToSkipWhenAllowed is the allowDemote=true
// counterpart of TestUpdateManifestNeverDemotesPassToSkipSilently.
func TestUpdateManifestDemotesPassToSkipWhenAllowed(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneBlocking, Skipped: true, SkipReason: "script exited 255"}}
	existing := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectPass}}

	updated, report := updateManifest(existing, scripts, results, demotePolicy{All: true})
	if updated["a.sh"].Expect != ExpectSkip {
		t.Errorf("updated[a.sh].Expect = %v, want skip (allowDemote=true)", updated["a.sh"].Expect)
	}
	if updated["a.sh"].Reason == "" {
		t.Error("updated[a.sh].Reason is empty; a demoted skip entry must still carry a reason")
	}
	if len(report.Demoted) != 1 || report.Demoted[0] != "a.sh" {
		t.Errorf("Demoted = %v, want [a.sh]", report.Demoted)
	}
}

func TestUpdateManifestRecordsSkipReason(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneNetwork}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneNetwork, Skipped: true, SkipReason: "network lane disabled"}}

	updated, _ := updateManifest(Manifest{}, scripts, results, demotePolicy{})
	if updated["a.sh"].Expect != ExpectSkip {
		t.Errorf("updated[a.sh].Expect = %v, want skip", updated["a.sh"].Expect)
	}
	if updated["a.sh"].Reason != "network lane disabled" {
		t.Errorf("updated[a.sh].Reason = %q, want the skip reason", updated["a.sh"].Reason)
	}
}

func TestUpdateManifestKeepsHandWrittenSkipReason(t *testing.T) {
	scripts := []Script{{Path: "a.sh", Lane: LaneNetwork}}
	results := []ScriptResult{{Path: "a.sh", Lane: LaneNetwork, Skipped: true, SkipReason: "network lane disabled; set SONDE_CONFORMANCE_NETWORK=1"}}
	existing := Manifest{"a.sh": {Lane: LaneNetwork, Expect: ExpectSkip, Reason: "network"}}

	updated, _ := updateManifest(existing, scripts, results, demotePolicy{})
	if updated["a.sh"].Reason != "network" {
		t.Errorf("updated[a.sh].Reason = %q, want the hand-written reason kept", updated["a.sh"].Reason)
	}
}

func TestUpdateManifestNewScriptGetsPlaceholderReason(t *testing.T) {
	scripts := []Script{{Path: "new.sh", Lane: LaneBlocking}}
	results := []ScriptResult{{Path: "new.sh", Lane: LaneBlocking, SemanticPass: false}}

	updated, _ := updateManifest(Manifest{}, scripts, results, demotePolicy{})
	if updated["new.sh"].Expect != ExpectFail || updated["new.sh"].Reason == "" {
		t.Errorf("updated[new.sh] = %+v, want expect: fail with a non-empty placeholder reason", updated["new.sh"])
	}
}

func TestUpdateManifestOutputAlwaysValidates(t *testing.T) {
	scripts := []Script{
		{Path: "a.sh", Lane: LaneBlocking},
		{Path: "b.sh", Lane: LaneBlocking},
		{Path: "c.sh", Lane: LaneNetwork},
	}
	results := []ScriptResult{
		{Path: "a.sh", Lane: LaneBlocking, SemanticPass: true},
		{Path: "b.sh", Lane: LaneBlocking, SemanticPass: false},
		{Path: "c.sh", Lane: LaneNetwork, Skipped: true, SkipReason: "network"},
	}
	updated, _ := updateManifest(Manifest{}, scripts, results, demotePolicy{})
	if err := updated.Validate(); err != nil {
		t.Errorf("updateManifest produced an invalid manifest: %v", err)
	}
}

// TestGateConformanceEveryGatingLaneFails is the "deliberately broken
// fixture" check: an expect: pass entry that stops passing fails the run in
// each gating lane, and only there.
func TestGateConformanceEveryGatingLaneFails(t *testing.T) {
	for _, tc := range []struct {
		lane Lane
		path string
		gate bool
	}{
		{LaneBlocking, "hurl/tests_ok/hello/hello.sh", true},
		{LaneHurlfmt, "hurlfmt/tests_ok/format.sh", true},
		{LaneHurlfmt, "hurlfmt/tests_export/body.hurl#json", true},
		{LanePTY, "hurl/tests_pty/color/color.sh", true},
		{LaneExtended, "hurl/tests_ssl/cacert.sh", false},
		{LaneTiming, "hurl/tests_ok/delay/delay.sh", false},
		{LaneNetwork, "hurl/tests_ssl/live.sh", false},
	} {
		scripts := []Script{{Path: tc.path, Lane: tc.lane}}
		results := []ScriptResult{{Path: tc.path, Lane: tc.lane, ActualExit: 1}}
		manifest := Manifest{tc.path: {Lane: tc.lane, Expect: ExpectPass}}

		got := gateConformance(scripts, results, manifest)
		if gated := len(got.Regressions) == 1; gated != tc.gate {
			t.Errorf("%s (%s): Regressions = %v, want gating = %v", tc.path, tc.lane, got.Regressions, tc.gate)
		}
	}
}

func TestParseDemotePolicy(t *testing.T) {
	scripts := []Script{{Path: "hurl/a.sh"}, {Path: "hurl/b.sh"}}

	if _, err := parseDemotePolicy("hurl/a.sh", "", false, scripts); err == nil {
		t.Error("DEMOTE_ONLY without a reason: want an error")
	}
	if _, err := parseDemotePolicy("hurl/a.sh,hurl/typo.sh", "why", false, scripts); err == nil {
		t.Error("DEMOTE_ONLY naming an unknown script: want an error")
	}
	p, err := parseDemotePolicy(" hurl/a.sh , ", "why", false, scripts)
	if err != nil {
		t.Fatal(err)
	}
	if !p.allows("hurl/a.sh") || p.allows("hurl/b.sh") {
		t.Errorf("policy %+v: want only hurl/a.sh allowed", p)
	}
	if p, _ := parseDemotePolicy("", "", true, scripts); !p.allows("hurl/b.sh") {
		t.Error("blanket policy: want every path allowed")
	}
}

func TestUpdateManifestTargetedDemotion(t *testing.T) {
	scripts := []Script{{Path: "hurl/a.sh", Lane: LaneBlocking}, {Path: "hurl/b.sh", Lane: LaneBlocking}}
	results := []ScriptResult{
		{Path: "hurl/a.sh", Lane: LaneBlocking, ActualExit: 1},
		{Path: "hurl/b.sh", Lane: LaneBlocking, ActualExit: 1},
	}
	existing := Manifest{
		"hurl/a.sh": {Lane: LaneBlocking, Expect: ExpectPass},
		"hurl/b.sh": {Lane: LaneBlocking, Expect: ExpectPass},
	}
	policy := demotePolicy{Only: map[string]bool{"hurl/a.sh": true}, Reason: "next-version oracle"}

	updated, report := updateManifest(existing, scripts, results, policy)
	if e := updated["hurl/a.sh"]; e.Expect != ExpectFail || e.Reason != "next-version oracle" {
		t.Errorf("hurl/a.sh = %+v, want demoted with the policy reason", e)
	}
	if e := updated["hurl/b.sh"]; e.Expect != ExpectPass {
		t.Errorf("hurl/b.sh = %+v, want kept at pass (not named)", e)
	}
	if len(report.Demoted) != 1 || len(report.KeptDespiteFailure) != 1 {
		t.Errorf("report = %+v, want one demoted and one kept", report)
	}
}

// TestGateConformanceLegacyWire checks that a script only the own wire
// layer passes is no regression with SONDE_HTTP1_WIRE=legacy, and still
// is one otherwise.
func TestGateConformanceLegacyWire(t *testing.T) {
	path := "hurl/tests_ok/http_version/http_version_10.sh"
	scripts := []Script{{Path: path, Lane: LaneBlocking}}
	results := []ScriptResult{{Path: path, Lane: LaneBlocking, SemanticPass: false}}
	manifest := Manifest{path: {Lane: LaneBlocking, Expect: ExpectPass}}
	t.Setenv("SONDE_HTTP1_WIRE", "")
	if r := gateConformance(scripts, results, manifest); len(r.Regressions) != 1 {
		t.Errorf("default: regressions %v", r.Regressions)
	}
	t.Setenv("SONDE_HTTP1_WIRE", "legacy")
	if r := gateConformance(scripts, results, manifest); len(r.Regressions) != 0 {
		t.Errorf("legacy: regressions %v", r.Regressions)
	}
}
