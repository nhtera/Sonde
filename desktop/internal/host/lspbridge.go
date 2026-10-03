// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/lspbridge"
	"github.com/nhtera/sonde/internal/config"
)

func init() {
	register("lspbridge", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(lspbridge.NewBridge(h.Emit, config.FromOSEnviron(), h.Version),
			application.ServiceOptions{Name: "lspbridge"})
	})
}
