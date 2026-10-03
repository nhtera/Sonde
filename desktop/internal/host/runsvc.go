// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
)

func init() {
	register("runsvc", nil, func(h *Host) application.Service {
		return application.NewServiceWithOptions(runsvc.NewService(h.Runs), application.ServiceOptions{Name: "runsvc"})
	})
	// Reports are written to a folder picked in the window's dialog.
	register("reports", []Mode{ModeDesktop}, func(h *Host) application.Service {
		return application.NewServiceWithOptions(&Reports{runs: h.Runs, h: h.Handles}, application.ServiceOptions{Name: "reports"})
	})
}

// Reports writes a test run's reports into a folder the user picked.
type Reports struct {
	runs *runsvc.Runs
	h    *handles.Table
}

// Export writes test run runID's report in format (html, json, junit,
// tap) into the folder of dir, a folder dialog's handle; it returns what
// it wrote.
func (r *Reports) Export(runID, format, dir string) (string, error) {
	path, err := r.h.Take(dir, handles.OpenDir)
	if err != nil {
		return "", apperr.Wrap(apperr.Expired, err)
	}
	return r.runs.Export(runID, format, path)
}
