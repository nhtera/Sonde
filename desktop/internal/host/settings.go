// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/history"
	"github.com/nhtera/sonde/desktop/internal/jar"
	"github.com/nhtera/sonde/desktop/internal/settings"
)

func init() {
	register("settings", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(settings.NewService(h.Settings), application.ServiceOptions{Name: "settings"})
	})
	register("jar", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(jar.NewService(h.Jars), application.ServiceOptions{Name: "jar"})
	})
	register("history", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(history.NewService(h.History), application.ServiceOptions{Name: "history"})
	})
}
