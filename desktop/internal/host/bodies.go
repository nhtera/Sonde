// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/bodies"
	"github.com/nhtera/sonde/desktop/internal/serverauth"
)

func init() {
	// Bodies by URL only: the route binds nothing.
	register("bodies", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(bodies.NewRoute(h.Bodies),
			application.ServiceOptions{Name: "bodies", Route: serverauth.BodyPrefix})
	})
	// Saving a response's raw bytes and opening one in another app are the
	// window app's.
	register("bodiesDesktop", []Mode{ModeDesktop}, func(h *Host) application.Service {
		return application.NewServiceWithOptions(bodies.NewDesktop(h.Bodies, pickSave),
			application.ServiceOptions{Name: "bodiesDesktop"})
	})
}

// pickSave shows the native save dialog; "" when canceled.
func pickSave(suggested string) (string, error) {
	return application.Get().Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Filename: suggested,
		Title:    "Save response",
	}).PromptForSingleSelection()
}
