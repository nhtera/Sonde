// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package config

import "syscall"

// syscallMkfifo creates a FIFO at path, for TestLoadProjectReferencedFileNotRegular.
func syscallMkfifo(path string) error {
	return syscall.Mkfifo(path, 0o600)
}
