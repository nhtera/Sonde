// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/clipboard"
	"github.com/nhtera/sonde/desktop/internal/copyas"
)

func init() {
	register("copyas", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(copyas.NewService(h.copier()), application.ServiceOptions{Name: "copyas"})
	})
	// Copy as with credentials writes the clipboard from Go: the window
	// app only (a server's clipboard is not the user's).
	register("copyasReveal", []Mode{ModeDesktop}, func(h *Host) application.Service {
		return application.NewServiceWithOptions(copyas.NewReveal(h.copier(), clipboard.WriteSecret),
			application.ServiceOptions{Name: "copyasReveal"})
	})
}

func (h *Host) copier() *copyas.Copier {
	c := copyas.New(h.Runs, func() string {
		if r := h.Workspace.Root(); r != nil {
			return r.Dir()
		}
		return "the project folder"
	})
	c.Command = h.Envs.Command
	return c
}
