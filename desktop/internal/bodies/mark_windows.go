// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package bodies

import "os"

// markDownloaded adds the Mark of the Web (an Internet zone stream), so
// Windows treats the file as downloaded.
func markDownloaded(path string) {
	//nolint:forbidigo // the file's own alternate data stream
	_ = os.WriteFile(path+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o600)
}
