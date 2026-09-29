// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestPlan checks that Plan lists exactly what Write then writes, with
// the planned paths aligned with Output.Files, and writes nothing itself.
func TestPlan(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	out := Output{
		Files:       []GeneratedFile{{Path: "a/b c", File: mustFile(t, "https://x/1")}, {Path: "a/b c", File: mustFile(t, "https://x/2")}},
		Extra:       []RawFile{{Path: "dev.secrets", Data: []byte("k=\n"), Keep: true}},
		ProjectYAML: []byte("version: 1\n"),
	}
	res, files, err := Plan(target, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("Plan created the directory")
	}
	if len(files) != 4 || files[0].Path == files[1].Path || files[2].Perm != 0o600 || files[3].Path != ProjectFileName {
		t.Fatalf("files %+v", files)
	}
	if res.RequestCount != 2 || res.Project != ProjectFileName {
		t.Errorf("result %+v", res)
	}
	if _, err := Write(target, out, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		got, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(f.Path)))
		if err != nil || !bytes.Equal(got, f.Data) {
			t.Errorf("%s: written %q, planned %q (%v)", f.Path, got, f.Data, err)
		}
	}
	// Now every file exists: a plan without --force reports conflicts,
	// and keeps the secrets stub and sonde.yaml with --force.
	var conflicts *ErrConflicts
	if _, _, err := Plan(target, out, Options{}); !errors.As(err, &conflicts) || len(conflicts.Files) != 2 {
		t.Errorf("conflicts: %v", err)
	}
	res, files, err = Plan(target, out, Options{Force: true})
	if err != nil || len(files) != 2 || !res.ProjectSkipped || len(res.ExtraKept) != 1 {
		t.Errorf("forced plan %+v %+v %v", res, files, err)
	}
}
