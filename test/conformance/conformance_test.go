// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package conformance runs the reference implementation's own integration
// test scripts against a binary under test through a `hurl` -> sonde shim,
// to measure how close that binary is to the reference implementation's
// behavior.
//
// It is opt-in: plain `go test ./...` only runs the fast, in-memory unit
// tests in this package (lanes_test.go, oracle_test.go, pattern_test.go).
// The full suite, TestConformance, requires SONDE_CONFORMANCE=1 (set by
// `make conformance`) because it builds a binary, creates a Python
// virtualenv, starts background servers and can take tens of minutes.
// See docs/conformance.md.
package conformance

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

// harness is the shared, opt-in-only test environment: the binary under
// test, the Python venv, and the background servers. It is populated by
// TestMain only when SONDE_CONFORMANCE=1, and torn down on the way out,
// including on SIGINT/SIGTERM/panic.
type harness struct {
	root     string // repository root, for locating the committed manifest.
	hurlRoot string
	shimDir  string
	target   string // the `hurl` binary under test, per SONDE_CONFORMANCE_BIN or a fresh ./cmd/sonde build.
	python   string
	servers  *serverManager
}

var (
	globalHarness   *harness
	globalHarnessMu sync.Mutex
)

func TestMain(m *testing.M) {
	if !boolEnv("SONDE_CONFORMANCE") {
		// Fast path: unit tests only, no environment to set up.
		os.Exit(m.Run())
	}

	h, err := setupHarness()
	if err != nil {
		fmt.Fprintln(os.Stderr, "conformance: setup failed:", err)
		os.Exit(1)
	}
	globalHarnessMu.Lock()
	globalHarness = h
	globalHarnessMu.Unlock()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		h.teardown()
		os.Exit(1)
	}()

	code := func() (code int) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintln(os.Stderr, "conformance: panic during run:", r)
				code = 1
			}
			h.teardown()
		}()
		return m.Run()
	}()
	os.Exit(code)
}

func setupHarness() (*harness, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	hurlRoot := filepath.Join(root, "testdata", "conformance", "hurl")
	shimDir := filepath.Join(root, "test", "conformance", "shim")

	if err := os.MkdirAll(filepath.Join(hurlRoot, "build"), 0o750); err != nil {
		return nil, fmt.Errorf("preparing build dir: %w", err)
	}

	python, err := setupVenv(hurlRoot)
	if err != nil {
		return nil, fmt.Errorf("setting up venv: %w", err)
	}

	target := os.Getenv("SONDE_CONFORMANCE_BIN")
	if target == "" {
		buildDir, err := os.MkdirTemp("", "sonde-conformance-bin-*")
		if err != nil {
			return nil, err
		}
		target, err = buildSondeBinary(buildDir)
		if err != nil {
			return nil, fmt.Errorf("building binary under test: %w", err)
		}
	}

	servers := newServerManager(python, hurlRoot)
	if err := servers.StartBlocking(); err != nil {
		return nil, fmt.Errorf("starting blocking-lane servers: %w", err)
	}
	if err := servers.StartExtended(func(msg string) { fmt.Fprintln(os.Stderr, "conformance:", msg) }); err != nil {
		servers.Stop()
		return nil, fmt.Errorf("starting extended-lane servers: %w", err)
	}

	return &harness{root: root, hurlRoot: hurlRoot, shimDir: shimDir, target: target, python: python, servers: servers}, nil
}

func (h *harness) teardown() {
	if h == nil || h.servers == nil {
		return
	}
	h.servers.Stop()
}

