// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Lane groups scripts that need the same test environment and the same
// gating policy.
type Lane string

const (
	// LaneBlocking scripts need only the plain HTTP server on :8000. This
	// is the lane whose pass rate matters for CI gating (Phase 5).
	LaneBlocking Lane = "blocking"
	// LaneExtended scripts additionally need TLS servers, the Unix-socket
	// server, an IPv6 listener, or the local proxy.
	LaneExtended Lane = "extended"
	// LaneNetwork scripts talk to hosts outside localhost/127.0.0.1/::1;
	// skipped unless SONDE_CONFORMANCE_NETWORK=1.
	LaneNetwork Lane = "network"
	// LaneTiming scripts are known to be timing-sensitive (sleeps,
	// retries, rate limits) and flaky under load; run, but reported
	// separately from LaneBlocking.
	LaneTiming Lane = "timing"
	// LaneUnsupported scripts need tooling the harness does not provide.
	// Not run.
	LaneUnsupported Lane = "unsupported"
	// LaneHurlfmt holds the formatter tree: its scripts plus one synthetic
	// entry per export fixture comparison (lint/json/html). Gating.
	LaneHurlfmt Lane = "hurlfmt"
	// LanePTY scripts need a terminal on stdout and stderr; they run
	// through the upstream two-pty runner (integration/term.py). Gating.
	LanePTY Lane = "pty"
)

// gatingLanes are the lanes whose expect: pass entries fail the run when
// they stop passing. Every other lane is report-only.
var gatingLanes = map[Lane]bool{LaneBlocking: true, LaneHurlfmt: true, LanePTY: true}

// Tree names: the first segment of every manifest key, and the directory
// under the conformance root that holds the vendored upstream tree.
const (
	TreeHurl    = "hurl"
	TreeHurlfmt = "hurlfmt"
)

// dirsInScope lists the vendored directories the harness walks, and
// whether their *.sh scripts are found recursively (tests_ok and
// tests_failed nest one script per subdirectory) or directly inside the
// directory. This mirrors integration/hurl/integration.py in the reference
// repository, plus tests_unix_socket which that script omits but our plan
// includes.
var dirsInScope = []struct {
	dir       string
	recursive bool
}{
	{"tests_ok", true},
	{"tests_ok_not_linted", false},
	{"tests_failed", true},
	{"tests_failed_not_linted", false},
	{"tests_error_parser", false},
	{"tests_ssl", false},
	{"tests_unix_socket", false},
	{"tests_pty", true},
}

// laneOverride is an explicit classification for a script whose lane
// heuristics alone would get wrong or that needs a human-readable reason.
type laneOverride struct {
	lane   Lane
	reason string
}

// explicitLanes overrides the heuristic for scripts where "what capability
// does this script need" cannot be read off its request lines:
//   - timing-sensitive directories (sleeps, retries, rate limiting) are
//     pulled out of LaneBlocking so a slow CI runner cannot make the
//     headline blocking pass rate flaky.
//   - resolve/proxy_option.sh's target is written inline in its .hurl file
//     with a "resolve:"/"connect-to:" option, not on the command line; the
//     heuristic (see isLocallyResolved) already handles that case without
//     an override.
var explicitLanes = buildExplicitLanes()

func buildExplicitLanes() map[string]laneOverride {
	unsupported := map[string]string{
		// Sources ../../completions/hurl.bash: outside the vendored tree
		// (scripts/sync-hurl-conformance.sh only pulls integration/hurl/,
		// bin/requirements-frozen.txt and LICENSE from upstream).
		"tests_ok/completion": "needs completions/hurl.bash, which is not vendored",
	}
	timing := []string{
		"tests_ok/delay",
		"tests_ok/retry",
		"tests_failed/retry",
		"tests_ok/limit_rate",
		"tests_ok/progress_bar",
		"tests_failed/timeout",
		"tests_failed/connect_timeout",
		"tests_ok/bench",
		"tests_ok/repeat",
	}
	m := map[string]laneOverride{}
	for prefix, reason := range unsupported {
		m[prefix] = laneOverride{lane: LaneUnsupported, reason: reason}
	}
	for _, prefix := range timing {
		m[prefix] = laneOverride{lane: LaneTiming, reason: "known timing-sensitive directory"}
	}
	return m
}

