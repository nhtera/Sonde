// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build e2eharness

package update

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/nhtera/sonde/desktop/internal/closeguard"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/settings"
	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

// Script drives the e2e harness's update service: the real Manager and
// Service, on scripted parts (what a check finds, the download, the
// installer, the other windows). Test-only.
type Script struct {
	m *Manager

	mu       sync.Mutex
	scenario string
	others   int
	install  bool
	installs int
}

// Scenarios a check can meet.
var scenarios = map[string]bool{
	"available": true, "up-to-date": true, "verification": true, "offline": true,
	"incomplete": true, "cannot-install": true, "others": true,
}

// NewHarness returns a Manager of version 0.2.0, attached to scripted
// parts and idle, and the Script that drives it.
func NewHarness(st *settings.Store, e emit.Emitter, g *closeguard.Guard) (*Manager, *Script) {
	s := &Script{scenario: "available", install: true}
	m := New("0.2.0", st, e, g)
	s.m = m
	p := &provider{}
	m.current, _ = parseVersion("0.2.0")
	m.engine, m.testServer = "v1.3.1+0000000", false
	m.feed, m.prov, m.up, m.inst = s, p, &scriptUpdater{p: p}, s
	m.instances = s.othersOpen
	m.openURL = func(string) error { return nil }
	m.st = m.base(StateIdle)
	return m, s
}

// Set picks what the next checks meet.
func (s *Script) Set(scenario string) error {
	if !scenarios[scenario] {
		return fmt.Errorf("unknown update scenario %q", scenario)
	}
	s.mu.Lock()
	s.scenario = scenario
	s.install = scenario != "cannot-install"
	s.others = 0
	if scenario == "others" {
		s.others = 2
	}
	s.mu.Unlock()
	// The installer's answer is part of every status.
	s.m.mu.Lock()
	seq := s.m.st.Seq
	st := s.m.st
	b := s.m.base(st.State)
	st.CanInstall, st.Reason, st.Seq = b.CanInstall, b.Reason, seq
	s.m.st = st
	s.m.mu.Unlock()
	return nil
}

// Background runs a background check, as the daily schedule does.
func (s *Script) Background() { s.m.Check(false) }

// latest answers a check as the scenario says.
func (s *Script) latest(context.Context, string, version) (*found, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.scenario {
	case "up-to-date":
		return nil, nil
	case "verification":
		return nil, &Error{Kind: KindVerification, Err: fmt.Errorf("%w: the signature does not match key \"k1\"", manifest.ErrVerification)}
	case "offline":
		return nil, &Error{Kind: KindNetwork, Err: errors.New("dial tcp: lookup api.github.com: no such host")}
	case "incomplete":
		return nil, &Error{Kind: KindRelease, Err: errors.New("the release has no Sonde-Desktop-0.2.1.update.json yet")}
	}
	return &found{
		manifest: manifest.Manifest{Version: "0.2.1", Notes: "Sonde Desktop 0.2.1\n\n- Faster runs.\n- <b>Not bold</b>: notes are text."},
		artifact: manifest.Artifact{Platform: "linux", Arch: "amd64", Filename: "Sonde-Desktop-0.2.1-linux-x86_64.AppImage", Size: 1000},
	}, nil
}

func (s *Script) othersOpen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.others
}

// CanInstall is the scripted installer's answer.
func (s *Script) CanInstall() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.install {
		return false, reasonNotOwned
	}
	return true, ""
}

// Install records the hand-off; nothing quits in the harness.
func (s *Script) Install(Record) error {
	s.mu.Lock()
	s.installs++
	s.mu.Unlock()
	return nil
}

// Installs counts the hand-offs.
func (s *Script) Installs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.installs
}

// scriptUpdater downloads in ten steps, for the page's progress.
type scriptUpdater struct{ p *provider }

func (u *scriptUpdater) Init(updater.Config) error { return nil }

func (u *scriptUpdater) Check(context.Context) (*updater.Release, error) {
	f, _ := u.p.current()
	return &updater.Release{Version: f.manifest.Version}, nil
}

func (u *scriptUpdater) DownloadAndInstall(ctx context.Context) error {
	f, progress := u.p.current()
	for w := f.artifact.Size / 10; w <= f.artifact.Size; w += f.artifact.Size / 10 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(60 * time.Millisecond):
		}
		progress(w, f.artifact.Size)
	}
	return nil
}

func (u *scriptUpdater) DownloadedPath() string {
	return "/staged/Sonde-Desktop-0.2.1-linux-x86_64.AppImage"
}
