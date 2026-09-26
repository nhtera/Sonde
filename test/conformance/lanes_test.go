// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

// writeScript creates dir/rel with content, making any intermediate
// directories as needed.
func writeScript(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // G306: test fixture.
		t.Fatal(err)
	}
}

func TestClassifyBlockingByDefault(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/hello/hello.sh", "hurl tests_ok/hello/hello.hurl\n")
	writeScript(t, root, "tests_ok/hello/hello.hurl", "GET http://localhost:8000/hello\nHTTP 200\n")

	got := classify(root, "tests_ok/hello/hello.sh")
	if got.Lane != LaneBlocking {
		t.Errorf("Lane = %v, want %v (%s)", got.Lane, LaneBlocking, got.Reason)
	}
}

func TestClassifyNetworkHost(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/live/live.sh", "hurl tests_ok/live/live.hurl\n")
	writeScript(t, root, "tests_ok/live/live.hurl", "GET https://example.com/hello\nHTTP 200\n")

	got := classify(root, "tests_ok/live/live.sh")
	if got.Lane != LaneNetwork {
		t.Errorf("Lane = %v, want %v", got.Lane, LaneNetwork)
	}
}

func TestClassifyResolveOptionStaysBlocking(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/resolve/resolve.sh", "hurl tests_ok/resolve/resolve.hurl\n")
	writeScript(t, root, "tests_ok/resolve/resolve.hurl",
		"GET http://foo.com:8000/x\n[Options]\nresolve: foo.com:8000:127.0.0.1\nHTTP 200\n")

	got := classify(root, "tests_ok/resolve/resolve.sh")
	if got.Lane != LaneBlocking {
		t.Errorf("Lane = %v, want %v (%s)", got.Lane, LaneBlocking, got.Reason)
	}
}

func TestClassifyConnectToFlagStaysBlocking(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/connect_to/connect_to.sh",
		"hurl --connect-to foo.com:80:localhost:8000 tests_ok/connect_to/connect_to.hurl\n")
	writeScript(t, root, "tests_ok/connect_to/connect_to.hurl",
		"GET http://foo.com/x\nHTTP 200\n")

	got := classify(root, "tests_ok/connect_to/connect_to.sh")
	if got.Lane != LaneBlocking {
		t.Errorf("Lane = %v, want %v (%s)", got.Lane, LaneBlocking, got.Reason)
	}
}

func TestClassifyUnixSocketStaysExtended(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_unix_socket/unix_socket.sh",
		"hurl --unix-socket build/unix_socket.sock tests_unix_socket/unix_socket.hurl\n")
	writeScript(t, root, "tests_unix_socket/unix_socket.hurl",
		"GET http://example/hello\nHTTP 200\n")

	got := classify(root, "tests_unix_socket/unix_socket.sh")
	if got.Lane != LaneExtended {
		t.Errorf("Lane = %v, want %v (%s)", got.Lane, LaneExtended, got.Reason)
	}
}

func TestClassifySSLDirIsExtended(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ssl/cacert.sh", "hurl tests_ssl/cacert.hurl\n")
	writeScript(t, root, "tests_ssl/cacert.hurl", "GET https://localhost:8001/hello\nHTTP 200\n")

	got := classify(root, "tests_ssl/cacert.sh")
	if got.Lane != LaneExtended {
		t.Errorf("Lane = %v, want %v (%s)", got.Lane, LaneExtended, got.Reason)
	}
}

func TestClassifyIPv6IsExtended(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/ipv6/ipv6.sh", "hurl --ipv6 tests_ok/ipv6/ipv6.hurl\n")
	writeScript(t, root, "tests_ok/ipv6/ipv6.hurl", "GET http://[::1]:8004/hello\nHTTP 200\n")

	got := classify(root, "tests_ok/ipv6/ipv6.sh")
	if got.Lane != LaneExtended {
		t.Errorf("Lane = %v, want %v (%s)", got.Lane, LaneExtended, got.Reason)
	}
}

func TestClassifyProxyNeedsProxyCapability(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/proxy/proxy.sh", "hurl --proxy localhost:3128 tests_ok/proxy/proxy.hurl\n")
	writeScript(t, root, "tests_ok/proxy/proxy.hurl", "GET http://127.0.0.1:8000/proxy\nHTTP 200\n")

	got := classify(root, "tests_ok/proxy/proxy.sh")
	if got.Lane != LaneExtended || !got.NeedsProxy {
		t.Errorf("got %+v, want extended lane with NeedsProxy", got)
	}
}

func TestClassifyGlobExpansion(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/glob/glob.sh", `hurl --glob "tests_ok/glob/*.hurl"`+"\n")
	writeScript(t, root, "tests_ok/glob/a.hurl", "GET https://not-local.example/hello\nHTTP 200\n")

	got := classify(root, "tests_ok/glob/glob.sh")
	if got.Lane != LaneNetwork {
		t.Errorf("Lane = %v, want %v (%s) — glob-referenced .hurl files must be scanned", got.Lane, LaneNetwork, got.Reason)
	}
}

func TestClassifyExplicitTimingOverride(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_ok/delay/delay.sh", "hurl tests_ok/delay/delay.hurl\n")
	writeScript(t, root, "tests_ok/delay/delay.hurl", "GET http://localhost:8000/delay\nHTTP 200\n")

	got := classify(root, "tests_ok/delay/delay.sh")
	if got.Lane != LaneTiming {
		t.Errorf("Lane = %v, want %v", got.Lane, LaneTiming)
	}
}

// TestDiscoverScriptsCorpusShape is a corpus-drift canary: it runs the real
// classifier against the vendored fixtures (testdata/conformance/hurl,
// pinned by SOURCE) and checks gross shape, not exact counts, since the
// heuristic is expected to keep working as long as the vendored tag does
// not change. It needs no environment variable and does not start any
// server or process.
func TestDiscoverScriptsCorpusShape(t *testing.T) {
	root := repoRootHurlDir(t)
	scripts, err := DiscoverScripts(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(scripts) < 300 {
		t.Errorf("DiscoverScripts found %d scripts, expected at least 300", len(scripts))
	}

	counts := map[Lane]int{}
	for _, s := range scripts {
		counts[s.Lane]++
		if s.Lane == "" {
			t.Errorf("script %s has an empty lane", s.Path)
		}
	}
	if counts[LaneBlocking] == 0 {
		t.Error("no scripts classified as blocking")
	}
	// As of the vendored reference-implementation 8.0.1 tag, exactly one script (completion_bash.sh)
	// needs a file outside the vendored tree; see the explicit override in
	// buildExplicitLanes. A different count means the corpus changed.
	if counts[LaneUnsupported] != 1 {
		t.Errorf("LaneUnsupported has %d scripts, want 1 (tests_ok/completion/completion_bash.sh)", counts[LaneUnsupported])
	}
}

// repoRootHurlDir locates testdata/conformance/hurl relative to this test
// file, independent of the working directory `go test` chooses to run in.
func repoRootHurlDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// This file lives at <root>/test/conformance/lanes_test.go.
	return filepath.Join(wd, "..", "..", "testdata", "conformance", "hurl")
}
