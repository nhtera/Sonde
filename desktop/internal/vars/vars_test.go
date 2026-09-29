// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package vars

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
	"github.com/nhtera/sonde/internal/sandbox"
)

const secret = "vars-secret-sentinel-6" //nolint:gosec // G101: test sentinel

func TestFor(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"sonde.yaml":            "version: 1\nenvironments:\n  local:\n    variables:\n      base: http://l\n      user: u1\n    secrets_files: [local.secrets]\ndefaults:\n  env: local\n",
		"local.secrets":         "key=" + secret + "\n",
		"api/users.hurl":        "GET {{base}}\n",
		"other/sonde.yaml":      "version: 1\nenvironments:\n  local:\n    variables:\n      only_other: x\n",
		"other/deep/their.hurl": "GET x\n",
	}
	for rel, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, _ := sandbox.Open(dir)
	v := New(func() *sandbox.Root { return root },
		func(string) []runsvc.Capture {
			return []runsvc.Capture{{Name: "user", Value: "u7"}, {Name: "tok", Secret: true}}
		},
		func() map[string]string { return map[string]string{"base": "http://override"} })
	got, err := v.For("api/users.hurl", "")
	if err != nil {
		t.Fatal(err)
	}
	redactcheck.AssertNoSecret(t, "vars", got, secret)
	want := map[string]Var{
		"base": {Name: "base", Display: "http://override", Source: SourceOverride},
		"key":  {Name: "key", Source: SourceProject, Origin: "local.secrets", Secret: true},
		"tok":  {Name: "tok", Source: SourceCapture, Secret: true},
		"user": {Name: "user", Display: "u7", Source: SourceCapture},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, g := range got {
		if g != want[g.Name] {
			t.Errorf("%s: %+v, want %+v", g.Name, g, want[g.Name])
		}
	}
	// A nested project's own sonde.yaml, as a run would find it.
	got, err = v.For("other/deep/their.hurl", "local")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range got {
		found = found || g.Name == "only_other"
	}
	if !found {
		t.Errorf("nested project: %+v", got)
	}
	if _, err := v.For("../x.hurl", ""); err == nil {
		t.Error("a path outside the project")
	}
}
