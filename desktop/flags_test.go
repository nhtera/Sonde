// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRootMakesAbsolute(t *testing.T) {
	tempDir := t.TempDir()
	root, err := projectRoot(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(root) {
		t.Errorf("root %q is not absolute", root)
	}
	if root != tempDir {
		t.Errorf("root %q, want %q", root, tempDir)
	}
}

func TestProjectRootRejectsNonexistent(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does", "not", "exist")
	_, err := projectRoot(nonexistent)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
	if !contains(err.Error(), "--root") {
		t.Errorf("error %v should mention --root", err)
	}
}

func TestProjectRootRejectsFile(t *testing.T) {
	tempDir := t.TempDir()
	file := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(file, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := projectRoot(file)
	if err == nil {
		t.Error("expected error for file instead of directory")
	}
	if !contains(err.Error(), "not a directory") {
		t.Errorf("error %v should mention 'not a directory'", err)
	}
}

func TestProjectRootAcceptsSymlink(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "target")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tempDir, "link")
	if err := os.Symlink(targetDir, linkPath); err != nil {
		t.Fatal(err)
	}
	root, err := projectRoot(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	// On most systems, Stat on a symlink returns the target's info
	if root != linkPath {
		t.Logf("symlink resolved to %q (expected %q), but this is platform-dependent", root, linkPath)
	}
}

func TestOpenDirsCustomPath(t *testing.T) {
	tempDir := t.TempDir()
	dirs, err := openDirs(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer dirs.Close()
	if dirs == nil {
		t.Error("expected non-nil dirs")
	}
	configDir := dirs.Config().Dir()
	expectedConfig := filepath.Join(tempDir, "config")
	if configDir != expectedConfig {
		t.Errorf("config dir %q, want %q", configDir, expectedConfig)
	}
	cacheDir := dirs.Cache().Dir()
	expectedCache := filepath.Join(tempDir, "cache")
	if cacheDir != expectedCache {
		t.Errorf("cache dir %q, want %q", cacheDir, expectedCache)
	}
}

func TestOpenDirsCreatesDirectories(t *testing.T) {
	tempDir := t.TempDir()
	dirs, err := openDirs(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer dirs.Close()

	// Check that config directory was created
	configPath := filepath.Join(tempDir, "config")
	if fi, err := os.Stat(configPath); err != nil || !fi.IsDir() {
		t.Errorf("config directory not created at %q", configPath)
	}

	// Check that cache directory was created
	cachePath := filepath.Join(tempDir, "cache")
	if fi, err := os.Stat(cachePath); err != nil || !fi.IsDir() {
		t.Errorf("cache directory not created at %q", cachePath)
	}
}

// contains is a helper to check if a string contains a substring (simple substring check).
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
