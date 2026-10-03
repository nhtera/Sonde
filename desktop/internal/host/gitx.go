// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/gitx"
)

func init() {
	register("gitx", nil, func(h *Host) application.Service {
		g := gitx.New(h.Workspace.Root, h.Dirs.Config())
		g.Secret = h.Envs.IsSecretFile
		return application.NewServiceWithOptions(g, application.ServiceOptions{Name: "gitx"})
	})
}
