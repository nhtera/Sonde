// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"cmp"
	"slices"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/bodies"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/envsvc"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/history"
	"github.com/nhtera/sonde/desktop/internal/jar"
	"github.com/nhtera/sonde/desktop/internal/mocksvc"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
	"github.com/nhtera/sonde/desktop/internal/settings"
	"github.com/nhtera/sonde/desktop/internal/workspace"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
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
	Runs      *runsvc.Runs
	Bodies    *bodies.Store
	Settings  *settings.Store
	Jars      *jar.Jars
	History   *history.History
	Envs      *envsvc.Envs
	Mocks     *mocksvc.Mocks
}

// version is the app's version (set at build time); it names the default
// User-Agent of runs, as the CLI's does.
var version = "dev"

// setup builds the parts services share, once Emit is set, and opens Root
// when given.
func (h *Host) setup() error {
	h.Handles = handles.New()
	h.Workspace = workspace.New(h.Emit)
	h.Bodies = bodies.New(h.Dirs.Cache())
	h.Settings = settings.Open(h.Dirs.Config(), h.Emit, h.Handles)
	h.Jars = jar.New(h.Dirs.Config(), h.Workspace.Root, func() bool { return h.Settings.Get().Cookies.Keep })
	h.History = history.New(h.Dirs.Config(), h.Workspace.Root, h.historyPolicy, h.Emit.Emit)
	env := config.FromOSEnviron()
	h.Envs = envsvc.New(h.Emit, h.Dirs.Config(), h.Workspace.Root, env, version, h.Settings.Apply)
	h.Mocks = mocksvc.New(h.Emit.Emit, h.Workspace.Root, h.Envs.SetMock)
	h.Runs = runsvc.New(h.Emit, h.Workspace.Root, env, version, h.Bodies, h.Handles)
	h.Runs.Hooks = runsvc.Hooks{
		Extend:      h.Envs.Extend,
		Overrides:   h.Envs.Digest,
		KeptJar:     h.Jars.KeptJar,
		KeepCookies: h.Jars.Keep,
		Record: func(s *runsvc.Summary, results []*engine.UnitResult) {
			h.History.Add(s, results)
			h.Mocks.Observe(results)
		},
	}
	if h.Root != "" {
		if _, err := h.Workspace.Open(h.Root); err != nil {
			return err
		}
	}
	return nil
}

// historyPolicy reads the history settings.
func (h *Host) historyPolicy() history.Policy {
	s := h.Settings.Get().History
	p := history.Policy{Enabled: s.Enabled}
	switch s.Retention {
	case "7d":
		p.Retention = 7 * 24 * time.Hour
	case "30d":
		p.Retention = 30 * 24 * time.Hour
	}
	return p
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
