// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package example holds the example project the welcome screen offers
// ("Try the example project"): a small shop API whose server is the
// mock of its own OpenAPI spec, so it runs with nothing else installed.
package example

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Name is the example project's folder name.
const Name = "shop-api"

//go:embed all:shop-api
var files embed.FS

// Write copies the example into parent/Name and returns that folder. A
// folder already there is left as it is (the example tried before,
// perhaps changed since): it is opened, never overwritten.
func Write(parent string) (string, error) {
	dir := filepath.Join(parent, Name)
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return "", fmt.Errorf("%s is a file, not a folder", dir)
		}
		return dir, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	// Written beside dir, then renamed: no half-written example remains.
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, "."+Name+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	err = fs.WalkDir(files, Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == Name {
			return err
		}
		rel, _ := filepath.Rel(Name, filepath.FromSlash(p))
		if d.IsDir() {
			return os.Mkdir(filepath.Join(tmp, rel), 0o755)
		}
		data, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(tmp, rel), data, 0o644)
	})
	if err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		// Written meanwhile (a second click): that one is opened.
		if fi, serr := os.Stat(dir); serr == nil && fi.IsDir() {
			return dir, nil
		}
		return "", err
	}
	return dir, nil
}
