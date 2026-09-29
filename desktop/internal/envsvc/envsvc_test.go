// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package envsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/sandbox"
)

const secretValue = "env-secret-sentinel-3"  //nolint:gosec // G101: test sentinel
const tokenValue = "plain-becomes-secret-99" //nolint:gosec // G101: test sentinel

const yaml = `version: 1
# the project's environments
environments:
  local:
    variables:
      base_url: http://localhost:8080
      retries: 3
      token: ` + tokenValue + `
    secrets_files:
      - secrets/local.secrets
  staging:
    variables:
      base_url: https://staging.example
defaults:
  env: local
`

func project(t *testing.T, dirs ...string) (*Envs, string, string) {
	t.Helper()
	proj, cfg := t.TempDir(), t.TempDir()
	if len(dirs) == 2 {
		proj, cfg = dirs[0], dirs[1]
	} else {
		write(t, proj, "sonde.yaml", yaml, 0o644)
		write(t, proj, "secrets/local.secrets", "api_key="+secretValue+"\n", 0o600)
	}
	root, err := sandbox.Open(proj)
	if err != nil {
		t.Fatal(err)
	}
	appCfg, err := sandbox.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	return New(&emit.Recorder{}, appCfg, func() *sandbox.Root { return root }, config.Env{"HOME": home, "XDG_CONFIG_HOME": home}, "test", nil), proj, cfg
}

func write(t *testing.T, dir, rel, text string, perm os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), perm); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return "<missing>"
	}
	return string(data)
}

func TestListHidesSecrets(t *testing.T) {
	e, _, _ := project(t)
	p, err := e.List()
	if err != nil {
		t.Fatal(err)
	}
	redactcheck.AssertNoSecret(t, "env list", p, secretValue)
	if len(p.Envs) != 2 || p.Envs[0].Name != "local" || !p.Envs[0].Default {
		t.Fatalf("envs %+v", p.Envs)
	}
	var key, retries *Var
	for i, v := range p.Envs[0].Variables {
		switch v.Name {
		case "api_key":
			key = &p.Envs[0].Variables[i]
		case "retries":
			retries = &p.Envs[0].Variables[i]
		}
	}
	if key == nil || !key.Secret || key.Value != "" || key.Source != "secrets/local.secrets" {
		t.Errorf("api_key %+v", key)
	}
	if retries == nil || retries.Value != "3" || retries.Type != "integer" || retries.Source != "sonde.yaml" {
		t.Errorf("retries %+v", retries)
	}
}

func TestSetVariableKeepsTypeAndComments(t *testing.T) {
	e, proj, _ := project(t)
	if err := e.SetVariable("local", "retries", json.RawMessage(`5`)); err != nil {
		t.Fatal(err)
	}
	if err := e.SetVariable("local", "debug", json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	got := read(t, proj, "sonde.yaml")
	if !strings.Contains(got, "retries: 5") || !strings.Contains(got, "debug: true") || !strings.Contains(got, "# the project's environments") {
		t.Errorf("sonde.yaml:\n%s", got)
	}
	var ae *apperr.Error
	if err := e.SetVariable("local", "x", json.RawMessage(`"a\nb"`)); !errors.As(err, &ae) || ae.Code != apperr.Invalid {
		t.Errorf("multi-line: %v", err)
	}
	if err := e.SetVariable("nope", "x", json.RawMessage(`1`)); err == nil {
		t.Error("unknown env")
	}
}

func TestMarkSecret(t *testing.T) {
	e, proj, _ := project(t)
	if err := e.MarkSecret("local", "token"); err != nil {
		t.Fatal(err)
	}
	if y := read(t, proj, "sonde.yaml"); strings.Contains(y, tokenValue) {
		t.Errorf("value left in sonde.yaml:\n%s", y)
	}
	if s := read(t, proj, "secrets/local.secrets"); !strings.Contains(s, "token="+tokenValue) {
		t.Errorf("secrets file:\n%s", s)
	}
	if fi, _ := os.Stat(filepath.Join(proj, "secrets", "local.secrets")); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("secrets mode %v", fi.Mode().Perm())
	}
	p, _ := e.List()
	redactcheck.AssertNoSecret(t, "env list", p, tokenValue, secretValue)
	// staging lists no secrets file: one is created and referenced.
	if err := e.SetVariable("staging", "pw", json.RawMessage(`"`+secretValue+`x"`)); err != nil {
		t.Fatal(err)
	}
	if err := e.MarkSecret("staging", "pw"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, proj, "sonde.yaml"), "secrets/staging.secrets") {
		t.Error("new secrets file not referenced")
	}
}

