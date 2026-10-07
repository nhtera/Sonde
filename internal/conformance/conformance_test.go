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
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// harness is the shared, opt-in-only test environment: the binary under
// test, the Python venv, and the background servers. It is populated by
// TestMain only when SONDE_CONFORMANCE=1, and torn down on the way out,
// including on SIGINT/SIGTERM/panic.
type harness struct {
	root     string // repository root, for locating the committed manifest.
	confRoot string // holds the hurl/, hurlfmt/ and integration/ trees.
	next     bool   // running the pinned upstream snapshot (make conformance-next).
	// treeRoots maps a tree name to the directory its entry points run
	// from. The formatter tree runs from a scratch copy, so a script that
	// rewrites its input (--in-place) can never touch a vendored file.
	treeRoots map[string]string
	shimDir   string
	ptyDriver string
	target    string // the `hurl` binary under test, per SONDE_CONFORMANCE_BIN or a fresh ./cmd/sonde build.
	python    string
	servers   *serverManager
	scratch   string
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
	next := boolEnv("SONDE_CONFORMANCE_NEXT")
	confRoot, err := conformanceRoot(root, next)
	if err != nil {
		return nil, err
	}
	hurlRoot := filepath.Join(confRoot, TreeHurl)
	shimDir := filepath.Join(root, "internal", "conformance", "shim")

	scratch, err := os.MkdirTemp("", "sonde-conformance-trees-*")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(scratch) //nolint:errcheck // best-effort cleanup after a failed setup.
		}
	}()
	hurlfmtRoot := filepath.Join(scratch, TreeHurlfmt)
	if err := os.CopyFS(hurlfmtRoot, os.DirFS(filepath.Join(confRoot, TreeHurlfmt))); err != nil {
		return nil, fmt.Errorf("copying the formatter tree: %w", err)
	}

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
	// In CI the proxy scripts must run: squid is installed there, so a
	// missing proxy is a broken environment, not a reason to skip.
	if boolEnv("CI") && !servers.HasCapability("proxy") {
		servers.Stop()
		return nil, fmt.Errorf("CI=true but the squid proxy is unavailable; install squid (and stop its system service) so the proxy scripts run")
	}

	ok = true
	return &harness{
		root:      root,
		confRoot:  confRoot,
		next:      next,
		treeRoots: map[string]string{TreeHurl: hurlRoot, TreeHurlfmt: hurlfmtRoot},
		shimDir:   shimDir,
		ptyDriver: filepath.Join(root, "internal", "conformance", "pty-capture.py"),
		target:    target,
		python:    python,
		servers:   servers,
		scratch:   scratch,
	}, nil
}

// conformanceRoot resolves the directory holding the upstream trees:
// SONDE_CONFORMANCE_ROOT if set, else the vendored testdata/conformance, or
// for the pinned snapshot <user cache>/sonde-conformance/next, synced from
// the commit pinned in manifest-next.yaml whenever it does not match.
func conformanceRoot(repo string, next bool) (string, error) {
	confRoot := os.Getenv("SONDE_CONFORMANCE_ROOT")
	if !next {
		if confRoot == "" {
			confRoot = filepath.Join(repo, "testdata", "conformance")
		}
		return confRoot, nil
	}

	commit, err := readPinnedCommit(nextManifestPath(repo))
	if err != nil {
		return "", err
	}
	if !fullCommitRE.MatchString(commit) {
		return "", fmt.Errorf("%s: %q header line must hold a full 40-hex commit, got %q", nextManifestPath(repo), strings.TrimSpace(pinnedCommitPrefix), commit)
	}
	if confRoot != "" {
		// The snapshot root is wiped and re-synced, so it must never be
		// (or sit inside) the repository, where the vendored trees live.
		if confRoot, err = filepath.Abs(confRoot); err != nil {
			return "", err
		}
		if rel, err := filepath.Rel(repo, confRoot); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("SONDE_CONFORMANCE_ROOT=%s is inside the repository; the next snapshot is re-synced there and would overwrite vendored files", confRoot)
		}
	}
	if confRoot == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		// Kept short: the Unix-socket server binds a path under it, and
		// AF_UNIX paths are limited to about 104 bytes.
		confRoot = filepath.Join(cacheDir, "sonde-conformance", "next")
	}
	// integration/SOURCE is the last file the sync script writes, so a
	// matching commit there means the whole sync completed.
	if syncedCommit(filepath.Join(confRoot, "integration", "SOURCE")) == commit {
		return confRoot, nil
	}
	cmd := exec.Command("bash", filepath.Join(repo, "scripts", "sync-hurl-conformance.sh"), commit, confRoot) //nolint:gosec // G204: commit comes from the committed manifest header.
	cmd.Dir = repo
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("syncing upstream %s into %s: %w", commit, confRoot, err)
	}
	return confRoot, nil
}

var fullCommitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// syncedCommit reads the "commit:" line of a synced tree's SOURCE file.
func syncedCommit(sourcePath string) string {
	b, err := os.ReadFile(sourcePath) //nolint:gosec // G304: SOURCE file written by the sync script.
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if c, ok := strings.CutPrefix(line, "commit: "); ok {
			return strings.TrimSpace(c)
		}
	}
	return ""
}

func (h *harness) teardown() {
	if h == nil || h.servers == nil {
		return
	}
	h.servers.Stop()
	if h.scratch != "" {
		_ = os.RemoveAll(h.scratch) //nolint:errcheck // best-effort scratch cleanup.
	}
}

// TestConformance runs every discovered script through the binary under
// test, lane by lane, writes a full report to SONDE_CONFORMANCE_RESULTS
// (default: a fixed path under the OS temp directory; see resultsPath),
// and checks the result against the committed manifest
// (internal/conformance/manifest.yaml).
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

	scripts, err := Discover(h.confRoot)
	if err != nil {
		t.Fatalf("discovering scripts: %v", err)
	}
	if only := os.Getenv("SONDE_CONFORMANCE_ONLY"); only != "" {
		scripts = filterScripts(scripts, only)
	}

	runNetwork := boolEnv("SONDE_CONFORMANCE_NETWORK")
	results := make([]ScriptResult, 0, len(scripts))
	for _, s := range scripts {
		results = append(results, h.runOne(t, s, runNetwork))
	}

	rPath := resultsPath(h.next)
	if err := writeResultsJSON(rPath, results); err != nil {
		t.Fatalf("writing results.json: %v", err)
	}
	t.Logf("conformance results written to %s", rPath)
	printSummaryTable(os.Stdout, results)

	mPath, header := manifestPath(h.root), manifestHeader
	if h.next {
		mPath = nextManifestPath(h.root)
		commit, err := readPinnedCommit(mPath)
		if err != nil {
			t.Fatalf("reading %s: %v", mPath, err)
		}
		header = nextManifestHeader(commit)
	}
	manifest, err := LoadManifest(mPath)
	if err != nil {
		t.Fatalf("loading manifest %s: %v", mPath, err)
	}

	if boolEnv("SONDE_CONFORMANCE_UPDATE") {
		if os.Getenv("SONDE_CONFORMANCE_ONLY") != "" {
			t.Fatal("SONDE_CONFORMANCE_ONLY runs a subset; it cannot rewrite the manifest")
		}
		policy, err := parseDemotePolicy(os.Getenv("CONFORMANCE_DEMOTE_ONLY"), os.Getenv("CONFORMANCE_DEMOTE_REASON"),
			boolEnv("CONFORMANCE_ALLOW_DEMOTE"), scripts)
		if err != nil {
			t.Fatal(err)
		}
		updated, report := updateManifest(manifest, scripts, results, policy)
		if err := writeManifest(mPath, header, updated); err != nil {
			t.Fatalf("writing manifest %s: %v", mPath, err)
		}
		t.Logf("conformance manifest updated: %s", mPath)
		printUpdateReport(os.Stdout, report)
		return
	}

	printLaneRates(os.Stdout, computeLaneRates(scripts, results, manifest))
	if os.Getenv("SONDE_CONFORMANCE_ONLY") != "" {
		manifest = manifestSubset(manifest, scripts)
	}
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

	treeRoot := h.treeRoots[s.Tree]
	var (
		got      processResult
		duration time.Duration
		err      error
	)
	switch {
	case s.Export != "":
		got, duration, err = runExport(h.shimDir, h.target, treeRoot, s.Rel, s.Export)
	case s.Lane == LanePTY:
		if runtime.GOOS == "windows" {
			base.Skipped = true
			base.SkipReason = "the pty lane needs a POSIX pty"
			return base
		}
		got, duration, err = runPTYScript(h.python, h.ptyDriver, filepath.Join(h.confRoot, "integration"),
			h.shimDir, h.target, treeRoot, s.Rel)
	default:
		got, duration, err = runScript(h.shimDir, h.target, treeRoot, s.Rel)
	}
	base.Duration = duration
	if err != nil {
		t.Errorf("%s: running script: %v", s.Path, err)
		base.SkipReason = "harness error: " + err.Error()
		return base
	}

	var verdict oracleVerdict
	if s.Export != "" {
		verdict = checkExport(filepath.Join(treeRoot, s.Rel), s.Export, got)
	} else {
		scriptAbs := filepath.Join(treeRoot, s.Rel)
		verdict = checkOracle(strings.TrimSuffix(scriptAbs, filepath.Ext(scriptAbs)), got)
	}

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