// Script is one discovered, classified conformance test entry point.
type Script struct {
	// Path is the manifest key: the tree name, then the path inside that
	// tree, forward-slash separated, e.g.
	// "hurl/tests_ok/add_header/add_header.sh". An export fixture
	// comparison appends "#lint", "#json" or "#html" to its .hurl path.
	// DiscoverScripts alone (one tree) leaves the tree prefix off.
	Path string
	// Tree is TreeHurl or TreeHurlfmt; Rel is the script (or, for an export
	// comparison, the .hurl input) relative to that tree's root.
	Tree string
	Rel  string
	// Export is the hurlfmt --out format an export fixture comparison
	// checks ("hurl", "json" or "html"); empty for a script.
	Export string
	Lane   Lane
	// Reason explains the lane, mainly useful for LaneUnsupported and
	// LaneNetwork.
	Reason string
	// NeedsProxy is set when the script's classification depends on the
	// local squid proxy (port 3128); the runner skips it at run time if
	// squid could not be started, without changing its static lane.
	NeedsProxy bool
}

// Discover finds every entry point under a conformance root (the directory
// holding the hurl/ and hurlfmt/ trees) and keys it by tree.
func Discover(confRoot string) ([]Script, error) {
	hurl, err := DiscoverScripts(filepath.Join(confRoot, TreeHurl))
	if err != nil {
		return nil, err
	}
	fmtScripts, err := DiscoverHurlfmt(filepath.Join(confRoot, TreeHurlfmt))
	if err != nil {
		return nil, err
	}
	out := make([]Script, 0, len(hurl)+len(fmtScripts))
	for _, s := range hurl {
		s.Tree, s.Rel, s.Path = TreeHurl, s.Path, TreeHurl+"/"+s.Path
		out = append(out, s)
	}
	return append(out, fmtScripts...), nil
}

// DiscoverScripts walks the vendored directories under root (the hurl
// tree) and classifies every *.sh entry point it finds. Paths are relative
// to root, without the tree prefix Discover adds.
func DiscoverScripts(root string) ([]Script, error) {
	var paths []string
	for _, d := range dirsInScope {
		found, err := findScripts(filepath.Join(root, d.dir), d.recursive)
		if err != nil {
			return nil, err
		}
		paths = append(paths, found...)
	}
	sort.Strings(paths)

	scripts := make([]Script, 0, len(paths))
	for _, abs := range paths {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		scripts = append(scripts, classify(root, rel))
	}
	return scripts, nil
}

