// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Expect is the semantic outcome the manifest records for a script: what
// TestConformance should see the next time it runs the suite.
type Expect string

const (
	// ExpectPass means the script must semantically pass (exit code and
	// stdout oracle, if any, both match); a regression here fails the
	// blocking-lane gate.
	ExpectPass Expect = "pass"
	// ExpectFail means the script is known to fail right now; it needs a
	// Reason and never gates the build, but a script that starts passing
	// is reported as newly passing so it can be promoted.
	ExpectFail Expect = "fail"
	// ExpectSkip means the script is expected to be skipped this run (an
	// unmet prerequisite, a disabled lane, or missing tooling); it needs a
	// Reason for the same reasons as ExpectFail.
	ExpectSkip Expect = "skip"
)

// ManifestEntry is the recorded expectation for one script.
type ManifestEntry struct {
	Lane   Lane   `yaml:"lane"`
	Expect Expect `yaml:"expect"`
	// Reason explains why the script is not expected to pass (required for
	// ExpectFail and ExpectSkip; e.g. "reports: Phase 5 in progress",
	// "digest unsupported", "network"). Left empty for ExpectPass.
	Reason string `yaml:"reason,omitempty"`
}

// Manifest maps a script path (relative to testdata/conformance/hurl,
// forward-slash separated, e.g. "tests_ok/hello/hello.sh") to its recorded
// expectation. It is committed at internal/conformance/manifest.yaml — never
// inside the vendored testdata/conformance/hurl tree.
type Manifest map[string]ManifestEntry

// Validate checks manifest invariants: Expect must be one of
// pass/fail/skip, and every fail/skip entry must carry a non-empty Reason.
func (m Manifest) Validate() error {
	var problems []string
	for path, e := range m {
		switch e.Expect {
		case ExpectPass, ExpectFail, ExpectSkip:
		default:
			problems = append(problems, fmt.Sprintf("%s: expect %q is not one of pass|fail|skip", path, e.Expect))
			continue
		}
		if e.Expect != ExpectPass && strings.TrimSpace(e.Reason) == "" {
			problems = append(problems, fmt.Sprintf("%s: expect %q needs a reason", path, e.Expect))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("manifest validation failed:\n  %s", strings.Join(problems, "\n  "))
}

// LoadManifest reads and validates the manifest at path. A missing file is
// treated as an empty manifest (every script implicitly unclassified)
// rather than an error, so a fresh checkout can bootstrap one with
// `make conformance-update`.
func LoadManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is the fixed, repo-relative manifest location.
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, nil
		}
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m == nil {
		m = Manifest{}
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// manifestHeader is written above the YAML mapping so a human opening the
// file understands what it is and how it is maintained without reading this
// package's source.
const manifestHeader = `# Conformance manifest: the expected outcome for every classified script in
# the vendored Hurl integration-test corpus (testdata/conformance/hurl).
#
# Generated and gated by internal/conformance; see docs/conformance.md. Do not
# hand-edit lane/expect for large swaths at once — run
# ` + "`make conformance-update`" + ` and let it promote newly passing scripts,
# then hand-edit reasons for anything still failing. Demoting an expect:
# pass entry requires CONFORMANCE_ALLOW_DEMOTE=1, so a regression always
# fails ` + "`make conformance`" + ` first rather than being silently absorbed.
`

// WriteManifest writes m to path as YAML, sorted by script path for a
// stable, diffable file.
func WriteManifest(path string, m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(manifestHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec // G306: committed source file, not sensitive.
}

// manifestPath returns the fixed, committed location of the conformance
// manifest given the repository root.
func manifestPath(root string) string {
	return filepath.Join(root, "internal", "conformance", "manifest.yaml")
}
