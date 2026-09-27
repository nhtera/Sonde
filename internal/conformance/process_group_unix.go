// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package conformance

import "syscall"

// groupAttr starts a process in a new process group, so killGroup also
// reaches the processes it starts.
func groupAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

func killGroup(pid int, sig syscall.Signal) {
	// Negative pid signals the whole process group created by Setpgid.
	_ = syscall.Kill(-pid, sig) //nolint:errcheck // best-effort teardown.
}
