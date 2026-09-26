// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to path by creating a temporary file in the
// same directory, writing and closing it, then renaming it into place —
// so a process killed mid-write is left with, at worst, an orphaned temp
// file, never a truncated report that a later cumulative run then fails
// to parse (rename within one directory is atomic on every platform
// sonde targets). path's directory must already exist.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*") //nolint:gosec // G304: dir comes from a CLI-trusted --report-* flag
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmpPath) //nolint:errcheck,gosec // G104: best-effort cleanup on the failure path
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck,gosec // already failing; the write error is what's returned
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close() //nolint:errcheck,gosec // already failing; the chmod error is what's returned
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}
