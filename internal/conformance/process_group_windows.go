// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"os"
	"syscall"
)

// groupAttr starts a process in a new process group. The harness needs a
// Unix shell and servers; on Windows it only has to build.
func groupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killGroup kills the process itself: Windows has no signal for a group.
func killGroup(pid int, _ syscall.Signal) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill() //nolint:errcheck // best-effort teardown.
	}
}
