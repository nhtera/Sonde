// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func TestProjectResolveHappyPath(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	vars, secrets, err := p.Resolve("local")
	if err != nil {
		t.Fatal(err)
	}
	// variables_files overrides the plain "variables" map entry of the
	// same name (docs/sonde-yaml.md precedence: variables, then
	// variables_files).
	if vars["base_url"] != value.String("http://localhost:9090") {
		t.Errorf("base_url = %#v, want the variables_files override", vars["base_url"])
	}
	if vars["retries"] != value.Int(3) {
		t.Errorf("retries = %#v, want the inline value 3", vars["retries"])
	}
	if vars["from_file"] != value.Int(1) {
		t.Errorf("from_file = %#v, want 1 from the variables file", vars["from_file"])
	}
	if secrets["token"] != "s3cr3t" {
		t.Errorf("secrets[token] = %q, want s3cr3t", secrets["token"])
	}
	if _, isVar := vars["token"]; isVar {
		t.Error("secrets_files entries must not leak into variables")
	}
}

func TestProjectResolveNoEnvSelected(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	vars, secrets, err := p.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 0 || len(secrets) != 0 {
		t.Errorf("Resolve(\"\") = %v, %v, want empty maps", vars, secrets)
	}
}

func TestProjectResolveUnknownEnv(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.Resolve("doesnotexist")
	if err == nil {
		t.Fatal("expected an error for an unknown environment")
	}
	if !strings.Contains(err.Error(), "local") || !strings.Contains(err.Error(), "staging") {
		t.Errorf("error = %q, want it to list the available environments", err)
	}
}

func TestSelectEnv(t *testing.T) {
	tests := []struct {
		name                           string
		flag, envVar, defaultEnv, want string
	}{
		{"flag wins over everything", "flag", "env", "default", "flag"},
		{"env var wins over default", "", "env", "default", "env"},
		{"default when nothing else set", "", "", "default", "default"},
		{"empty when nothing set at all", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SelectEnv(tt.flag, tt.envVar, tt.defaultEnv); got != tt.want {
				t.Errorf("SelectEnv(%q,%q,%q) = %q, want %q", tt.flag, tt.envVar, tt.defaultEnv, got, tt.want)
			}
		})
	}
}
