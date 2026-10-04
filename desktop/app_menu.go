// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !server

package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/emit"
)

// helpURL is the desktop guide, opened in the default browser.
const helpURL = "https://github.com/nhtera/Sonde/blob/main/docs/desktop.md"

// appMenu is the default application menu without Reload and Force
// Reload: ⌘R runs the file, and a reload would drop unsaved edits. On
// macOS, File › Close Window is ⇧⌘W, so ⌘W reaches the page, where it
// closes the tab (rebindable there, like every app shortcut). Edit ›
// Undo and Redo ask the page (editMenu). Help is Sonde's own: the stock
// one loads its page into the app's window, dropping unsaved edits.
// "Check for Updates…" runs checkUpdates, in Help and, on macOS, in the
// app menu.
func appMenu(checkUpdates func()) *application.Menu {
	m := application.NewMenu()
	if runtime.GOOS == "darwin" {
		// The stock app menu's roles, with the update item after About.
		app := m.AddSubmenu("Sonde")
		app.AddRole(application.About)
		app.Add("Check for Updates…").OnClick(func(*application.Context) { checkUpdates() })
		app.AddSeparator()
		app.AddRole(application.ServicesMenu)
		app.AddSeparator()
		app.AddRole(application.Hide)
		app.AddRole(application.HideOthers)
		app.AddRole(application.UnHide)
		app.AddSeparator()
		app.AddRole(application.Quit)
	} else {
		m.AddRole(application.AppMenu)
	}
	if runtime.GOOS == "darwin" {
		m.AddSubmenu("File").Add("Close Window").SetAccelerator("CmdOrCtrl+Shift+W").OnClick(func(*application.Context) {
			if w := application.Get().Window.Current(); w != nil {
				w.Close()
			}
		})
	} else {
		m.AddRole(application.FileMenu)
	}
	editMenu(m.AddSubmenu("Edit"))
	view := m.AddSubmenu("View")
	view.AddRole(application.ResetZoom)
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	m.AddRole(application.WindowMenu)
	help := m.AddSubmenu("Help")
	help.Add("Sonde Desktop Help").OnClick(func(*application.Context) {
		_ = application.Get().Browser.OpenURL(helpURL)
	})
	help.Add("Check for Updates…").OnClick(func(*application.Context) { checkUpdates() })
	return m
}

// editMenu is the Edit menu's roles, but Undo and Redo ask the page
// (app:edit): the native undo reaches only the webview's own undo stack,
// which the code editor, keeping its own history, never fills. Their keys
// reach the page first, as before.
func editMenu(m *application.Menu) {
	edit := func(kind string) func(*application.Context) {
		return func(*application.Context) { emit.Wails{}.Emit("app:edit", kind) }
	}
	m.Add("Undo").SetAccelerator("CmdOrCtrl+Z").OnClick(edit("undo"))
	m.Add("Redo").SetAccelerator("CmdOrCtrl+Shift+Z").OnClick(edit("redo"))
	m.AddSeparator()
	m.AddRole(application.Cut)
	m.AddRole(application.Copy)
	m.AddRole(application.Paste)
	if runtime.GOOS == "darwin" {
		m.AddRole(application.PasteAndMatchStyle)
		m.AddRole(application.Delete)
		m.AddRole(application.SelectAll)
		m.AddSeparator()
		m.AddRole(application.SpeechMenu)
	} else {
		m.AddRole(application.Delete)
		m.AddSeparator()
		m.AddRole(application.SelectAll)
	}
}
