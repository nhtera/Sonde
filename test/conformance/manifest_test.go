// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValidateRejectsUnknownExpect(t *testing.T) {
	m := Manifest{"a.sh": {Lane: LaneBlocking, Expect: "maybe"}}
	if err := m.Validate(); err == nil {
		t.Fatal("Validate: want error for expect \"maybe\", got nil")
	}
}

func TestManifestValidateRequiresReasonForFailAndSkip(t *testing.T) {
	for _, expect := range []Expect{ExpectFail, ExpectSkip} {
		m := Manifest{"a.sh": {Lane: LaneBlocking, Expect: expect}}
		if err := m.Validate(); err == nil {
			t.Errorf("Validate: want error for expect %q with no reason, got nil", expect)
		}
	}
}

func TestManifestValidateAllowsPassWithoutReason(t *testing.T) {
	m := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectPass}}
	if err := m.Validate(); err != nil {
		t.Errorf("Validate: unexpected error: %v", err)
	}
}

func TestLoadManifestMissingFileIsEmpty(t *testing.T) {
	m, err := LoadManifest(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("LoadManifest of a missing file = %v, want empty", m)
	}
}

func TestLoadManifestRejectsInvalidContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte("a.sh:\n  lane: blocking\n  expect: fail\n"), 0o644); err != nil { //nolint:gosec // G306: test fixture.
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("LoadManifest: want error for expect: fail with no reason, got nil")
	}
}

func TestWriteThenLoadManifestRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	want := Manifest{
		"tests_ok/hello/hello.sh":      {Lane: LaneBlocking, Expect: ExpectPass},
		"tests_error_parser/base64.sh": {Lane: LaneBlocking, Expect: ExpectFail, Reason: "reports: Phase 5 in progress"},
		"tests_ssl/cacert.sh":          {Lane: LaneExtended, Expect: ExpectSkip, Reason: "network"},
		"tests_ok/live/live.sh":        {Lane: LaneNetwork, Expect: ExpectSkip, Reason: "network"},
	}

	if err := WriteManifest(path, want); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	got, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("LoadManifest round-trip = %d entries, want %d", len(got), len(want))
	}
	for path, entry := range want {
		if got[path] != entry {
			t.Errorf("entry %s = %+v, want %+v", path, got[path], entry)
		}
	}
}

func TestWriteManifestRejectsInvalidContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	bad := Manifest{"a.sh": {Lane: LaneBlocking, Expect: ExpectFail}} // no reason
	if err := WriteManifest(path, bad); err == nil {
		t.Fatal("WriteManifest: want error for expect: fail with no reason, got nil")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("WriteManifest: should not have written a file when validation fails")
	}
}

func TestWriteManifestIsSortedAndStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	m := Manifest{
		"zeta.sh":  {Lane: LaneBlocking, Expect: ExpectPass},
		"alpha.sh": {Lane: LaneBlocking, Expect: ExpectPass},
	}
	if err := WriteManifest(path, m); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(path, m); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("WriteManifest is not stable across repeated writes of the same content")
	}

	alphaIdx := indexOf(t, string(first), "alpha.sh:")
	zetaIdx := indexOf(t, string(first), "zeta.sh:")
	if alphaIdx > zetaIdx {
		t.Error("WriteManifest did not sort entries by script path")
	}
}

func indexOf(t *testing.T, s, substr string) int {
	t.Helper()
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	t.Fatalf("substring %q not found in %q", substr, s)
	return -1
}

func TestManifestPathIsUnderTestConformanceNotVendoredTree(t *testing.T) {
	got := manifestPath("/repo")
	want := filepath.Join("/repo", "test", "conformance", "manifest.yaml")
	if got != want {
		t.Errorf("manifestPath = %q, want %q", got, want)
	}
}
