// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

const editYAML = `# Project settings
version: 1
defaults:
  env: dev # the default
environments:
  dev:
    variables:
      host: dev.test # comment kept
      port: 8080
      quoted: 'it''s'
      base: &base http://x
    variables_files:
      - dev.properties
  prod:
    variables:
      host: prod.test
`

// project writes a project and loads it.
func project(t *testing.T, yamlSrc string, files map[string]string) *Project {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(yamlSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProject(filepath.Join(dir, "sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// only returns the one edit of file base.
func only(t *testing.T, edits []FileEdit, base string) FileEdit {
	t.Helper()
	for _, e := range edits {
		if filepath.Base(e.Path) == base {
			return e
		}
	}
	t.Fatalf("no edit of %s in %d edits", base, len(edits))
	return FileEdit{}
}

func TestSetVariableInline(t *testing.T) {
	p := project(t, editYAML, map[string]string{"dev.properties": "# dev\ntoken_ttl=60\n"})
	for _, tc := range []struct {
		name string
		v    value.Value
		want string // the line of the result
	}{
		{"host", value.String("new.test"), "      host: new.test # comment kept\n"},
		{"host", value.String("true"), "      host: \"true\" # comment kept\n"},
		{"host", value.String("a: b"), "      host: \"a: b\" # comment kept\n"},
		{"port", value.Int(9090), "      port: 9090\n"},
		{"port", value.String("9090"), "      port: \"9090\"\n"},
		{"quoted", value.String("o'k"), "      quoted: 'o''k'\n"},
		{"new", value.Bool(true), "      base: &base http://x\n      new: true\n    variables_files:"},
		{"ünï", value.Float(1.5), "      ünï: 1.5\n"},
	} {
		edits, err := p.SetVariable("dev", tc.name, tc.v)
		if err != nil {
			t.Fatalf("%s=%v: %v", tc.name, tc.v, err)
		}
		e := only(t, edits, "sonde.yaml")
		if !strings.Contains(string(e.Data), tc.want) || e.Created {
			t.Errorf("%s=%v:\n%s", tc.name, tc.v, e.Data)
		}
		// Every other line is unchanged.
		if got := strings.Count(string(e.Data), "\n") - strings.Count(editYAML, "\n"); got > 1 {
			t.Errorf("%s: %d lines added", tc.name, got)
		}
		if !strings.Contains(string(e.Data), "# Project settings\nversion: 1\ndefaults:\n  env: dev # the default\n") {
			t.Errorf("%s: header changed:\n%s", tc.name, e.Data)
		}
	}
	for name, v := range map[string]value.Value{"base": value.String("y"), "host": value.String("a\nb")} {
		if _, err := p.SetVariable("dev", name, v); !errors.Is(err, ErrEditByHand) {
			t.Errorf("%s: %v, want ErrEditByHand", name, err)
		}
	}
	if _, err := p.SetVariable("nope", "x", value.Int(1)); err == nil {
		t.Error("unknown environment accepted")
	}
}

func TestSetVariableSources(t *testing.T) {
	p := project(t, editYAML+"    secrets_files: [prod.secrets]\n", map[string]string{ //nolint:gosec // G101: test fixture
		"dev.properties": "# dev\r\ntoken_ttl=60\r\nhost=from-file\r\n",
		"prod.secrets":   "api_key=k1\n",
	})
	// host: the variables file wins over the inline value.
	edits, err := p.SetVariable("dev", "host", value.String("file.test"))
	if err != nil {
		t.Fatal(err)
	}
	if e := only(t, edits, "dev.properties"); string(e.Data) != "# dev\r\ntoken_ttl=60\r\nhost=file.test\r\n" || len(edits) != 1 {
		t.Errorf("variables file:\n%q", e.Data)
	}
	if edits, err = p.SetVariable("dev", "token_ttl", value.String("60")); err != nil {
		t.Fatal(err)
	}
	if e := only(t, edits, "dev.properties"); !strings.Contains(string(e.Data), "token_ttl=\"60\"\r\n") {
		t.Errorf("typed string:\n%q", e.Data)
	}
	// A secret stays one, in its file.
	if edits, err = p.SetVariable("prod", "api_key", value.String("k2")); err != nil {
		t.Fatal(err)
	}
	if e := only(t, edits, "prod.secrets"); string(e.Data) != "api_key=k2\n" {
		t.Errorf("secrets file:\n%q", e.Data)
	}
}

func TestSetSecret(t *testing.T) {
	p := project(t, editYAML, map[string]string{"dev.properties": "port=1\n"})
	edits, err := p.SetSecret("dev", "port", "s3cr3t")
	if err != nil {
		t.Fatal(err)
	}
	sec := only(t, edits, "dev.secrets")
	if string(sec.Data) != "port=s3cr3t\n" || !sec.Created || sec.Perm != 0o600 || !strings.HasSuffix(filepath.ToSlash(sec.Path), "env/dev.secrets") {
		t.Errorf("secrets file %+v", sec)
	}
	y := string(only(t, edits, "sonde.yaml").Data)
	if strings.Contains(y, "port: 8080") || !strings.Contains(y, "    secrets_files:\n      - env/dev.secrets\n") {
		t.Errorf("sonde.yaml:\n%s", y)
	}
	if string(only(t, edits, "dev.properties").Data) != "" {
		t.Error("the variables file still defines the name")
	}

	// An existing block list gets one more item.
	p = project(t, strings.Replace(editYAML, "  prod:\n", "    secrets_files:\n      - other.secrets\n  prod:\n", 1), map[string]string{"dev.properties": "", "other.secrets": "a=1\n"})
	if edits, err = p.SetSecret("dev", "b", "2"); err != nil {
		t.Fatal(err)
	}
	if y := string(only(t, edits, "sonde.yaml").Data); !strings.Contains(y, "      - other.secrets\n      - env/dev.secrets\n  prod:") {
		t.Errorf("block list:\n%s", y)
	}
	// A secret defined twice is refused by the validation.
	p = project(t, strings.Replace(editYAML, "  prod:\n", "    secrets_files: [a.secrets, env/dev.secrets]\n  prod:\n", 1), map[string]string{
		"dev.properties": "", "a.secrets": "x=1\n", "env/dev.secrets": "y=2\n",
	})
	if edits, err = p.SetSecret("dev", "z", "3"); err != nil {
		t.Fatal(err)
	}
	if e := only(t, edits, "dev.secrets"); string(e.Data) != "y=2\nz=3\n" || e.Created || len(edits) != 1 {
		t.Errorf("existing secrets file: %+v (%d edits)", e, len(edits))
	}
	if _, err := p.SetSecret("dev", "q", "multi\nline"); err == nil {
		t.Error("a multi-line secret is accepted")
	}
}

func TestRemoveVariable(t *testing.T) {
	p := project(t, editYAML, map[string]string{"dev.properties": "host=1\nkeep=2\n"})
	edits, err := p.RemoveVariable("dev", "host")
	if err != nil {
		t.Fatal(err)
	}
	if y := string(only(t, edits, "sonde.yaml").Data); strings.Contains(y, "dev.test") || !strings.Contains(y, "prod.test") {
		t.Errorf("sonde.yaml:\n%s", y)
	}
	if f := string(only(t, edits, "dev.properties").Data); f != "keep=2\n" {
		t.Errorf("variables file %q", f)
	}
}

func TestEditRefusesFlowStyle(t *testing.T) {
	p := project(t, "version: 1\nenvironments:\n  dev: {variables: {a: 1}}\n", nil)
	if _, err := p.SetVariable("dev", "a", value.Int(2)); !errors.Is(err, ErrEditByHand) {
		t.Errorf("flow style: %v", err)
	}
}

func TestEditCRLFAndEmptyEnv(t *testing.T) {
	src := strings.ReplaceAll("version: 1\nenvironments:\n  dev:\n  prod:\n    variables:\n      a: 1\n", "\n", "\r\n")
	p := project(t, src, nil)
	edits, err := p.SetVariable("dev", "x", value.String("y"))
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 1\r\nenvironments:\r\n  dev:\r\n    variables:\r\n      x: y\r\n  prod:\r\n"
	if y := string(only(t, edits, "sonde.yaml").Data); !strings.HasPrefix(y, want) {
		t.Errorf("sonde.yaml:\n%q", y)
	}
}

func TestProperties(t *testing.T) {
	for _, tc := range []struct{ in, name, raw, want string }{
		{"", "a", "1", "a=1\n"},
		{"# c\na=1\nb=2", "a", "3", "# c\na=3\nb=2"},
		{"a=1\na=2\n", "a", "3", "a=1\na=3\n"},
		{"b=2", "a", "1", "b=2\na=1\n"},
		{"b=2\r\n", "a", "1", "b=2\r\na=1\r\n"},
	} {
		got, err := SetProperty([]byte(tc.in), tc.name, tc.raw)
		if err != nil || string(got) != tc.want {
			t.Errorf("SetProperty(%q, %s=%s) = %q, %v", tc.in, tc.name, tc.raw, got, err)
		}
	}
	for _, bad := range [][2]string{{"a=b", "1"}, {"a", "x\ny"}, {"#a", "1"}, {"newUuid", "1"}, {"a", " 1"}} {
		if _, err := SetProperty(nil, bad[0], bad[1]); err == nil {
			t.Errorf("SetProperty(%q, %q) accepted", bad[0], bad[1])
		}
	}
	if got := string(RemoveProperty([]byte("a=1\n# a=2\nb=3\na=4"), "a")); got != "# a=2\nb=3\n" {
		t.Errorf("RemoveProperty = %q", got)
	}
	for v, want := range map[value.Value]string{
		value.String("x"): "x", value.String("true"): `"true"`, value.String("1.5"): `"1.5"`,
		value.String(`"q"`): `""q""`, value.Int(3): "3", value.Bool(false): "false", value.Null{}: "null", value.Float(2): "2.0",
	} {
		got, err := PropertyText(v)
		if err != nil || got != want {
			t.Errorf("PropertyText(%v) = %q, %v, want %q", v, got, err, want)
		}
		if back, err := InferValue(got); err != nil || !value.Equal(back, v) || back.Kind() != v.Kind() {
			t.Errorf("%q reads back as %v", got, back)
		}
	}
}

// TestEditValidation refuses an edit that breaks the project, and not one
// of an environment that was already broken.
func TestEditValidation(t *testing.T) {
	p := project(t, editYAML+"    secrets_files: [missing.secrets]\n", map[string]string{"dev.properties": ""})
	e := p.newEdit()
	if err := e.write("sonde.yaml", []byte("version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.result(); err == nil {
		t.Error("a broken sonde.yaml is accepted")
	}
	e = p.newEdit()
	if err := e.write("dev.properties", []byte("no assignment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.result(); err == nil {
		t.Error("a broken variables file is accepted")
	}
	// prod does not resolve (its secrets file is missing): dev still edits.
	if _, err := p.SetVariable("dev", "x", value.Int(1)); err != nil {
		t.Errorf("an already broken environment blocks the edit: %v", err)
	}
}
