// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// errNotRequestFile reports a path that is not a .hurl or .sonde file:
// tools never read other files, whose content a parse error would quote.
var errNotRequestFile = errors.New("not a request file (.hurl or .sonde)")

// requestFile is a request file named in a tool call, confined to the root.
type requestFile struct {
	abs string // absolute path
	rel string // relative to the root, with forward slashes
	src []byte
}

// isRequestFile reports whether name has a request file extension.
func isRequestFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".hurl", ".sonde":
		return true
	}
	return false
}

// resolve returns the absolute and root-relative forms of name (relative
// to the root, or absolute inside it). A path leaving the root, directly
// or through a symbolic link, is an error.
func (s *server) resolve(name string) (abs, rel string, err error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", "", fmt.Errorf("invalid path %q", name)
	}
	abs, err = s.box.Path(name)
	if err != nil {
		return "", "", fmt.Errorf("%s: outside the server root", name)
	}
	rel, err = filepath.Rel(s.cfg.Root, abs)
	if err != nil {
		return "", "", fmt.Errorf("%s: outside the server root", name)
	}
	return abs, filepath.ToSlash(rel), nil
}

// readRequestFile reads a request file named in a tool call.
func (s *server) readRequestFile(name string) (*requestFile, error) {
	if !isRequestFile(name) {
		return nil, fmt.Errorf("%s: %w", name, errNotRequestFile)
	}
	abs, rel, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("%s: no such file", rel)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", rel)
	}
	if info.Size() > syntax.MaxFileSize {
		return nil, fmt.Errorf("%s: larger than %d MiB", rel, syntax.MaxFileSize>>20)
	}
	// Read through the root, which refuses a symbolic link out of it.
	src, err := s.box.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		return nil, fmt.Errorf("%s: outside the server root or unreadable", rel)
	}
	if len(src) > syntax.MaxFileSize {
		return nil, fmt.Errorf("%s: larger than %d MiB", rel, syntax.MaxFileSize>>20)
	}
	return &requestFile{abs: abs, rel: rel, src: src}, nil
}

// inRoot reports whether an absolute path is the root or under it.
func (s *server) inRoot(abs string) bool {
	rel, err := filepath.Rel(s.cfg.Root, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
