// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package config

import (
	"os"
	"syscall"
)

// candidateIsSecure reports whether a discovered (not --config) sonde.yaml
// is safe to use: owned by the user running sonde, and not writable by
// group or others. See docs/sonde-yaml.md, Discovery: a candidate failing
// this check is treated as absent, so a file planted elsewhere on a
// shared filesystem (a world-writable /tmp, a compromised $HOME) cannot
// silently configure a run it was never meant to.
func candidateIsSecure(info os.FileInfo) bool {
	if info.Mode().Perm()&0o022 != 0 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return int(st.Uid) == os.Getuid()
}
