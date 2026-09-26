// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildVariablesPrecedence(t *testing.T) {
	env := Env{
		"HURL_VARIABLE_foo":     "from-hurl-env",
		"SONDE_VARIABLE_foo":    "from-sonde-env",
		"HURL_VARIABLE_onlyenv": "42",
	}
	file := writeTemp(t, "vars.env", "foo=from-file\nfromfile=1\n")

	vars, err := BuildVariables(env, []string{file}, []string{"foo=from-flag"})
	if err != nil {
		t.Fatal(err)
	}
	// --variable wins over everything.
	if vars["foo"] != value.String("from-flag") {
		t.Errorf("foo = %#v, want from-flag", vars["foo"])
	}
	if vars["fromfile"] != value.Int(1) {
		t.Errorf("fromfile = %#v, want 1", vars["fromfile"])
	}
	if vars["onlyenv"] != value.Int(42) {
		t.Errorf("onlyenv = %#v, want 42", vars["onlyenv"])
	}
}

func TestBuildVariablesFileOverridesEnv(t *testing.T) {
	env := Env{"HURL_VARIABLE_foo": "1"}
	file := writeTemp(t, "vars.env", "foo=2\n")
	vars, err := BuildVariables(env, []string{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if vars["foo"] != value.Int(2) {
		t.Errorf("foo = %#v, want 2 (file overrides env)", vars["foo"])
	}
}

func TestBuildVariablesMultipleFilesInOrder(t *testing.T) {
	f1 := writeTemp(t, "a.env", "x=1\n")
	f2 := writeTemp(t, "b.env", "x=2\n")
	vars, err := BuildVariables(nil, []string{f1, f2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if vars["x"] != value.Int(2) {
		t.Errorf("x = %#v, want 2 (last file wins)", vars["x"])
	}
}

func TestBuildVariablesMissingFile(t *testing.T) {
	if _, err := BuildVariables(nil, []string{"/no/such/file"}, nil); err == nil {
		t.Fatal("expected an error for a missing variables file")
	}
}

func TestBuildVariablesBadFlag(t *testing.T) {
	if _, err := BuildVariables(nil, nil, []string{"noequals"}); err == nil {
		t.Fatal("expected an error for a variable with no '='")
	}
}

func TestBuildSecretsDuplicateErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     Env
		files   []string
		secrets []string
	}{
		{"duplicate flags", nil, nil, []string{"a=1", "a=2"}},
		{"env then flag", Env{"HURL_SECRET_a": "1"}, nil, []string{"a=2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildSecrets(tt.env, tt.files, tt.secrets); err == nil {
				t.Fatal("expected a reassignment error")
			}
		})
	}
}

func TestBuildSecretsPrecedence(t *testing.T) {
	env := Env{"HURL_SECRET_a": "from-env"}
	file := writeTemp(t, "secrets.env", "b=from-file\n")
	secrets, err := BuildSecrets(env, []string{file}, []string{"c=from-flag"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a": "from-env", "b": "from-file", "c": "from-flag"}
	for k, v := range want {
		if secrets[k] != v {
			t.Errorf("secrets[%q] = %q, want %q", k, secrets[k], v)
		}
	}
}

func TestBuildSecretsForcesString(t *testing.T) {
	secrets, err := BuildSecrets(nil, nil, []string{"n=30"})
	if err != nil {
		t.Fatal(err)
	}
	if secrets["n"] != "30" {
		t.Errorf("secrets[n] = %q, want the literal string 30", secrets["n"])
	}
}

func TestBuildSecretsMissingFile(t *testing.T) {
	if _, err := BuildSecrets(nil, []string{"/no/such/file"}, nil); err == nil {
		t.Fatal("expected an error for a missing secrets file")
	}
}

func TestBuildSecretsBadFile(t *testing.T) {
	file := writeTemp(t, "secrets.env", "noequals\n")
	if _, err := BuildSecrets(nil, []string{file}, nil); err == nil {
		t.Fatal("expected an error for a malformed secrets file")
	}
}

func TestBuildSecretsFileDuplicate(t *testing.T) {
	file := writeTemp(t, "secrets.env", "a=1\na=2\n")
	if _, err := BuildSecrets(nil, []string{file}, nil); err == nil {
		t.Fatal("expected a reassignment error within the file")
	}
}

func TestBuildVariablesEnvError(t *testing.T) {
	env := Env{"HURL_VARIABLE_foo": `"unterminated`}
	if _, err := BuildVariables(env, nil, nil); err == nil {
		t.Fatal("expected an error for a malformed env value")
	}
}

func TestReadPropertiesFileUnreadable(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "secrets")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := readPropertiesFile(sub); err == nil {
		t.Fatal("expected an error reading a directory")
	}
}
