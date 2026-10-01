// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !server

// Command sonde-desktop is the Sonde desktop app: it reads and runs .hurl
// files. Built with -tags server it is a loopback HTTP server for a
// browser instead (see main_server.go).
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/perftrace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sonde-desktop:", err)
		os.Exit(1)
	}
}

func run() error {
	start := time.Now()
	fs := flag.NewFlagSet("sonde-desktop", flag.ContinueOnError)
	root := fs.String("root", "", "open this project folder")
	trace := fs.String("perf-trace", "", "measure the performance budgets, write them to this JSON file, then quit")
	tour := fs.String("perf-tour", "{}", "the measures' parameters (JSON, from scripts/perf.mjs)")
	data := fs.String("data", "", "app data folder (default: Sonde in the user config and cache folders)")
	// macOS may pass -psn_… to an app opened from the Finder.
	var args []string
	for _, a := range os.Args[1:] {
		if !strings.HasPrefix(a, "-psn_") {
			args = append(args, a)
		}
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	dirs, err := openDirs(*data)
	if err != nil {
		return err
	}
	defer dirs.Close()
	if *root != "" {
		if *root, err = projectRoot(*root); err != nil {
			return err
		}
	}
	h := &Host{Mode: ModeDesktop, Root: *root, Dirs: dirs, Emit: emit.Wails{}}
	var app *application.App
	h.Perf = perftrace.New(*trace, *tour, start, func() { app.Quit() })
	if err := h.setup(); err != nil {
		return err
	}
	app = application.New(appOptions(h))
	app.Menu.SetApplicationMenu(appMenu())
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
