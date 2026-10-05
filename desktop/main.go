// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !server

// Command sonde-desktop is the Sonde desktop app: it reads and runs .hurl
// files. Built with -tags server it is a loopback HTTP server for a
// browser instead (see main_server.go).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/host"
	"github.com/nhtera/sonde/desktop/internal/perftrace"
	"github.com/nhtera/sonde/desktop/internal/settings"
	"github.com/nhtera/sonde/desktop/internal/update"
)

// engine is the CLI engine the app is built on, "v1.3.1+a7404ac" (set at
// build time with -X main.engine, from scripts/engine.mjs).
var engine = "dev"

func main() {
	// First: started by an update to swap the app's files, this process is
	// the updater's helper and exits here, before any of Sonde's startup.
	updater.HandleHelperMode()
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
	// Hidden, for testing releases (docs/desktop.md › window flags).
	channel := fs.String("update-channel", "", "save the update channel: stable or prerelease")
	updateAPI := fs.String("update-api", "", "a test release server for this run (https, or http on loopback)")
	installCheck := fs.Bool("update-install-check", false, "print whether this copy can install updates in place, then quit")
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
	// For CI and support: the in-place install check, without a window.
	if *installCheck {
		if ok, reason := update.NewInstaller(update.InstallerDeps{}).CanInstall(); !ok {
			return errors.New(reason)
		}
		fmt.Println("this copy can install updates in place")
		return nil
	}
	dirs, err := openDirs(*data)
	if err != nil {
		return err
	}
	defer dirs.Close()
	// Restart refuses while another Sonde window runs this copy: the
	// locks live in the user's own app data, whatever --data says.
	others := func() int { return 0 }
	if inst, err := update.LockInstance(); err != nil {
		fmt.Fprintln(os.Stderr, "sonde-desktop: instance lock:", err)
	} else {
		defer inst.Close()
		others = inst.Others
	}
	if *channel != "" && *channel != settings.ChannelStable && *channel != settings.ChannelPrerelease {
		return fmt.Errorf("--update-channel %q: stable or prerelease", *channel)
	}
	if *root != "" {
		if *root, err = projectRoot(*root); err != nil {
			return err
		}
	}
	h := &host.Host{Mode: host.ModeDesktop, Version: version, Root: *root, Dirs: dirs, Emit: emit.Wails{}}
	var app *application.App
	// The tour types into a file: its quit leaves the edits unsaved, unasked.
	h.Perf = perftrace.New(*trace, *tour, start, func() { h.Guard.Leave() })
	if err := h.Setup(); err != nil {
		return err
	}
	if *channel != "" {
		// Saved, so the app relaunched by an update keeps it.
		if err := h.Settings.SetUpdateState(func(u *settings.Updates) { u.Channel = *channel }); err != nil {
			return err
		}
	}
	appOpts := appOptions(h)
	// ⌘Q and the menu's Quit wait for the page while edits are unsaved.
	appOpts.ShouldQuit = func() bool { return !h.Guard.Hold() }
	app = application.New(appOpts)
	h.Guard.Quit = app.Quit
	if err := h.Update.Attach(update.AttachOptions{
		Updater: app.Updater, Engine: engine, API: *updateAPI,
		Installer: update.NewInstaller(update.InstallerDeps{Quit: app.Quit, Restart: app.Updater.Restart}),
		OpenURL:   app.Browser.OpenURL, Others: others,
	}); err != nil {
		return err
	}
	defer h.Update.Stop()
	// A measured run (--perf-trace) checks nothing: its timings stay its own.
	if *trace == "" {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { h.Update.Start() })
	}
	app.Event.OnApplicationEvent(events.Common.SystemDidWake, func(*application.ApplicationEvent) { h.Update.Wake() })
	app.Menu.SetApplicationMenu(appMenu(func() { go h.Update.Check(true) }))
	opts := application.WebviewWindowOptions{
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
	}
	// A Manual theme's panel color shows until the page paints. With Sync
	// the OS's look is unknown before Run: the default stays.
	if c, ok := background(h.Settings.Get()); ok {
		opts.BackgroundColour = c
	}
	win := app.Window.NewWithOptions(opts)
	// So do the close button and Close Window (⇧⌘W).
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if h.Guard.Hold() {
			e.Cancel()
		}
	})
	// The UI font size is the webview's zoom: every measure scales alike
	// (menus, resizers, the editor), unlike a page's CSS zoom.
	zoom := func(s settings.Settings) {
		if n := s.Appearance.UIFontSize; n > 0 {
			win.SetZoom(float64(n) / 13)
		}
	}
	zoom(h.Settings.Get())
	// Changes come one at a time, in the order they were saved.
	h.Settings.Changed = func(s settings.Settings) {
		zoom(s)
		if c, ok := background(s); ok {
			win.SetBackgroundColour(c)
		}
	}
	return app.Run()
}

// background is the window color of a Manual theme; false with Sync.
func background(s settings.Settings) (application.RGBA, bool) {
	t, ok := settings.ThemeByID(s.Appearance.Theme)
	if !ok {
		return application.RGBA{}, false
	}
	c := t.Background
	return application.NewRGB(uint8(c>>16&0xff), uint8(c>>8&0xff), uint8(c&0xff)), true
}
