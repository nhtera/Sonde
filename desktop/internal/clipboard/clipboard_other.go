// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows && !(darwin && cgo)

package clipboard

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// writeConcealed writes plain text: this system has no concealed type.
func writeConcealed(text string) (any, error) {
	app := application.Get()
	if app == nil || !app.Clipboard.SetText(text) {
		return nil, errors.New("clipboard: unavailable")
	}
	return text, nil
}

func clearIf(token any) {
	app := application.Get()
	if app == nil {
		return
	}
	if cur, ok := app.Clipboard.Text(); ok && cur == token.(string) {
		app.Clipboard.SetText("")
	}
}
