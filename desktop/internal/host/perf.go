// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/perftrace"
)

func init() {
	register("perf", []Mode{ModeDesktop}, func(h *Host) application.Service {
		t := h.Perf
		if t == nil {
			t = perftrace.New("", "", time.Now(), nil)
		}
		return application.NewServiceWithOptions(perftrace.NewService(t), application.ServiceOptions{Name: "perf"})
	})
}