func findScripts(dir string, recursive bool) ([]string, error) {
	if recursive {
		var found []string
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".sh") {
				found = append(found, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return found, nil
	}
	return filepath.Glob(filepath.Join(dir, "*.sh"))
}

var (
	hurlFileRefRE = regexp.MustCompile(`[^\s"']+\.hurl\b`)
	globFlagRE    = regexp.MustCompile(`--glob[ =]"([^"]+)"`)
	// The host group skips an optional "user:pass@" prefix and accepts a
	// bracketed IPv6 literal ("[::1]") or a plain hostname.
	requestLineRE = regexp.MustCompile(`(?m)^(?:GET|HEAD|POST|PUT|DELETE|PATCH|OPTIONS|CONNECT|TRACE)\s+https?://(?:[^/@\s]+@)?(\[[^\]]+\]|[^/:\s]+)`)
)

var localHosts = map[string]bool{
	"localhost": true,
	"127.0.0.1": true,
	"::1":       true,
	"[::1]":     true,
}

// classify decides which lane rel (a script path relative to root) belongs
// to, reading the script and the .hurl files it passes to the CLI under
// test so the decision is based on what the script actually does, not just
// its directory.
func classify(root, rel string) Script {
	for prefix, override := range explicitLanes {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return Script{Path: rel, Lane: override.lane, Reason: override.reason}
		}
	}
	if strings.HasPrefix(rel, "tests_pty/") {
		return Script{Path: rel, Lane: LanePTY, Reason: "needs a terminal (upstream two-pty runner)"}
	}

	scriptBody, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return Script{Path: rel, Lane: LaneUnsupported, Reason: "could not read script: " + err.Error()}
	}

	combined := string(scriptBody)
	for _, hurlRel := range referencedHurlFiles(root, string(scriptBody)) {
		if b, err := os.ReadFile(filepath.Join(root, hurlRel)); err == nil { //nolint:gosec // G304: vendored fixture tree.
			combined += "\n" + string(b)
		}
	}

	if host, ok := externalHost(combined); ok {
		return Script{Path: rel, Lane: LaneNetwork, Reason: "references external host " + host}
	}

	needsProxy := strings.Contains(combined, "3128")
	switch {
	case strings.HasPrefix(rel, "tests_unix_socket/"):
		return Script{Path: rel, Lane: LaneExtended, Reason: "Unix-socket server", NeedsProxy: needsProxy}
	case strings.HasPrefix(rel, "tests_ssl/"):
		return Script{Path: rel, Lane: LaneExtended, Reason: "TLS server", NeedsProxy: needsProxy}
	case strings.Contains(combined, "--ipv6") || strings.Contains(combined, "HURL_IPV6"):
		return Script{Path: rel, Lane: LaneExtended, Reason: "IPv6 listener", NeedsProxy: needsProxy}
	case needsProxy:
		return Script{Path: rel, Lane: LaneExtended, Reason: "local proxy (squid)", NeedsProxy: true}
	default:
		return Script{Path: rel, Lane: LaneBlocking}
	}
}

// referencedHurlFiles finds every .hurl file a script passes to the CLI,
// resolving --glob "PATTERN" expressions as well as literal path tokens.
func referencedHurlFiles(root, scriptBody string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(rel string) {
		rel = filepath.ToSlash(rel)
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}

	for _, tok := range hurlFileRefRE.FindAllString(scriptBody, -1) {
		add(tok)
	}
	for _, m := range globFlagRE.FindAllStringSubmatch(scriptBody, -1) {
		matches, _ := filepath.Glob(filepath.Join(root, m[1]))
		for _, abs := range matches {
			if rel, err := filepath.Rel(root, abs); err == nil {
				add(rel)
			}
		}
	}
	return out
}

// externalHost reports the first non-local host found in a real request
// line (METHOD URL) in combined script+fixture text, unless the text also
// shows the script resolving hosts locally via --resolve/--connect-to (on
// the command line or as an inline [Options] entry), in which case every
// non-local hostname in it is presumed to be redirected to localhost.
func externalHost(combined string) (string, bool) {
	if strings.Contains(combined, "resolve:") || strings.Contains(combined, "connect-to:") ||
		strings.Contains(combined, "--resolve") || strings.Contains(combined, "--connect-to") ||
		strings.Contains(combined, "--unix-socket") {
		return "", false
	}
	for _, m := range requestLineRE.FindAllStringSubmatch(combined, -1) {
		host := m[1]
		if !localHosts[host] {
			return host, true
		}
	}
	return "", false
}

// filterScripts keeps the scripts whose manifest key starts with one of the
// comma-separated prefixes in only (SONDE_CONFORMANCE_ONLY), for iterating
// on a handful of scripts without running the whole suite.
func filterScripts(scripts []Script, only string) []Script {
	var prefixes []string
	for _, p := range strings.Split(only, ",") {
		if p = strings.TrimSpace(p); p != "" {
			prefixes = append(prefixes, p)
		}
	}
	var out []Script
	for _, s := range scripts {
		for _, p := range prefixes {
			if strings.HasPrefix(s.Path, p) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// manifestSubset keeps the manifest entries of the given scripts, so a
// filtered run does not report every other entry as stale.
func manifestSubset(m Manifest, scripts []Script) Manifest {
	out := make(Manifest, len(scripts))
	for _, s := range scripts {
		if e, ok := m[s.Path]; ok {
			out[s.Path] = e
		}
	}
	return out
}
