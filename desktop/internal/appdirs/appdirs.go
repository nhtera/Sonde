// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package appdirs locates the app's own data: settings, run history, kept
// cookie jars and folder trust in the user config directory, and
// short-lived files (a server launch link) in the user cache directory.
// Both are private to the user (0700) and reached only through a
// sandbox.Root.
package appdirs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nhtera/sonde/internal/sandbox"
)

// Names inside the config directory.
const (
	SettingsFile = "settings.json"
	HistoryDir   = "history"
	CookiesDir   = "cookies"
	TrustFile    = "trust.json"
)

// appName is the directory the app uses under the user directories.
const appName = "Sonde"

// Dirs is the app's config and cache directories.
type Dirs struct {
	config *sandbox.Root
	cache  *sandbox.Root
}

// Default opens <user config dir>/Sonde and <user cache dir>/Sonde,
// creating them.
func Default() (*Dirs, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("appdirs: %w", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("appdirs: %w", err)
	}
	return Open(filepath.Join(config, appName), filepath.Join(cache, appName))
}

// Open opens configDir and cacheDir, creating them (0700) when missing. An
// existing directory that others can reach is narrowed to 0700.
func Open(configDir, cacheDir string) (*Dirs, error) {
	config, err := openPrivate(configDir)
	if err != nil {
		return nil, err
	}
	cache, err := openPrivate(cacheDir)
	if err != nil {
		_ = config.Close()
		return nil, err
	}
	return &Dirs{config: config, cache: cache}, nil
}

// Config is the config directory.
func (d *Dirs) Config() *sandbox.Root { return d.config }

// Cache is the cache directory.
func (d *Dirs) Cache() *sandbox.Root { return d.cache }

// Close releases both directories.
func (d *Dirs) Close() error {
	return errors.Join(d.config.Close(), d.cache.Close())
}

func openPrivate(dir string) (*sandbox.Root, error) {
	if dir == "" {
		return nil, errors.New("appdirs: empty directory")
	}
	//nolint:forbidigo // the directory itself is created before a Root can hold it
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("appdirs: %w", err)
	}
	root, err := sandbox.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("appdirs: %w", err)
	}
	fi, err := root.Stat(".")
	// Windows has no group or other bits: ACLs guard the profile dirs.
	if err == nil && runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		err = root.Chmod(".", 0o700)
	}
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("appdirs: %w", err)
	}
	return root, nil
}
