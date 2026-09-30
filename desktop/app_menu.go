// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !server

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// appMenu is the default application menu without Reload and Force
// Reload: ⌘R runs the file, and a reload would drop unsaved edits.
func appMenu() *application.Menu {
	m := application.NewMenu()
	m.AddRole(application.AppMenu)
	m.AddRole(application.FileMenu)
	m.AddRole(application.EditMenu)
	view := m.AddSubmenu("View")
	view.AddRole(application.ResetZoom)
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	m.AddRole(application.WindowMenu)
	m.AddRole(application.HelpMenu)
	return m
}