// TestConformance runs every discovered script through the binary under
// test, lane by lane, writes a full report to SONDE_CONFORMANCE_RESULTS
// (default: a fixed path under the OS temp directory; see resultsPath),
// and checks the result against the committed manifest
// (test/conformance/manifest.yaml).
//
// With SONDE_CONFORMANCE_UPDATE=1 (`make conformance-update`) it instead
// rewrites the manifest from this run — see updateManifest — and does not
// gate. Otherwise it gates: any blocking-lane script the manifest marks
// expect: pass that does not semantically pass — including one that is now
// merely skipped, e.g. it starts exiting 255 — fails this test, as does any
// manifest entry whose script no longer exists (see gateConformance);
// newly-passing scripts and manifest gaps are reported but do not fail the
// build. It also fails on harness-level problems (a server that would not
// start, a script the classifier could not read, a results.json or
// manifest write error).
func TestConformance(t *testing.T) {
	if !boolEnv("SONDE_CONFORMANCE") {
		t.Skip("set SONDE_CONFORMANCE=1 to run the conformance suite (see docs/conformance.md); `make conformance` does this")
	}
	globalHarnessMu.Lock()
	h := globalHarness
	globalHarnessMu.Unlock()
	if h == nil {
		t.Fatal("conformance: harness was not initialized by TestMain")
	}

	scripts, err := DiscoverScripts(h.hurlRoot)
	if err != nil {
		t.Fatalf("discovering scripts: %v", err)
	}

	runNetwork := boolEnv("SONDE_CONFORMANCE_NETWORK")
	results := make([]ScriptResult, 0, len(scripts))
	for _, s := range scripts {
		results = append(results, h.runOne(t, s, runNetwork))
	}

	if err := writeResultsJSON(resultsPath(), results); err != nil {
		t.Fatalf("writing results.json: %v", err)
	}
	t.Logf("conformance results written to %s", resultsPath())
	printSummaryTable(os.Stdout, results)

	mPath := manifestPath(h.root)
	manifest, err := LoadManifest(mPath)
	if err != nil {
		t.Fatalf("loading manifest %s: %v", mPath, err)
	}

	if boolEnv("SONDE_CONFORMANCE_UPDATE") {
		updated, report := updateManifest(manifest, scripts, results, boolEnv("CONFORMANCE_ALLOW_DEMOTE"))
		if err := WriteManifest(mPath, updated); err != nil {
			t.Fatalf("writing manifest %s: %v", mPath, err)
		}
		t.Logf("conformance manifest updated: %s", mPath)
		printUpdateReport(os.Stdout, report)
		return
	}

	printLaneRates(os.Stdout, computeLaneRates(scripts, results, manifest))
	gr := gateConformance(scripts, results, manifest)
	printGateReport(os.Stdout, gr)
	for _, r := range gr.Regressions {
		t.Error(r)
	}
	for _, p := range gr.Stale {
		t.Errorf("%s: manifest entry has no matching script (renamed, removed, or a typo); fix it or remove the entry", p)
	}

	enforceMinSemantic(t, results)
}

// runOne runs a single script if its lane is currently runnable, or
// records why it was skipped.
func (h *harness) runOne(t *testing.T, s Script, runNetwork bool) ScriptResult {
	t.Helper()
	base := ScriptResult{Path: s.Path, Lane: s.Lane}

	switch {
	case s.Lane == LaneUnsupported:
		base.Skipped = true
		base.SkipReason = s.Reason
		return base
	case s.Lane == LaneNetwork && !runNetwork:
		base.Skipped = true
		base.SkipReason = "network lane disabled; set SONDE_CONFORMANCE_NETWORK=1"
		return base
	case s.NeedsProxy && !h.servers.HasCapability("proxy"):
		base.Skipped = true
		base.SkipReason = "local proxy (squid) is not available on this host"
		return base
	}

	got, duration, err := runScript(h.shimDir, h.target, h.hurlRoot, s.Path)
	base.Duration = duration
	if err != nil {
		t.Errorf("%s: running script: %v", s.Path, err)
		base.SkipReason = "harness error: " + err.Error()
		return base
	}

	scriptAbs := filepath.Join(h.hurlRoot, s.Path)
	basename := scriptAbs[:len(scriptAbs)-len(filepath.Ext(scriptAbs))]
	verdict := checkOracle(basename, got)

	base.Skipped = verdict.Skip
	if verdict.Skip {
		base.SkipReason = "script exited 255 (the reference runner's own skip signal, e.g. an unmet prerequisite)"
	}
	base.ExpectedExit = verdict.ExpectedExit
	base.ActualExit = verdict.ActualExit
	base.SemanticPass = verdict.SemanticPass()
	base.FullPass = verdict.FullPass()
	base.MismatchInfo = verdict.Reason
	return base
}

// enforceMinSemantic optionally gates the run on the blocking lane's
// semantic pass rate. It is off by default (SONDE_CONFORMANCE_MIN_SEMANTIC
// unset); Phase 5 is expected to set it once sonde's engine lands.
func enforceMinSemantic(t *testing.T, results []ScriptResult) {
	raw := os.Getenv("SONDE_CONFORMANCE_MIN_SEMANTIC")
	if raw == "" {
		return
	}
	var minPct float64
	if _, err := fmt.Sscanf(raw, "%f", &minPct); err != nil {
		t.Fatalf("SONDE_CONFORMANCE_MIN_SEMANTIC=%q is not a number: %v", raw, err)
	}
	for _, s := range summarize(results) {
		if s.Lane != LaneBlocking {
			continue
		}
		if got := s.semanticPct(); got < minPct {
			t.Errorf("blocking-lane semantic pass rate %.1f%% is below SONDE_CONFORMANCE_MIN_SEMANTIC=%.1f%%", got, minPct)
		}
	}
}
