// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/agents"
)

func init() {
	register("agents", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(agents.NewService(agents.New(h.Workspace.Root)), application.ServiceOptions{Name: "agents"})
	})
}
