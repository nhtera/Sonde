// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package envsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/sandbox"
)

func TestIgnored(t *testing.T) {
	for _, c := range []struct {
		gitignore, rel string
		want           bool
	}{
		{"*.secrets\n", "secrets/local.secrets", true},
		{"secrets/\n", "secrets/local.secrets", true},
		{"/secrets\n", "secrets/local.secrets", true},
		{"**/secrets/\n", "a/secrets/local.secrets", true},
		{"secrets/*.secrets\n", "secrets/local.secrets", true},
		{"/secrets/local.secrets\n", "secrets/local.secrets", true},
		{"/secrets\n", "a/secrets/local.secrets", false},
		{"# *.secrets\n!secrets/\nnode_modules/\n", "secrets/local.secrets", false},
		{"", "secrets/local.secrets", false},
	} {
		if got := ignored([]byte(c.gitignore), c.rel); got != c.want {
			t.Errorf("%q covers %s: %v, want %v", c.gitignore, c.rel, got, c.want)
		}
	}
}

// TestSecretKeptOutOfGit: a secret written in a git repository whose
// .gitignore does not cover its file adds "*.secrets" there, once, and
// says so; outside a repository, or covered already, nothing is written.
func TestSecretKeptOutOfGit(t *testing.T) {
	setup := func(git bool, gitignore string) (*Envs, *emit.Recorder, string) {
		proj := t.TempDir()
		write(t, proj, "sonde.yaml", "version: 1\nenvironments:\n  local:\n", 0o644)
		if git {
			if err := os.Mkdir(filepath.Join(proj, ".git"), 0o750); err != nil {
				t.Fatal(err)
			}
		}
		if gitignore != "" {
			write(t, proj, ".gitignore", gitignore, 0o644)
		}
		root := sandboxtest.Open(t, proj)
		rec := &emit.Recorder{}
		return New(rec, sandboxtest.Open(t, t.TempDir()), func() *sandbox.Root { return root }, config.Env{}, "test", nil), rec, proj
	}
	gitignored := func(rec *emit.Recorder) int {
		n := 0
		for _, ev := range rec.Events() {
			if ev.Topic == TopicIgnored {
				n++
			}
		}
		return n
	}

	e, rec, proj := setup(true, "node_modules/")
	if err := e.SetSecret("local", "token", secretValue); err != nil {
		t.Fatal(err)
	}
	if err := e.SetSecret("local", "cookie", "c=1"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, proj, ".gitignore"); got != "node_modules/\n# secret values, kept out of git (Sonde)\n*.secrets\n" || gitignored(rec) != 1 {
		t.Errorf(".gitignore %q, %d events", got, gitignored(rec))
	}
	if !strings.Contains(read(t, proj, "sonde.yaml"), "secrets:\n      - token\n      - cookie\n") {
		t.Errorf("sonde.yaml:\n%s", read(t, proj, "sonde.yaml"))
	}

	e, rec, proj = setup(false, "")
	if err := e.SetSecret("local", "token", secretValue); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".gitignore")); err == nil || gitignored(rec) != 0 {
		t.Error("a .gitignore outside a repository")
	}

	e, rec, proj = setup(true, "secrets/\n")
	if err := e.SetSecret("local", "token", secretValue); err != nil {
		t.Fatal(err)
	}
	if read(t, proj, ".gitignore") != "secrets/\n" || gitignored(rec) != 0 {
		t.Error("a covering .gitignore is changed")
	}
}
