// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package config

import "os"

// candidateIsSecure always accepts on Windows: ownership and world/group
// writability are not how its ACL model expresses this, so the check
// docs/sonde-yaml.md describes is Unix-specific.
func candidateIsSecure(_ os.FileInfo) bool { return true }
