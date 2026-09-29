// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package bodies

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// markDownloaded sets the quarantine attribute, so macOS treats the file
// as downloaded.
func markDownloaded(path string) {
	value := fmt.Sprintf("0081;%08x;Sonde;", time.Now().Unix())
	_ = unix.Setxattr(path, "com.apple.quarantine", []byte(value), 0)
}
