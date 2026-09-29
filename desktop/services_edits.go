// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/editsvc"
	"github.com/nhtera/sonde/desktop/internal/vars"
)

func init() {
	register("vars", nil, func(h *Host) application.Service {
		v := vars.New(h.Workspace.Root, h.Runs.Captures, h.Envs.SessionOverrides)
		return application.NewServiceWithOptions(vars.NewService(v), application.ServiceOptions{Name: "vars"})
	})
	register("editsvc", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(editsvc.NewService(editsvc.New(h.Bodies, h.Runs.Prepare)),
			application.ServiceOptions{Name: "editsvc"})
	})
}
