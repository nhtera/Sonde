// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package sandboxtest opens file roots for tests.
package sandboxtest

import (
	"testing"

	"github.com/nhtera/sonde/internal/sandbox"
)

// Open opens a file root on dir and closes it when the test ends: a root
// holds its folder open, and Windows cannot remove an open folder.
func Open(t testing.TB, dir string) *sandbox.Root {
	t.Helper()
	root, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}
