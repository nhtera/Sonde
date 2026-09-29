// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !server

// Command sonde-desktop is the Sonde desktop app: it reads and runs .hurl
// files. Built with -tags server it is a loopback HTTP server for a
// browser instead (see main_server.go).
package main

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
	"github.com/nhtera/sonde/desktop/internal/emit"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sonde-desktop:", err)
		os.Exit(1)
	}
}

func run() error {
	dirs, err := appdirs.Default()
	if err != nil {
		return err
	}
	defer dirs.Close()
	h := &Host{Mode: ModeDesktop, Dirs: dirs, Emit: emit.Wails{}}
	if err := h.setup(); err != nil {
		return err
	}
	app := application.New(appOptions(h))
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Sonde",
		Width:     1280,
		Height:    800,
		MinWidth:  900,
		MinHeight: 560,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 38,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})
	return app.Run()
}
