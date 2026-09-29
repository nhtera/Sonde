// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !windows

package bodies

// markDownloaded does nothing: Linux desktops have no such mark.
func markDownloaded(string) {}
