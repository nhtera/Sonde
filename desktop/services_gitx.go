// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/gitx"
)

func init() {
	register("gitx", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(gitx.New(h.Workspace.Root, h.Dirs.Config()), application.ServiceOptions{Name: "gitx"})
	})
}
