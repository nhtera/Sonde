// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package gitx shows the project's git branch and status and commits
// request files. A project is untrusted until the user trusts its folder:
// a repository's own config can make git run programs (filter drivers on
// status and add, hooks on commit, fsmonitor anywhere). So:
//
//   - the branch is read from .git/HEAD without running git;
//   - status and commit need the folder trusted;
//   - every git run disables fsmonitor and hooks, ignores the system
//     config and never prompts; a commit in a trusted folder runs the
//     user's hooks.
package gitx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Timeout bounds every git run.
const Timeout = 30 * time.Second

// trustFile keeps the trusted folders in the app's config folder.
const trustFile = "trust.json"

// FileStatus is a changed file (porcelain v1 codes).
type FileStatus struct {
	Path     string `json:"path"`
	Index    string `json:"index"`    // staged change: M, A, D, R, ?…
	Worktree string `json:"worktree"` // unstaged change
	// Secret: the file holds secrets (Commit refuses it).
	Secret bool `json:"secret,omitempty"`
}

// Info is the project's git state.
type Info struct {
	// Repo tells whether the project is in a git repository.
	Repo    bool   `json:"repo"`
	Branch  string `json:"branch"`
	Trusted bool   `json:"trusted"`
	// Git tells whether a git program was found.
	Git bool `json:"git"`
}

// Service is the git bindings.
type Service struct {
	project func() *sandbox.Root
	config  *sandbox.Root
	git     string // the git program; "" when absent
	// Secret reports whether a project path is a secrets file of the
	// project's config (*.secrets files are, always): never committed.
	Secret func(rel string) bool

	mu sync.Mutex
}

// New returns the git service for the project project returns; config is
// the app's config folder (trusted folders).
func New(project func() *sandbox.Root, config *sandbox.Root) *Service {
	git, _ := exec.LookPath("git")
	return &Service{project: project, config: config, git: git}
}

func (s *Service) dir() (string, error) {
	r := s.project()
	if r == nil {
		return "", apperr.New(apperr.NotFound, "no project is open")
	}
	return r.Dir(), nil
}

// Info returns the branch and whether the folder is trusted.
func (s *Service) Info() (*Info, error) {
	dir, err := s.dir()
	if err != nil {
		return nil, err
	}
	info := &Info{Trusted: s.trusted(dir), Git: s.git != ""}
	head, err := headFile(dir)
	if err != nil {
		return info, nil //nolint:nilerr // not a repository
	}
	info.Repo = true
	info.Branch = branchOf(head)
	return info, nil
}

// Trust trusts the project folder: git may then run its config (status)
// and its hooks (commit).
func (s *Service) Trust() error {
	dir, err := s.dir()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.trustedList()
	if !slices.Contains(list, dir) {
		list = append(list, dir)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return s.config.WriteFileAtomic(trustFile, data, 0o600)
}

func (s *Service) trustedList() []string {
	var list []string
	if data, err := s.config.ReadFile(trustFile); err == nil {
		_ = json.Unmarshal(data, &list)
	}
	return list
}

func (s *Service) trusted(dir string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.trustedList(), dir)
}

// Status lists the changed files (trusted folders only).
func (s *Service) Status(ctx context.Context) ([]FileStatus, error) {
	dir, err := s.trustedDir()
	if err != nil {
		return nil, err
	}
	out, err := s.run(ctx, dir, false, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	list := parseStatus(out)
	for i := range list {
		list[i].Secret = s.IsSecret(list[i].Path)
	}
	return list, nil
}

// Commit commits files (project paths) with message and returns the new
// commit's short hash (trusted folders only; the user's hooks run).
func (s *Service) Commit(ctx context.Context, message string, files []string) (string, error) {
	dir, err := s.trustedDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" || len(files) == 0 {
		return "", apperr.New(apperr.Invalid, "a commit needs a message and files")
	}
	for _, f := range files {
		if f == "" || strings.HasPrefix(f, "-") || strings.Contains(f, "..") || filepath.IsAbs(f) || strings.ContainsRune(f, ':') {
			return "", apperr.New(apperr.Denied, "not a path in the project: "+f)
		}
		if s.IsSecret(f) {
			return "", apperr.New(apperr.Denied, "a secrets file is never committed: "+f+" (keep it in .gitignore)")
		}
	}
	args := append([]string{"add", "--"}, files...)
	if _, err := s.run(ctx, dir, false, args...); err != nil {
		return "", err
	}
	args = append([]string{"commit", "-m", message, "--"}, files...)
	if _, err := s.run(ctx, dir, true, args...); err != nil {
		return "", err
	}
	out, err := s.run(ctx, dir, false, "rev-parse", "--short", "HEAD")
	return strings.TrimSpace(string(out)), err
}

// IsSecret reports whether rel (a project path) holds secrets.
func (s *Service) IsSecret(rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	return strings.EqualFold(path.Ext(rel), ".secrets") || s.Secret != nil && s.Secret(rel)
}

func (s *Service) trustedDir() (string, error) {
	dir, err := s.dir()
	if err != nil {
		return "", err
	}
	if s.git == "" {
		return "", apperr.New(apperr.NotFound, "git was not found")
	}
	if !s.trusted(dir) {
		return "", apperr.New(apperr.Denied, "trust this folder to use git in it")
	}
	return dir, nil
}

// run runs git in dir, hardened; hooks run only when hooks is set.
func (s *Service) run(ctx context.Context, dir string, hooks bool, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	full := []string{"-c", "core.fsmonitor=false"}
	if !hooks {
		full = append(full, "-c", "core.hooksPath="+os.DevNull)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, s.git, full...) //nolint:gosec // G204: fixed git subcommands; paths checked
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New("git " + args[0] + ": " + msg)
	}
	return out, nil
}

// parseStatus parses `git status --porcelain=v1 -z`.
func parseStatus(out []byte) []FileStatus {
	list := []FileStatus{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		st := FileStatus{Index: strings.TrimSpace(f[:1]), Worktree: strings.TrimSpace(f[1:2]), Path: f[3:]}
		if f[0] == 'R' || f[0] == 'C' {
			i++ // the rename's source follows
		}
		list = append(list, st)
	}
	return list
}

// headFile finds the HEAD file of the repository holding dir, following a
// .git file (worktrees, submodules), without running git.
func headFile(dir string) ([]byte, error) {
	for d := dir; ; {
		dotgit := filepath.Join(d, ".git")
		fi, err := os.Stat(dotgit)
		if err == nil {
			gitdir := dotgit
			if !fi.IsDir() {
				data, err := os.ReadFile(dotgit) //nolint:gosec // G304: the project's .git file
				if err != nil {
					return nil, err
				}
				line := strings.TrimSpace(string(data))
				if !strings.HasPrefix(line, "gitdir:") {
					return nil, errors.New("not a .git file")
				}
				gitdir = strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
				if !filepath.IsAbs(gitdir) {
					gitdir = filepath.Join(d, gitdir)
				}
			}
			return os.ReadFile(filepath.Join(gitdir, "HEAD")) //nolint:gosec // G304: the repository's HEAD
		}
		parent := filepath.Dir(d)
		if parent == d {
			return nil, os.ErrNotExist
		}
		d = parent
	}
}

// branchOf reads a HEAD file: the branch name, or the short commit hash
// when detached.
func branchOf(head []byte) string {
	line := strings.TrimSpace(string(head))
	if ref, ok := strings.CutPrefix(line, "ref:"); ok {
		return strings.TrimPrefix(strings.TrimSpace(ref), "refs/heads/")
	}
	if len(line) > 7 {
		return line[:7]
	}
	return line
}
