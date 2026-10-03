// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/wsession"
)

func init() {
	register("wsession", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(wsession.NewService(h.Sessions), application.ServiceOptions{Name: "wsession"})
	})
}
