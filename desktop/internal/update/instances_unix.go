// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package update

import (
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes f's exclusive lock without waiting; an error when another
// open file holds it.
func tryLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) //nolint:gosec // a file descriptor
}

func unlock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:gosec // a file descriptor
}
