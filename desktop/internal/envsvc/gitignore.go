// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package envsvc

import (
	"bytes"
	"errors"
	"io/fs"
	"path"
	"strings"

	"github.com/nhtera/sonde/internal/sandbox"
)

// TopicIgnored is sent with the line added to .gitignore.
const TopicIgnored = "env:gitignored"

// secretsPattern is the .gitignore line that keeps every secrets file out
// of git.
const secretsPattern = "*.secrets"

// ignoreSecrets keeps rel, a secrets file just written (project path),
// out of git: in a git repository whose .gitignore has no line covering
// it, it appends secretsPattern. The one dot file the app writes, by a
// line only (docs/desktop.md). It reports whether it added the line.
func ignoreSecrets(root *sandbox.Root, rel string) (bool, error) {
	if _, err := root.Stat(".git"); err != nil {
		return false, nil // not a repository (or one the app can not tell)
	}
	data, err := root.ReadFile(".gitignore")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if ignored(data, rel) {
		return false, nil
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	data = append(data, "# secret values, kept out of git (Sonde)\n"+secretsPattern+"\n"...)
	mode := fs.FileMode(0o644)
	if fi, err := root.Stat(".gitignore"); err == nil {
		mode = fi.Mode().Perm()
	}
	return true, root.WriteFileAtomic(".gitignore", data, mode)
}

// ignored reports whether a line of .gitignore covers rel (a project
// path): the common shapes of git's patterns (a name or glob matching any
// of its path's segments, or an anchored path or folder), not every rule;
// a negation is not followed. Not found, the caller adds secretsPattern,
// which is harmless when git ignored rel already.
func ignored(gitignore []byte, rel string) bool {
	segs := strings.Split(rel, "/")
	for _, line := range strings.Split(string(gitignore), "\n") {
		pat := strings.TrimSpace(line)
		if pat == "" || strings.HasPrefix(pat, "#") || strings.HasPrefix(pat, "!") {
			continue
		}
		pat = strings.TrimSuffix(strings.TrimPrefix(pat, "**/"), "/")
		if !strings.Contains(strings.TrimPrefix(pat, "/"), "/") {
			// A name: any segment of the path (a folder holds the file).
			name := strings.TrimPrefix(pat, "/")
			for i, s := range segs {
				if ok, _ := path.Match(name, s); ok && (!strings.HasPrefix(pat, "/") || i == 0) {
					return true
				}
			}
			continue
		}
		// A path from the root: the file, or a folder above it.
		pat = strings.TrimPrefix(pat, "/")
		for i := range segs {
			if ok, _ := path.Match(pat, strings.Join(segs[:i+1], "/")); ok {
				return true
			}
		}
	}
	return false
}
