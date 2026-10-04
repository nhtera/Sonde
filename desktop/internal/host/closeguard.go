// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	// Unsaved edits hold the window's close and the app's quit back; a
	// browser tab (server mode) asks through beforeunload instead.
	register("closeGuard", []Mode{ModeDesktop}, func(h *Host) application.Service {
		return application.NewServiceWithOptions(h.Guard, application.ServiceOptions{Name: "closeGuard"})
	})
}
