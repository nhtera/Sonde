// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !windows && !linux

package update

import "errors"

// NewInstaller returns an installer that offers the release page only.
func NewInstaller(InstallerDeps) Installer { return noInstaller{} }

type noInstaller struct{}

func (noInstaller) CanInstall() (bool, string) { return false, reasonUnsupported }
func (noInstaller) Install(Record) error {
	return &Error{Kind: KindInstall, Err: errors.New(reasonUnsupported)}
}
