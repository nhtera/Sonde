// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/sandbox"
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
	if string(sec.Data) != "port=s3cr3t\n" || !sec.Created || sec.Perm != 0o600 || !strings.HasSuffix(filepath.ToSlash(sec.Path), "secrets/dev.secrets") {
		t.Errorf("secrets file %+v", sec)
	}
	y := string(only(t, edits, "sonde.yaml").Data)
	if strings.Contains(y, "port: 8080") || !strings.Contains(y, "    secrets_files:\n      - secrets/dev.secrets\n    secrets:\n      - port\n") {
		t.Errorf("sonde.yaml:\n%s", y)
	}
	if string(only(t, edits, "dev.properties").Data) != "" {
		t.Error("the variables file still defines the name")
	}

	// A listed secrets file is used, even when it is not checked out.
	p = project(t, strings.Replace(editYAML, "  prod:\n", "    secrets_files:\n      - other.secrets\n  prod:\n", 1), map[string]string{"dev.properties": ""})
	if edits, err = p.SetSecret("dev", "b", "2"); err != nil {
		t.Fatal(err)
	}
	// Its name is listed in secrets: (the yaml edit), for a fresh clone.
	if e := only(t, edits, "other.secrets"); string(e.Data) != "b=2\n" || !e.Created || len(edits) != 2 || !strings.Contains(string(only(t, edits, "sonde.yaml").Data), "    secrets:\n      - b\n") {
		t.Errorf("listed secrets file: %+v (%d edits)", e, len(edits))
	}
	if _, err := p.SetVariable("dev", "c", value.Int(3)); err != nil {
		t.Errorf("a missing secrets file blocks SetVariable: %v", err)
	}
	// An existing secrets file gets the new secret.
	p = project(t, strings.Replace(editYAML, "  prod:\n", "    secrets_files: [a.secrets, env/dev.secrets]\n  prod:\n", 1), map[string]string{
		"dev.properties": "", "a.secrets": "x=1\n", "env/dev.secrets": "y=2\n",
	})
	if edits, err = p.SetSecret("dev", "z", "3"); err != nil {
		t.Fatal(err)
	}
	if e := only(t, edits, "dev.secrets"); string(e.Data) != "y=2\nz=3\n" || e.Created || len(edits) != 2 {
		t.Errorf("existing secrets file: %+v (%d edits)", e, len(edits))
	}
	if _, err := p.SetSecret("dev", "q", "multi\nline"); err == nil {
		t.Error("a multi-line secret is accepted")
	}
}

