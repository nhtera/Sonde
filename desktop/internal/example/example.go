// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package example holds the example project the welcome screen offers
// ("Try the example project"): a small shop API whose server is the
// mock of its own OpenAPI spec, so it runs with nothing else installed.
package example

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/nhtera/sonde/internal/sandbox"
)

// Name is the example project's folder name.
const Name = "shop-api"

//go:embed all:shop-api
var files embed.FS

// Write copies the example into the folder dir/Name of root (dir is
// made when missing) and returns that folder's path in root. A folder
// already there is left as it is (the example tried before, perhaps
// changed since): it is opened, never overwritten.
func Write(root *sandbox.Root, dir string) (string, error) {
	target := path.Join(dir, Name)
	if fi, err := root.Stat(target); err == nil {
		if !fi.IsDir() {
			return "", fmt.Errorf("%s is a file, not a folder", target)
		}
		return target, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	// Written beside the target, then renamed: no half-written example
	// remains under its name.
	var b [6]byte
	_, _ = rand.Read(b[:])
	tmp := path.Join(dir, "."+Name+"-"+hex.EncodeToString(b[:]))
	if err := root.MkdirAll(tmp, 0o750); err != nil {
		return "", err
	}
	err := fs.WalkDir(files, Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return root.WriteFileAtomic(path.Join(tmp, p[len(Name)+1:]), data, 0o600)
	})
	if err == nil {
		err = root.Rename(tmp, target)
	}
	if err != nil {
		removeTree(root, tmp)
		// Written meanwhile (a second click): that one is opened.
		if fi, serr := root.Stat(target); serr == nil && fi.IsDir() {
			return target, nil
		}
		return "", err
	}
	return target, nil
}

// removeTree removes dir of root and what it holds, as far as it can.
func removeTree(root *sandbox.Root, dir string) {
	entries, _ := root.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			removeTree(root, path.Join(dir, e.Name()))
		} else {
			_ = root.Remove(path.Join(dir, e.Name()))
		}
	}
	_ = root.Remove(dir)
}
