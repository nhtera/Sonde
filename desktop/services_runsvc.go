// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/runsvc"
)

func init() {
	register("runsvc", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(runsvc.NewService(h.Runs), application.ServiceOptions{Name: "runsvc"})
	})
}
