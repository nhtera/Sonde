// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build e2eharness

package host

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/update"
)

// harnessUpdates drives the harness's update service.
var harnessUpdates *update.Script

func init() {
	// The real update Service on scripted parts, so the page's bindings
	// are the window app's own.
	register("update", []Mode{ModeHarness}, func(h *Host) application.Service {
		m, s := update.NewHarness(h.Settings, h.Emit, h.Guard)
		harnessUpdates = s
		return application.NewServiceWithOptions(update.NewService(m), application.ServiceOptions{Name: "update"})
	})
}

// UpdateScenario picks what the update checks meet next: available,
// up-to-date, verification, offline, incomplete, cannot-install, others.
func (s *HarnessService) UpdateScenario(name string) error {
	if harnessUpdates == nil {
		return errors.New("no update service")
	}
	return harnessUpdates.Set(name)
}

// UpdateBackgroundCheck runs a check as the daily schedule does.
func (s *HarnessService) UpdateBackgroundCheck() {
	if harnessUpdates != nil {
		harnessUpdates.Background()
	}
}

// UpdateInstalls counts the updates handed to the installer.
func (s *HarnessService) UpdateInstalls() int {
	if harnessUpdates == nil {
		return 0
	}
	return harnessUpdates.Installs()
}
