// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// release makes a key set (k1) and the four update files of 0.2.1 in a
// temp folder, and returns the folder.
func release(t *testing.T) (dir string, keys string) {
	t.Helper()
	dir, keys = t.TempDir(), t.TempDir()
	must(t, run([]string{"genkey", "-id", "k1", "-out", keys}, io.Discard))
	for name := range updateFiles("0.2.1") {
		must(t, os.WriteFile(filepath.Join(dir, name), []byte("contents of "+name), 0o600))
	}
	must(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("Sonde Desktop 0.2.1\n"), 0o600))
	return dir, keys
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func signArgs(dir, keys string) []string {
	args := []string{"sign", "-version", "0.2.1", "-key", filepath.Join(keys, "k1.key"), "-keyid", "k1",
		"-notes", filepath.Join(dir, "notes.txt"), "-out", filepath.Join(dir, "Sonde-Desktop-0.2.1.update.json")}
	for name := range updateFiles("0.2.1") {
		args = append(args, filepath.Join(dir, name))
	}
	return args
}

func verifyArgs(dir, keys string) []string {
	return []string{"verify", "-keys", keys, "-manifest", filepath.Join(dir, "Sonde-Desktop-0.2.1.update.json"), "-version", "0.2.1"}
}

func TestSignVerify(t *testing.T) {
	dir, keys := release(t)
	must(t, run(signArgs(dir, keys), io.Discard))
	must(t, run(verifyArgs(dir, keys), io.Discard))

	// The private key is readable by its owner only, and never replaced.
	if info, err := os.Stat(filepath.Join(keys, "k1.key")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("k1.key mode = %v, %v; want 0600", info.Mode(), err)
	}
	if err := run([]string{"genkey", "-id", "k1", "-out", keys}, io.Discard); err == nil {
		t.Fatal("genkey replaced an existing key")
	}
}

func TestGenkeyRefusesAWorktree(t *testing.T) {
	repo := t.TempDir()
	must(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	keys := filepath.Join(repo, "internal", "update", "keys")
	must(t, os.MkdirAll(keys, 0o700))
	if err := run([]string{"genkey", "-id", "k1", "-out", keys}, io.Discard); err == nil {
		t.Fatal("genkey wrote a private key inside a git worktree")
	}
	if _, err := os.Stat(filepath.Join(keys, "k1.key")); !os.IsNotExist(err) {
		t.Fatalf("k1.key exists: %v", err)
	}
}

func TestVerifyRefuses(t *testing.T) {
	file := func(dir string) string { return filepath.Join(dir, "Sonde-Desktop-0.2.1.update.json") }
	edit := func(t *testing.T, dir string, change func(m map[string]any)) {
		b, err := os.ReadFile(file(dir))
		must(t, err)
		var m map[string]any
		must(t, json.Unmarshal(b, &m))
		change(m)
		b, err = json.Marshal(m)
		must(t, err)
		must(t, os.WriteFile(file(dir), b, 0o600))
	}
	artifact := func(m map[string]any, i int) map[string]any { return m["artifacts"].([]any)[i].(map[string]any) }

	cases := map[string]func(t *testing.T, dir, keys string){
		"a byte of an update file": func(t *testing.T, dir, _ string) {
			f := filepath.Join(dir, "Sonde-Desktop-0.2.1-linux-x86_64.AppImage")
			b, err := os.ReadFile(f)
			must(t, err)
			b[0] ^= 1
			must(t, os.WriteFile(f, b, 0o600)) //nolint:gosec // G703: a temp folder
		},
		"an update file missing": func(t *testing.T, dir, _ string) {
			must(t, os.Remove(filepath.Join(dir, "Sonde-Desktop-0.2.1-macos-universal.zip")))
		},
		"version":   func(t *testing.T, dir, _ string) { edit(t, dir, func(m map[string]any) { m["version"] = "0.2.2" }) },
		"notes":     func(t *testing.T, dir, _ string) { edit(t, dir, func(m map[string]any) { m["notes"] = "other" }) },
		"size":      func(t *testing.T, dir, _ string) { edit(t, dir, func(m map[string]any) { artifact(m, 0)["size"] = 1 }) },
		"signature": func(t *testing.T, dir, _ string) { edit(t, dir, func(m map[string]any) { delete(m, "signature") }) },
		"unsigned field": func(t *testing.T, dir, _ string) {
			edit(t, dir, func(m map[string]any) { m["url"] = "https://example.com" })
		},
		"another key set": func(t *testing.T, _, keys string) {
			must(t, os.Remove(filepath.Join(keys, "k1.pub")))
			other := t.TempDir()
			must(t, run([]string{"genkey", "-id", "k2", "-out", other}, io.Discard))
			must(t, os.Rename(filepath.Join(other, "k2.pub"), filepath.Join(keys, "k2.pub")))
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			dir, keys := release(t)
			must(t, run(signArgs(dir, keys), io.Discard))
			change(t, dir, keys)
			if err := run(verifyArgs(dir, keys), io.Discard); err == nil {
				t.Fatal("verify accepted it")
			}
		})
	}
}

func TestSignRefuses(t *testing.T) {
	cases := map[string]func(args []string, dir string) []string{
		"a file not in the release": func(args []string, dir string) []string {
			extra := filepath.Join(dir, "Sonde-Desktop-0.2.1-linux-arm64.AppImage")
			must(t, os.WriteFile(extra, []byte("x"), 0o600))
			return append(args, extra)
		},
		"an update file missing": func(args []string, _ string) []string { return args[:len(args)-1] },
		"another version's name": func(args []string, _ string) []string {
			// The flags name 0.2.2, the files are 0.2.1's.
			for i, a := range args {
				if a == "0.2.1" || strings.HasSuffix(a, "Sonde-Desktop-0.2.1.update.json") {
					args[i] = strings.ReplaceAll(a, "0.2.1", "0.2.2")
				}
			}
			return args
		},
		"a malformed key": func(args []string, dir string) []string {
			bad := filepath.Join(dir, "bad.key")
			must(t, os.WriteFile(bad, []byte("c2hvcnQ="), 0o600))
			for i, a := range args {
				if strings.HasSuffix(a, "k1.key") {
					args[i] = bad
				}
			}
			return args
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			dir, keys := release(t)
			if err := run(change(signArgs(dir, keys), dir), io.Discard); err == nil {
				t.Fatal("sign accepted it")
			}
		})
	}
}
