// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/importsvc"
)

func init() {
	register("importsvc", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(importsvc.NewAPI(h.Imports), application.ServiceOptions{Name: "importsvc"})
	})
}
