// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package update

import (
	"os"

	"golang.org/x/sys/windows"
)

// tryLock takes f's exclusive lock without waiting; an error when another
// handle holds it.
func tryLock(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
}

func unlock(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
