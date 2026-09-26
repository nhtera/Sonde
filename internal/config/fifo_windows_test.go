// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package config

import "errors"

// syscallMkfifo has no Windows equivalent; TestLoadProjectReferencedFileNotRegular
// skips itself on this platform before calling it.
func syscallMkfifo(string) error {
	return errors.New("mkfifo not supported on windows")
}
