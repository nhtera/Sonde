// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/host"
)

// version is the app's version (set at build time with
// -X main.version); the host passes it to runs as the User-Agent's.
var version = "dev"

// appOptions are the options every mode shares.
func appOptions(h *host.Host) application.Options {
	return application.Options{
		Name:        "Sonde",
		Description: "Reads and runs .hurl files",
		Services:    host.Services(h),
		// Coded errors reach the page as {code, message}.
		MarshalError: apperr.Marshal,
		Assets: application.AssetOptions{
			// The page carries the theme settings (no wrong-theme flash).
			Handler: themedIndex(application.AssetFileServerFS(assets), h.Settings.Get),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	}
}
