// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/editsvc"
	"github.com/nhtera/sonde/desktop/internal/vars"
)

func init() {
	register("vars", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(vars.NewService(h.Vars()), application.ServiceOptions{Name: "vars"})
	})
	register("editsvc", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(editsvc.NewService(editsvc.New(h.Bodies, h.Runs.Prepare)),
			application.ServiceOptions{Name: "editsvc"})
	})
}

// Vars is the variables lister of the open project.
func (h *Host) Vars() *vars.Vars {
	return vars.New(h.Workspace.Root, h.Runs.Captures, h.Envs.SessionOverrides)
}
