// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/workspace"
)

func init() {
	register("workspace", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(workspace.NewService(h.Workspace), application.ServiceOptions{Name: "workspace"})
	})
	// Opening folders, the recent list, the file manager and the trash
	// belong to the window app: server mode serves its --root only.
	register("workspaceDesktop", []Mode{ModeDesktop}, func(h *Host) application.Service {
		d := workspace.NewDesktop(h.Workspace, h.Dirs.Config(), h.Handles, pickFolder)
		return application.NewServiceWithOptions(d, application.ServiceOptions{Name: "workspaceDesktop"})
	})
}

// pickFolder shows the native folder dialog; "" when canceled.
func pickFolder() (string, error) {
	return application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		CanChooseDirectories: true,
		CanChooseFiles:       false,
		Title:                "Open a project folder",
	}).PromptForSingleSelection()
}
