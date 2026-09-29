// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"path/filepath"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/apperr"
)

// clean checks a project-relative, slash-separated path from the page and
// returns it in OS form. The page never sends absolute paths; ":" is
// refused everywhere (a drive letter, or a Windows alternate data stream).
func clean(rel string) (string, error) {
	if rel == "" || strings.ContainsAny(rel, "\x00\\:") ||
		path.IsAbs(rel) || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", apperr.New(apperr.Denied, "not a path in the project: "+rel)
	}
	c := path.Clean(rel)
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", apperr.New(apperr.Denied, "not a path in the project: "+rel)
	}
	return filepath.FromSlash(c), nil
}

// writable checks that the app may write rel: never inside a dot folder
// (.git, .vscode, …), which a project may use for tools that run code.
func writable(rel string) (string, error) {
	p, err := clean(rel)
	if err != nil {
		return "", err
	}
	if p == "." {
		return "", apperr.New(apperr.Denied, "not a file: "+rel)
	}
	dirs := strings.Split(filepath.ToSlash(filepath.Dir(p)), "/")
	for _, d := range dirs {
		if strings.HasPrefix(d, ".") && d != "." {
			return "", apperr.New(apperr.Denied, "the app does not write in dot folders: "+rel)
		}
	}
	return p, nil
}

// hashOf is the content hash the page passes back to Save.
func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ignoredDir reports whether the tree and the watcher skip a folder.
func ignoredDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}
