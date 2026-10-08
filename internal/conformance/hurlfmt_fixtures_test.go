// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"path/filepath"
	"testing"
)

func TestCheckExport(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tests_export/body.hurl", "GET http://localhost:8000\n")
	writeScript(t, root, "tests_export/body.json", "{\r\n}\n")
	hurlPath := filepath.Join(root, "tests_export/body.hurl")

	// Only stdout counts: the expected file is read with universal
	// newlines, and a non-zero exit with matching output still passes.
	if v := checkExport(hurlPath, "json", processResult{ExitCode: 3, Stdout: []byte("{\n}\n")}); !v.SemanticPass() || !v.FullPass() {
		t.Errorf("matching stdout: verdict %+v, want pass", v)
	}
	// A deliberately broken fixture fails.
	if v := checkExport(hurlPath, "json", processResult{Stdout: []byte("{}\n")}); v.SemanticPass() || v.Reason == "" {
		t.Errorf("differing stdout: verdict %+v, want fail with a reason", v)
	}
	if v := checkExport(hurlPath, "html", processResult{}); v.SemanticPass() {
		t.Errorf("missing expected file: verdict %+v, want fail", v)
	}
}
