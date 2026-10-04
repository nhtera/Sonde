// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/update"
)

func init() {
	// The window app updates itself; server mode is updated with the
	// binary that runs it. Untagged, so the bindings (generated with
	// -tags server) include it.
	register("update", []Mode{ModeDesktop}, func(h *Host) application.Service {
		return application.NewServiceWithOptions(update.NewService(h.Update), application.ServiceOptions{Name: "update"})
	})
}
