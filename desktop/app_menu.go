// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !server

package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// appMenu is the default application menu without Reload and Force
// Reload: ⌘R runs the file, and a reload would drop unsaved edits. On
// macOS, File › Close Window is ⇧⌘W, so ⌘W reaches the page, where it
// closes the tab (rebindable there, like every app shortcut).
func appMenu() *application.Menu {
	m := application.NewMenu()
	m.AddRole(application.AppMenu)
	if runtime.GOOS == "darwin" {
		m.AddSubmenu("File").Add("Close Window").SetAccelerator("CmdOrCtrl+Shift+W").OnClick(func(*application.Context) {
			if w := application.Get().Window.Current(); w != nil {
				w.Close()
			}
		})
	} else {
		m.AddRole(application.FileMenu)
	}
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
