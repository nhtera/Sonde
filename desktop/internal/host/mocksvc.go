// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/mocksvc"
)

func init() {
	register("mocksvc", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(mocksvc.NewService(h.Mocks), application.ServiceOptions{Name: "mocksvc"})
	})
}
