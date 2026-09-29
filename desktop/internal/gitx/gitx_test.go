// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/internal/sandbox"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repo is a repository whose own config and hooks would run programs:
// each leaves a marker file.
func repo(t *testing.T) (dir, markers string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if runtime.GOOS == "windows" {
		t.Skip("hook and filter scripts are POSIX shell")
	}
	dir, markers = t.TempDir(), t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "core.fsmonitor", "touch "+filepath.Join(markers, "fsmonitor")+"; true")
	git(t, dir, "config", "filter.evil.clean", "touch "+filepath.Join(markers, "filter")+"; cat")
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.hurl filter=evil\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+filepath.Join(markers, "hook")+"\n"), 0o700); err != nil { //nolint:gosec // an executable test hook
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.hurl"), []byte("GET https://x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, markers
}

func service(t *testing.T, dir string) *Service {
	t.Helper()
	root, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sandbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New(func() *sandbox.Root { return root }, cfg)
	t.Setenv("HOME", dir) // no user git config in the test
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	return s
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestUntrustedRunsNothing(t *testing.T) {
	dir, markers := repo(t)
	s := service(t, dir)
	info, err := s.Info()
	if err != nil || !info.Repo || info.Branch != "main" || info.Trusted {
		t.Fatalf("info %+v %v", info, err)
	}
	var e *apperr.Error
	if _, err := s.Status(t.Context()); !errors.As(err, &e) || e.Code != apperr.Denied {
		t.Errorf("untrusted status: %v", err)
	}
	if _, err := s.Commit(t.Context(), "m", []string{"a.hurl"}); !errors.As(err, &e) || e.Code != apperr.Denied {
		t.Errorf("untrusted commit: %v", err)
	}
	for _, m := range []string{"fsmonitor", "filter", "hook"} {
		if exists(filepath.Join(markers, m)) {
			t.Errorf("an untrusted repository ran its %s", m)
		}
	}
}

func TestTrustedStatusAndCommit(t *testing.T) {
	dir, markers := repo(t)
	s := service(t, dir)
	if err := s.Trust(); err != nil {
		t.Fatal(err)
	}
	st, err := s.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 2 || st[1].Path != "a.hurl" || st[1].Worktree != "?" {
		t.Errorf("status %+v", st)
	}
	if exists(filepath.Join(markers, "fsmonitor")) {
		t.Error("fsmonitor ran: it is always off")
	}
	hash, err := s.Commit(t.Context(), "add a", []string{"a.hurl"})
	if err != nil || len(hash) < 7 {
		t.Fatalf("commit %q %v", hash, err)
	}
	if !exists(filepath.Join(markers, "hook")) {
		t.Error("a trusted commit must run the user's hooks")
	}
	if _, err := s.Commit(t.Context(), "m", []string{"--amend"}); err == nil {
		t.Error("an option as a file")
	}
}

func TestBranchDetachedAndWorktreeFile(t *testing.T) {
	if got := branchOf([]byte("0123456789abcdef\n")); got != "0123456" {
		t.Errorf("detached: %q", got)
	}
	if got := branchOf([]byte("ref: refs/heads/feat/x\n")); got != "feat/x" {
		t.Errorf("branch: %q", got)
	}
	root := t.TempDir()
	gitdir := filepath.Join(root, "real")
	if err := os.MkdirAll(gitdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/wt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(root, "proj", "sub")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proj", ".git"), []byte("gitdir: ../real\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	head, err := headFile(proj)
	if err != nil || branchOf(head) != "wt" {
		t.Errorf("worktree: %q %v", head, err)
	}
}

func TestParseStatus(t *testing.T) {
	out := []byte(" M a.hurl\x00R  new.hurl\x00old.hurl\x00?? b c.hurl\x00")
	got := parseStatus(out)
	if len(got) != 3 || got[1].Path != "new.hurl" || got[1].Index != "R" || got[2].Path != "b c.hurl" {
		t.Errorf("%+v", got)
	}
}
