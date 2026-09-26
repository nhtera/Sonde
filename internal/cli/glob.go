// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// globFiles expands a glob pattern into matching file paths, supporting
// "**" (matches zero or more path segments, including none) in addition
// to filepath.Match's single-segment "*"/"?"/"[...]". Results are sorted
// for determinism.
func globFiles(pattern string) ([]string, error) {
	// filepath.Clean collapses a leading "./" and any "x/.." pair, but a
	// leading ".." (nothing to cancel it against) survives; globSegments
	// below walks that, and any other literal "." or "..", as a plain
	// directory step instead of matching it against directory entries
	// (which never include "." or ".." themselves).
	segments := strings.Split(filepath.ToSlash(filepath.Clean(pattern)), "/")
	root := "."
	if filepath.IsAbs(pattern) {
		root = "/"
		segments = segments[1:]
	}
	matches, err := globSegments(root, segments)
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// globSegments matches segments (already split on '/') against paths
// under base.
func globSegments(base string, segments []string) ([]string, error) {
	if len(segments) == 0 {
		if info, err := os.Stat(base); err == nil && !info.IsDir() {
			return []string{base}, nil
		}
		return nil, nil
	}
	seg, rest := segments[0], segments[1:]
	switch seg {
	case "**":
		return globDoubleStar(base, rest)
	case ".", "..":
		return globSegments(filepath.Join(base, seg), rest)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, nil //nolint:nilerr // an unreadable directory simply matches nothing
	}
	var out []string
	for _, e := range entries {
		ok, err := filepath.Match(seg, e.Name())
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		child := filepath.Join(base, e.Name())
		if len(rest) == 0 {
			if !e.IsDir() {
				out = append(out, child)
			}
			continue
		}
		matches, err := globSegments(child, rest)
		if err != nil {
			return nil, err
		}
		out = append(out, matches...)
	}
	return out, nil
}

// globDoubleStar matches "**" followed by rest: every descendant directory
// of base (including base itself), each tried against rest.
func globDoubleStar(base string, rest []string) ([]string, error) {
	var out []string
	matches, err := globSegments(base, rest)
	if err != nil {
		return nil, err
	}
	out = append(out, matches...)
	entries, err := os.ReadDir(base)
	if err != nil {
		return out, nil //nolint:nilerr // an unreadable directory simply contributes nothing more
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := globDoubleStar(filepath.Join(base, e.Name()), rest)
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
	}
	return out, nil
}
