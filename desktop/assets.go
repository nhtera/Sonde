// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

// assets is the built frontend (npm run build writes frontend/dist).
//
//go:embed all:frontend/dist
var assets embed.FS
