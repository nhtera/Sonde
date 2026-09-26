// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"fmt"
	"io"
	"os"
)

// MaxInput is the largest file an importer reads: a collection, a spec
// sidecar or an environment file.
const MaxInput = 64 << 20

// ReadInput reads the file an importer is given, at most limit bytes. It
// refuses anything but a regular file, before opening it, so a named pipe
// can't block the import.
func ReadInput(name string, limit int64) ([]byte, error) {
	st, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	f, err := os.Open(name) //nolint:gosec // G304: the user names the file to import
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return ReadLimited(f, name, limit)
}

// ReadLimited reads r, named name in errors, refusing more than limit bytes.
func ReadLimited(r io.Reader, name string, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: larger than %d MiB", name, limit>>20)
	}
	return data, nil
}
