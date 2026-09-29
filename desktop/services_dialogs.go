// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/handles"
)

// Dialogs shows the native file dialog and gives the page a handle for
// the file, never its path: a data file for a data run, a TLS file, a
// file to import. The window app only.
type Dialogs struct{ h *handles.Table }

// OpenFile asks for a file; it returns its handle, "" when canceled.
// Patterns filter the list, e.g. "*.csv;*.json".
func (d *Dialogs) OpenFile(title, filterName, patterns string) (string, error) {
	opts := &application.OpenFileDialogOptions{CanChooseFiles: true, Title: title}
	if patterns != "" {
		opts.Filters = []application.FileFilter{{DisplayName: filterName, Pattern: patterns}}
	}
	path, err := application.Get().Dialog.OpenFileWithOptions(opts).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	return d.h.Put(path, handles.OpenFile)
}

func init() {
	register("dialogs", []Mode{ModeDesktop}, func(h *Host) application.Service {
		return application.NewServiceWithOptions(&Dialogs{h: h.Handles}, application.ServiceOptions{Name: "dialogs"})
	})
}
