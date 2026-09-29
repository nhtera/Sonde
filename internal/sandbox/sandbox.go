// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package sandbox confines the files a request file may read or write to a
// file root. Every path that comes from a request file (bodies, multipart
// files, `output`, certificates, keys, netrc and cookie files, unix sockets)
// goes through a Root; paths given on the command line do not.
package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrDenied reports a path outside the file root (after resolving `..` and
// symbolic links).
var ErrDenied = errors.New("access denied outside file root")

// Root is a directory that confines file access.
type Root struct {
	dir  string // absolute path of the file root
	root *os.Root
}

// Open opens dir (relative to the working directory or absolute) as a
// file root.
func Open(dir string) (*Root, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	return &Root{dir: abs, root: root}, nil
}

// Dir returns the absolute path of the file root.
func (r *Root) Dir() string { return r.dir }

// ReadFile reads a file. name is relative to the root, or absolute and
// inside it; any other name returns an error wrapping ErrDenied.
func (r *Root) ReadFile(name string) ([]byte, error) {
	rel, err := r.rel(name)
	if err != nil {
		return nil, err
	}
	data, err := r.root.ReadFile(rel)
	if err != nil {
		return nil, r.wrap(name, err)
	}
	return data, nil
}

// WriteFile writes a file; its parent directory must exist.
func (r *Root) WriteFile(name string, data []byte) error {
	rel, err := r.rel(name)
	if err != nil {
		return err
	}
	if err := r.root.WriteFile(rel, data, 0o644); err != nil {
		return r.wrap(name, err)
	}
	return nil
}

// WriteFileAtomic writes a file with permission perm (before the umask) so
// that a reader, or a process killed while writing, never sees a partial
// file: see WriteFileAtomicIn. Missing parent directories are created.
func (r *Root) WriteFileAtomic(name string, data []byte, perm fs.FileMode) error {
	rel, err := r.rel(name)
	if err != nil {
		return err
	}
	return r.wrap(name, WriteFileAtomicIn(r.root, filepath.ToSlash(rel), data, perm))
}

// Rename renames (moves) oldname to newname, replacing a file at newname.
func (r *Root) Rename(oldname, newname string) error {
	from, err := r.rel(oldname)
	if err != nil {
		return err
	}
	to, err := r.rel(newname)
	if err != nil {
		return err
	}
	return r.wrap(oldname, r.root.Rename(from, to))
}

// Remove removes a file or an empty directory.
func (r *Root) Remove(name string) error {
	rel, err := r.rel(name)
	if err != nil {
		return err
	}
	return r.wrap(name, r.root.Remove(rel))
}

// ReadDir returns the entries of a directory, sorted by name.
func (r *Root) ReadDir(name string) ([]fs.DirEntry, error) {
	rel, err := r.rel(name)
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(r.root.FS(), filepath.ToSlash(rel))
	return entries, r.wrap(name, err)
}

// MkdirAll creates a directory and its missing parents with permission
// perm (before the umask).
func (r *Root) MkdirAll(name string, perm fs.FileMode) error {
	rel, err := r.rel(name)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	return r.wrap(name, r.root.MkdirAll(rel, perm))
}

// Stat describes a file, following symbolic links inside the root.
func (r *Root) Stat(name string) (fs.FileInfo, error) {
	rel, err := r.rel(name)
	if err != nil {
		return nil, err
	}
	fi, err := r.root.Stat(rel)
	return fi, r.wrap(name, err)
}

// Chmod changes the permission of a file.
func (r *Root) Chmod(name string, perm fs.FileMode) error {
	rel, err := r.rel(name)
	if err != nil {
		return err
	}
	return r.wrap(name, r.root.Chmod(rel, perm))
}

// WriteFileAtomicIn writes data to rel (slash-separated) inside root via a
// temporary file in the same directory, synced, then renamed into place
// and the directory synced, so a reader never sees a partial file and a
// write cut short leaves the previous content. Parent directories are
// created as needed (0o755); the file gets permission perm (before the
// umask).
func WriteFileAtomicIn(root *os.Root, rel string, data []byte, perm fs.FileMode) (err error) {
	dir := path.Dir(rel)
	if dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: output directories are 0o755, docs/guides/import-export.md
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	tmp, err := tempName(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	defer func() {
		if err != nil {
			_ = root.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("%s: %w", rel, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	if err = root.Rename(tmp, rel); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	syncDir(root, dir)
	return nil
}

// syncDir makes a rename in dir durable where the system allows syncing a
// directory; it is best effort (Windows can not open one for syncing).
func syncDir(root *os.Root, dir string) {
	d, err := root.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// tempName returns a random hidden file name for an atomic write's
// temporary file, in dir ("." for the root).
func tempName(dir string) (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	name := ".sonde-tmp-" + hex.EncodeToString(buf)
	if dir == "." {
		return name, nil
	}
	return path.Join(dir, name), nil
}

// Path returns the absolute path of name after checking that it stays in
// the root, for the APIs that need a path (unix sockets).
func (r *Root) Path(name string) (string, error) {
	rel, err := r.rel(name)
	if err != nil {
		return "", err
	}
	if err := r.checkEscape(rel); err != nil {
		return "", r.wrap(name, err)
	}
	return filepath.Join(r.dir, rel), nil
}

// Close releases the root.
func (r *Root) Close() error {
	return r.root.Close()
}

// rel resolves name to a path relative to the root. A relative name is
// joined to the root and normalized first, so `../root/x` is allowed when
// it lands back inside; names that end up outside are rejected. Escapes
// through symbolic links are caught afterwards by the os.Root methods,
// which refuse to follow a link out of the root.
func (r *Root) rel(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("sandbox: empty path: %w", ErrDenied)
	}
	full := name
	if !filepath.IsAbs(full) {
		full = filepath.Join(r.dir, name)
	}
	clean, err := filepath.Rel(r.dir, filepath.Clean(full))
	if err != nil || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("sandbox: %s: %w", name, ErrDenied)
	}
	return clean, nil
}

// checkEscape verifies that the longest existing prefix of rel resolves
// (following any symbolic links) without leaving the root; os.Root refuses
// such a resolution, which is how a symlink escape is caught. Components
// that do not exist yet cannot be symlinks, so they need no check.
func (r *Root) checkEscape(rel string) error {
	cur := rel
	for cur != "." && cur != "" {
		_, err := r.root.Lstat(cur)
		switch {
		case err == nil:
			// Existing entry: Stat (not Lstat) forces symlink resolution,
			// which os.Root rejects if it would leave the root.
			if _, err := r.root.Stat(cur); err != nil {
				return err
			}
			return nil
		case errors.Is(err, fs.ErrNotExist):
			cur = filepath.Dir(cur)
		default:
			return err
		}
	}
	return nil
}

// wrap turns an os.Root escape error into one wrapping ErrDenied; every
// other error (not found, permission, ...) is returned unchanged.
func (r *Root) wrap(name string, err error) error {
	if err == nil {
		return nil
	}
	if isEscape(err) {
		return fmt.Errorf("sandbox: %s: %w", name, ErrDenied)
	}
	return err
}

// isEscape reports whether err came from os.Root refusing to follow a path
// (typically a symbolic link) outside the root. os.Root does not export a
// sentinel for this, so the wording of its error is matched instead.
func isEscape(err error) bool {
	return strings.Contains(err.Error(), "escapes from parent")
}
