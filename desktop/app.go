// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"cmp"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/workspace"
)

// Mode is how the app is hosted.
type Mode int

// Modes.
const (
	ModeDesktop Mode = iota // a native window
	ModeServer              // loopback HTTP for a browser (-tags server)
	ModeHarness             // test-only HTTP host (-tags "server e2eharness")
)

// Host is what services are built from.
type Host struct {
	Mode Mode
	// Root is the project folder given with --root; empty until the user
	// opens one.
	Root string
	Dirs *appdirs.Dirs
	// Emit sends app events to the frontend (Wails events in the window,
	// the guarded event stream over HTTP).
	Emit emit.Emitter

	// Shared by the services; built by setup.
	Workspace *workspace.Workspace
	Handles   *handles.Table
}

// setup builds the parts services share, once Emit is set, and opens Root
// when given.
func (h *Host) setup() error {
	h.Handles = handles.New()
	h.Workspace = workspace.New(h.Emit)
	if h.Root != "" {
		if _, err := h.Workspace.Open(h.Root); err != nil {
			return err
		}
	}
	return nil
}

// registration is one service. Each services_*.go file registers its
// service from init, so adding or replacing a service never edits this
// file.
type registration struct {
	name  string
	modes []Mode // the modes that get the service; nil means every mode
	new   func(h *Host) application.Service
}

var registry []registration

// register adds a service for modes (nil: every mode).
func register(name string, modes []Mode, fn func(h *Host) application.Service) {
	registry = append(registry, registration{name: name, modes: modes, new: fn})
}

// stub is the placeholder of a service not built yet: it binds nothing.
type stub struct{}

// registerStub registers the placeholder for a service not built yet.
func registerStub(name string) {
	register(name, nil, func(*Host) application.Service {
		return application.NewServiceWithOptions(&stub{}, application.ServiceOptions{Name: name})
	})
}

// services builds the services of h's mode, ordered by name.
func services(h *Host) []application.Service {
	regs := slices.SortedFunc(slices.Values(registry), func(a, b registration) int { return cmp.Compare(a.name, b.name) })
	var out []application.Service
	for _, r := range regs {
		if r.modes == nil || slices.Contains(r.modes, h.Mode) {
			out = append(out, r.new(h))
		}
	}
	return out
}

// appOptions are the options every mode shares.
func appOptions(h *Host) application.Options {
	return application.Options{
		Name:        "Sonde",
		Description: "Reads and runs .hurl files",
		Services:    services(h),
		// Coded errors reach the page as {code, message}.
		MarshalError: apperr.Marshal,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	}
}
