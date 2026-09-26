// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func TestEmitProjectRoundTrip(t *testing.T) {
	skel := ProjectSkeleton{
		Environments: map[string]EnvironmentSkeleton{
			"local": {
				Variables:    map[string]string{"base_url": "http://localhost:8080", "retries": "3"},
				SecretsFiles: []string{"env/local.secrets"},
			},
			"staging": {
				Variables: map[string]string{"base_url": "https://staging.example.internal"},
			},
		},
		DefaultEnv:  "local",
		OpenAPISpec: "openapi.yaml",
	}
	b, err := EmitProject(skel)
	if err != nil {
		t.Fatalf("EmitProject: %v", err)
	}
	if !strings.HasPrefix(string(b), "version: 1\n") {
		t.Fatalf("output does not start with version: 1:\n%s", b)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sonde.yaml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	// buildOpenAPI resolves the spec path but doesn't require it to exist,
	// but LoadProject reads the file itself, so a spec path is enough.

	p, err := LoadProject(path)
	if err != nil {
		t.Fatalf("LoadProject(EmitProject output): %v\n%s", err, b)
	}
	if p.Version != 1 {
		t.Errorf("Version = %d, want 1", p.Version)
	}
	if p.Defaults.Env != "local" {
		t.Errorf("Defaults.Env = %q, want local", p.Defaults.Env)
	}
	if p.OpenAPI == nil || p.OpenAPI.Spec != filepath.Join(dir, "openapi.yaml") {
		t.Errorf("OpenAPI = %+v", p.OpenAPI)
	}
	local, ok := p.Environments["local"]
	if !ok {
		t.Fatal("missing local environment")
	}
	if got, want := local.Variables["base_url"], value.String("http://localhost:8080"); got != want {
		t.Errorf("local.base_url = %#v, want %#v", got, want)
	}
	if got, want := local.Variables["retries"], value.String("3"); got != want {
		t.Errorf("local.retries = %#v, want %#v (retries was emitted as a string, not a number)", got, want)
	}
	if len(local.SecretsFiles) != 1 || local.SecretsFiles[0] != "env/local.secrets" {
		t.Errorf("local.SecretsFiles = %v", local.SecretsFiles)
	}
	staging, ok := p.Environments["staging"]
	if !ok {
		t.Fatal("missing staging environment")
	}
	if got, want := staging.Variables["base_url"], value.String("https://staging.example.internal"); got != want {
		t.Errorf("staging.base_url = %#v, want %#v", got, want)
	}
}

func TestEmitProjectMinimal(t *testing.T) {
	b, err := EmitProject(ProjectSkeleton{})
	if err != nil {
		t.Fatalf("EmitProject: %v", err)
	}
	if string(b) != "version: 1\n" {
		t.Errorf("output = %q, want \"version: 1\\n\"", b)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sonde.yaml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProject(path)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if len(p.Environments) != 0 || p.Defaults.Env != "" || p.OpenAPI != nil {
		t.Errorf("unexpected content: %+v", p)
	}
}

// TestEmitProjectDeterministic checks that repeated emits of the same
// skeleton produce byte-identical output (map key order is stable).
func TestEmitProjectDeterministic(t *testing.T) {
	skel := ProjectSkeleton{
		Environments: map[string]EnvironmentSkeleton{
			"z": {Variables: map[string]string{"z1": "1", "a1": "2", "m1": "3"}},
			"a": {Variables: map[string]string{"x": "1"}},
			"m": {},
		},
	}
	first, err := EmitProject(skel)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		got, err := EmitProject(skel)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(first) {
			t.Fatalf("output is not deterministic:\n--- first ---\n%s\n--- got ---\n%s", first, got)
		}
	}
}

func TestEmitVariables(t *testing.T) {
	b := EmitVariables(map[string]string{"zeta": "1", "alpha": "hello", "mid": "true"})
	want := "alpha=hello\nmid=true\nzeta=1\n"
	if string(b) != want {
		t.Errorf("EmitVariables = %q, want %q", b, want)
	}

	// Round trip through ParseProperties, the reader for this format.
	assignments, err := ParseProperties(b, Forced)
	if err != nil {
		t.Fatalf("ParseProperties: %v", err)
	}
	got := map[string]value.Value{}
	for _, a := range assignments {
		got[a.Name] = a.Value
	}
	if got["alpha"] != value.String("hello") {
		t.Errorf("alpha = %#v", got["alpha"])
	}
}

func TestEmitVariablesEmpty(t *testing.T) {
	if b := EmitVariables(nil); len(b) != 0 {
		t.Errorf("EmitVariables(nil) = %q, want empty", b)
	}
}