// TestEditAfterMultiLineValues adds a variable after last values that
// span lines: the new line goes after them, never inside.
func TestEditAfterMultiLineValues(t *testing.T) {
	for name, last := range map[string]string{
		"folded":  "      desc: >\n        long text\n        continues\n",
		"literal": "      desc: |\n        line one\n\n        line three\n",
		"double":  "      desc: \"foo\n        bar\"\n",
		"plain":   "      desc: foo\n        bar\n",
		"nested":  "      desc: x # c\n      # trailing comment\n",
	} {
		src := "version: 1\nenvironments:\n  dev:\n    variables:\n      a: 1\n" + last + "  prod:\n    variables:\n      a: 2\n"
		p := project(t, src, nil)
		edits, err := p.SetVariable("dev", "new", value.String("y"))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		got, err := loadProjectData(p.Path, edits[0].Data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		vars := got.Environments["dev"].Variables
		if !value.Equal(vars["new"], value.String("y")) || !value.Equal(vars["desc"], p.Environments["dev"].Variables["desc"]) {
			t.Errorf("%s:\n%s", name, edits[0].Data)
		}
	}
}

// TestWriteEdits keeps an existing file's mode, gives a new one its Perm,
// and refuses a symbolic link.
func TestWriteEdits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes and links")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = WriteEdits(root, []FileEdit{
		{Path: filepath.Join(dir, "sonde.yaml"), Data: []byte("y"), Perm: 0o644},
		{Path: filepath.Join(dir, "secrets", "dev.secrets"), Data: []byte("k=v\n"), Perm: 0o600, Created: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{"sonde.yaml": 0o600, "secrets/dev.secrets": 0o600} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v %v", name, fi.Mode().Perm(), err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "sonde.yaml"), filepath.Join(dir, "link.properties")); err != nil {
		t.Fatal(err)
	}
	if err := WriteEdits(root, []FileEdit{{Path: filepath.Join(dir, "link.properties"), Data: []byte("z")}}); err == nil {
		t.Error("a symbolic link is replaced")
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

// TestAddEnvironments: an import's environments go after the last one,
// the file otherwise as it was, and read back as added.
func TestAddEnvironments(t *testing.T) {
	add := map[string]EnvironmentSkeleton{
		"collection": {Variables: map[string]string{"baseUrl": "http://x", "port": "8080"}, SecretsFiles: []string{"secrets/collection.secrets"}},
	}
	cases := []struct{ name, src, want string }{
		{"after the last", editYAML + "openapi:\n  spec: api.yaml\n", editYAML +
			"  collection:\n    variables:\n      baseUrl: http://x\n      port: \"8080\"\n    secrets_files:\n      - secrets/collection.secrets\n" +
			"openapi:\n  spec: api.yaml\n"},
		{"no environments", "version: 1 # v", "version: 1 # v\nenvironments:\n  collection:\n    variables:\n" +
			"      baseUrl: http://x\n      port: \"8080\"\n    secrets_files:\n      - secrets/collection.secrets\n"},
		{"an empty one", "version: 1\nenvironments: ~ # none yet\n", "version: 1\nenvironments: # none yet\n  collection:\n    variables:\n" +
			"      baseUrl: http://x\n      port: \"8080\"\n    secrets_files:\n      - secrets/collection.secrets\n"},
		{"CRLF", "version: 1\r\nenvironments:\r\n  dev:\r\n", "version: 1\r\nenvironments:\r\n  dev:\r\n  collection:\r\n    variables:\r\n" +
			"      baseUrl: http://x\r\n      port: \"8080\"\r\n    secrets_files:\r\n      - secrets/collection.secrets\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := project(t, c.src, map[string]string{"dev.properties": "a=1\n"})
			edits, err := p.AddEnvironments(add)
			if err != nil {
				t.Fatal(err)
			}
			if len(edits) != 1 {
				t.Fatalf("%d edits", len(edits))
			}
			if got := string(only(t, edits, "sonde.yaml").Data); got != c.want {
				t.Errorf("sonde.yaml:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
	// An empty one (a collection without variables) is "name:", which
	// edits; one written "{}" already does not keep others out.
	p := project(t, "version: 1\nenvironments:\n  dev: {}\n", nil)
	edits, err := p.AddEnvironments(map[string]EnvironmentSkeleton{"collection": {}, "local": add["collection"]})
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 1\nenvironments:\n  dev: {}\n  collection:\n  local:\n    variables:\n      baseUrl: http://x\n      port: \"8080\"\n" +
		"    secrets_files:\n      - secrets/collection.secrets\n"
	if got := string(only(t, edits, "sonde.yaml").Data); got != want {
		t.Errorf("sonde.yaml:\n%s\nwant:\n%s", got, want)
	}
	p = project(t, editYAML, map[string]string{"dev.properties": "a=1\n"})
	if _, err := p.AddEnvironments(map[string]EnvironmentSkeleton{"dev": {}}); err == nil {
		t.Error("an environment there already is added")
	}
	p = project(t, "version: 1\nenvironments: {dev: {}}\n", nil)
	if _, err := p.AddEnvironments(add); !errors.Is(err, ErrEditByHand) {
		t.Errorf("flow style: %v", err)
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

// TestSecretsListed: a secret set is listed in "secrets:" once, in block
// or flow style; removed, it leaves the list ("[]" when it was the last).
func TestSecretsListed(t *testing.T) {
	for _, c := range []struct{ name, src, after, removed string }{
		{"block", "version: 1\nenvironments:\n  dev:\n    secrets:\n      - a\n", "    secrets:\n      - a\n      - b\n", "    secrets:\n      - a\n"},
		{"flow", "version: 1\nenvironments:\n  dev:\n    secrets: [a]\n", "    secrets: [a, b]\n", "    secrets: [a]\n"},
		{"none", "version: 1\nenvironments:\n  dev:\n    variables:\n      x: 1\n", "    secrets:\n      - b\n", "    secrets: []\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := project(t, c.src, nil)
			edits, err := p.SetSecret("dev", "b", "v")
			if err != nil {
				t.Fatal(err)
			}
			if err := WriteEdits(sandboxOpen(t, p.Dir), edits); err != nil {
				t.Fatal(err)
			}
			if p, err = LoadProject(p.Path); err != nil {
				t.Fatal(err)
			}
			if y, _ := os.ReadFile(p.Path); !strings.Contains(string(y), c.after) {
				t.Fatalf("set:\n%s", y)
			}
			if edits, err = p.SetSecret("dev", "b", "w"); err != nil || strings.Contains(string(only(t, edits, "dev.secrets").Data), "b=v") {
				t.Fatalf("set again: %v", err)
			}
			for _, ed := range edits {
				if filepath.Base(ed.Path) == "sonde.yaml" {
					t.Error("listed twice")
				}
			}
			if edits, err = p.RemoveVariable("dev", "b"); err != nil {
				t.Fatal(err)
			}
			if y := string(only(t, edits, "sonde.yaml").Data); !strings.Contains(y, c.removed) || strings.Contains(y, "- b") {
				t.Errorf("removed:\n%s", y)
			}
		})
	}
}

// sandboxOpen opens dir as a Root closed with the test.
func sandboxOpen(t *testing.T, dir string) *sandbox.Root {
	t.Helper()
	root, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// TestMergeEnvironments: an import's values go into a picked environment:
// what it defines is kept (a variable, a secret, one listed), new names
// are added, an earlier merge winning; new environments are added too.
func TestMergeEnvironments(t *testing.T) {
	src := "version: 1\nenvironments:\n  local:\n    variables:\n      base_url: http://mine\n    secrets: [token]\n"
	p := project(t, src, nil)
	edits, kept, err := p.MergeEnvironments([]EnvMerge{
		{Env: "local", Variables: map[string]string{"policyId": "p-2", "base_url": "http://theirs"}, Secrets: []string{"nmk-cookie", "token"}},
		{Env: "local", Variables: map[string]string{"policyId": "p-1", "port": "8080"}},
	}, map[string]EnvironmentSkeleton{"partner": {Variables: map[string]string{"x": "1"}, Secrets: []string{"key"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 1\nenvironments:\n  local:\n    variables:\n      base_url: http://mine\n      policyId: p-2\n      port: \"8080\"\n" +
		"    secrets: [token, nmk-cookie]\n  partner:\n    variables:\n      x: \"1\"\n    secrets:\n      - key\n"
	if got := string(only(t, edits, "sonde.yaml").Data); got != want {
		t.Errorf("sonde.yaml:\n%s\nwant:\n%s", got, want)
	}
	if got := strings.Join(kept["local"], ","); got != "base_url,policyId,token" {
		t.Errorf("kept %v", kept)
	}
	if _, _, err := p.MergeEnvironments([]EnvMerge{{Env: "nope"}}, nil); err == nil {
		t.Error("an unknown environment is merged into")
	}
}