func TestOverrides(t *testing.T) {
	e, _, _ := project(t)
	e.settings = func(inv *runplan.Invocation) {
		inv.Proxy = "http://p:1"
		if inv.Set == nil {
			inv.Set = map[string]bool{}
		}
		inv.Set["proxy"] = true
	}
	d0 := e.Digest()
	if err := e.SetOverride("local", "user_id", "42"); err != nil {
		t.Fatal(err)
	}
	var ae *apperr.Error
	if err := e.SetOverride("local", "api_key", "x"); !errors.As(err, &ae) || ae.Code != apperr.Invalid {
		t.Errorf("override of a secret: %v", err)
	}
	e.SetMock("http://127.0.0.1:9999")
	o := e.Overrides()
	if o.Count != 3 || o.Items[0].Flag != "--proxy" || o.Items[1].Source != "mock" || o.Items[2].Name != "user_id" {
		t.Errorf("overrides %+v", o)
	}
	var inv runplan.Invocation
	e.Extend(&inv)
	if strings.Join(inv.Variables, ",") != "base_url=http://127.0.0.1:9999,user_id=42" || inv.Proxy != "http://p:1" {
		t.Errorf("invocation %+v", inv)
	}
	if e.Digest() == d0 {
		t.Error("the digest must change with the overrides")
	}
	e.RemoveOverride("user_id")
	e.SetMock("")
	if o := e.Overrides(); o.Count != 1 {
		t.Errorf("after removal %+v", o)
	}
}

// TestMarkSecretKill kills the app at each write point of Mark secret: the
// next start finds the project wholly before or wholly after the edit.
func TestMarkSecretKill(t *testing.T) {
	if os.Getenv("ENVSVC_KILL_AT") != "" {
		killChild()
		return
	}
	for n := 0; ; n++ {
		e, proj, cfg := project(t)
		before := snapshot(t, proj)
		cmd := exec.Command(os.Args[0], "-test.run=^TestMarkSecretKill$") //nolint:gosec // G204: the test binary itself
		cmd.Env = append(os.Environ(), "ENVSVC_KILL_AT="+strconv.Itoa(n), "ENVSVC_PROJ="+proj, "ENVSVC_CFG="+cfg)
		out, err := cmd.CombinedOutput()
		exited := err != nil
		if _, err := e.List(); err != nil { // recovers
			t.Fatalf("kill at %d: %v", n, err)
		}
		after := snapshot(t, proj)
		if !exited {
			// The edit completed: the last write point was passed.
			if after == before || !strings.Contains(after, "token="+tokenValue) {
				t.Fatalf("finished edit at %d:\n%s\n%s", n, after, out)
			}
			if n < 3 {
				t.Fatalf("only %d write points", n)
			}
			return
		}
		if after != before {
			t.Errorf("killed at write point %d: the project is neither before nor after:\n%s", n, after)
		}
	}
}

func killChild() {
	n, _ := strconv.Atoi(os.Getenv("ENVSVC_KILL_AT"))
	hook = func(step int) {
		if step == n {
			os.Exit(137) // no deferred work runs, as with SIGKILL
		}
	}
	root, _ := sandbox.Open(os.Getenv("ENVSVC_PROJ"))
	cfg, _ := sandbox.Open(os.Getenv("ENVSVC_CFG"))
	e := New(&emit.Recorder{}, cfg, func() *sandbox.Root { return root }, config.Env{}, "test", nil)
	if err := e.MarkSecret("local", "token"); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
}

// snapshot is every file of the project, for comparison.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			data, _ := os.ReadFile(p) //nolint:gosec // test files
			rel, _ := filepath.Rel(dir, p)
			fmt.Fprintf(&b, "== %s\n%s\n", filepath.ToSlash(rel), data)
		}
		return nil
	})
	return b.String()
}
