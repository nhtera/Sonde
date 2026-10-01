// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
)

// The flags the window app and server mode share: --data and --root.

// openDirs opens the app data folders: <data>/config and <data>/cache, or
// the user's own when data is empty.
func openDirs(data string) (*appdirs.Dirs, error) {
	if data == "" {
		return appdirs.Default()
	}
	return appdirs.Open(filepath.Join(data, "config"), filepath.Join(data, "cache"))
}

// projectRoot resolves --root to an existing directory.
func projectRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("--root: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("--root: %s is not a directory", abs)
	}
	return abs, nil
}
