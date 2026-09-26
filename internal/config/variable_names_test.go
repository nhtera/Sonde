// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVariableNamesHappyPath(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	names, err := p.VariableNames("local")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]VariableSource{
		"retries":   {},
		"ratio":     {},
		"strict":    {},
		"extra":     {},
		"base_url":  {File: "env/local.vars"}, // variables_files overrides the inline entry
		"from_file": {File: "env/local.vars"},
		"token":     {File: "env/local.secrets", Secret: true},
	}
	if len(names) != len(want) {
		t.Fatalf("names = %+v, want %d entries", names, len(want))
	}
	for name, wantSrc := range want {
		got, ok := names[name]
		if !ok {
			t.Errorf("missing %q", name)
			continue
		}
		if got != wantSrc {
			t.Errorf("%s = %+v, want %+v", name, got, wantSrc)
		}
	}
}

func TestVariableNamesNoEnvSelected(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	names, err := p.VariableNames("")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("VariableNames(\"\") = %v, want empty", names)
	}
}

func TestVariableNamesUnknownEnv(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.VariableNames("doesnotexist"); err == nil {
		t.Fatal("expected an error for an unknown environment")
	} else if !strings.Contains(err.Error(), "local") || !strings.Contains(err.Error(), "staging") {
		t.Errorf("error = %q, want it to list the available environments", err)
	}
}

// TestVariableNamesNeverKeepsSecretValue guards the whole point of this
// helper: the secret's value must never appear anywhere in the result, only
// its name and file.
func TestVariableNamesNeverKeepsSecretValue(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	names, err := p.VariableNames("local")
	if err != nil {
		t.Fatal(err)
	}
	src, ok := names["token"]
	if !ok || !src.Secret {
		t.Fatalf("token = %+v, ok=%v, want a secret source", src, ok)
	}
	if src.File != "env/local.secrets" {
		t.Errorf("token file = %q", src.File)
	}
}

// TestVariableNamesPartialOnFileError checks that a missing or malformed
// variables_files/secrets_files entry reports its error without discarding
// the inline "variables:" names, or the names of any other file that could
// still be read (M2 in the phase 9 LSP review: a run-time error used to
// wipe out even a name that never touched the failing file).
func TestVariableNamesPartialOnFileError(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "version: 1\nenvironments:\n  dev:\n    variables:\n      host: a\n    variables_files:\n      - missing.vars\n      - present.vars\n")
	if err := os.WriteFile(filepath.Join(dir, "present.vars"), []byte("fromFile=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProject(filepath.Join(dir, ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}

	names, err := p.VariableNames("dev")
	if err == nil || !strings.Contains(err.Error(), "missing.vars") {
		t.Fatalf("err = %v, want it to name missing.vars", err)
	}
	if _, ok := names["host"]; !ok {
		t.Errorf("names = %+v, want the inline \"host\" kept despite the file error", names)
	}
	if _, ok := names["fromFile"]; !ok {
		t.Errorf("names = %+v, want present.vars' \"fromFile\" kept despite missing.vars failing first", names)
	}
}

// TestVariableNamesDuplicateSecret checks a name defined by two
// secrets_files entries is reported the same way AddSecret would fail a
// run, and that the duplicate doesn't stop the rest of the walk (L8).
func TestVariableNamesDuplicateSecret(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "version: 1\nenvironments:\n  dev:\n    secrets_files:\n      - a.secrets\n      - b.secrets\n")
	if err := os.WriteFile(filepath.Join(dir, "a.secrets"), []byte("tok=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.secrets"), []byte("tok=y\nother=z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProject(filepath.Join(dir, ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}

	names, err := p.VariableNames("dev")
	if err == nil || !strings.Contains(err.Error(), "can't be reassigned") {
		t.Fatalf("err = %v, want AddSecret's reassignment message", err)
	}
	if src, ok := names["tok"]; !ok || src.File != "a.secrets" {
		t.Errorf(`names["tok"] = %+v, ok=%v, want the first source kept`, src, ok)
	}
	if _, ok := names["other"]; !ok {
		t.Errorf("names = %+v, want b.secrets' \"other\" kept despite tok's clash", names)
	}
}
