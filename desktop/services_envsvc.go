// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/envsvc"
)

func init() {
	register("envsvc", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(envsvc.NewService(h.Envs), application.ServiceOptions{Name: "envsvc"})
	})
}
