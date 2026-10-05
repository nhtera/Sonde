// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/nhtera/sonde/desktop/internal/closeguard"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/desktop/internal/settings"
	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

// fakeFinder answers checks with a release or an error.
type fakeFinder struct {
	mu    sync.Mutex
	f     *found
	err   error
	calls int
}

func (f *fakeFinder) latest(context.Context, string, version) (*found, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.f, f.err
}

func (f *fakeFinder) set(v string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.f, f.err = nil, err
	if v != "" {
		f.f = &found{
			manifest: manifest.Manifest{Version: v, Notes: "notes " + v},
			artifact: manifest.Artifact{Platform: "linux", Arch: "amd64", Filename: "Sonde-Desktop-" + v + "-linux-x86_64.AppImage", Size: 300},
		}
	}
}

func (f *fakeFinder) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeUpdater downloads by reporting progress through the provider.
type fakeUpdater struct {
	p   *provider
	err error
}

func (u *fakeUpdater) Init(updater.Config) error { return nil }
func (u *fakeUpdater) Check(context.Context) (*updater.Release, error) {
	f, _ := u.p.current()
	return &updater.Release{Version: f.manifest.Version}, nil
}
func (u *fakeUpdater) DownloadAndInstall(context.Context) error {
	f, progress := u.p.current()
	for w := int64(100); w <= f.artifact.Size; w += 100 {
		progress(w, f.artifact.Size)
	}
	return u.err
}
func (u *fakeUpdater) DownloadedPath() string { return "/staged/new" }

type fakeInstaller struct {
	can      bool
	err      error
	installs []Record
}

func (i *fakeInstaller) CanInstall() (bool, string) {
	if i.can {
		return true, ""
	}
	return false, "a copy this app cannot replace"
}
func (i *fakeInstaller) Install(r Record) error { i.installs = append(i.installs, r); return i.err }

type rig struct {
	m      *Manager
	finder *fakeFinder
	up     *fakeUpdater
	inst   *fakeInstaller
	rec    *emit.Recorder
	st     *settings.Store
	guard  *closeguard.Guard
	now    time.Time
	others int
	timers []func()
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{finder: &fakeFinder{}, inst: &fakeInstaller{can: true}, rec: &emit.Recorder{}, now: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
	r.st = settings.Open(sandboxtest.Open(t, t.TempDir()), &emit.Recorder{}, handles.New())
	r.guard = &closeguard.Guard{}
	r.m = New("0.2.0", r.st, r.rec, r.guard)
	r.m.now = func() time.Time { return r.now }
	r.m.after = func(_ time.Duration, f func()) { r.timers = append(r.timers, f) }
	p := &provider{}
	r.up = &fakeUpdater{p: p}
	r.m.current, r.m.feed, r.m.prov, r.m.up, r.m.inst = mustVersion(t, "0.2.0"), r.finder, p, r.up, r.inst
	r.m.instances = func() int { return r.others }
	r.m.st = r.m.base(StateIdle)
	return r
}

// install runs Install and waits for its download to end.
func (r *rig) install(t *testing.T) {
	t.Helper()
	if err := r.m.Install(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for s := r.m.Status().State; s == StateDownloading || s == StateVerifying; s = r.m.Status().State {
		if time.Now().After(deadline) {
			t.Fatal("the download never ended")
		}
		time.Sleep(time.Millisecond)
	}
}

func (r *rig) statuses() []Status {
	var out []Status
	for _, e := range r.rec.Events() {
		if e.Topic == TopicState {
			out = append(out, e.Data.(Status))
		}
	}
	return out
}

func TestSchedule(t *testing.T) {
	r := newRig(t)
	if !r.m.due() {
		t.Fatal("never checked: a check is due")
	}
	r.finder.set("", nil)
	r.m.CheckIfDue()
	if r.finder.count() != 1 || r.st.Get().Updates.LastCheck == "" {
		t.Fatalf("calls %d, lastCheck %q", r.finder.count(), r.st.Get().Updates.LastCheck)
	}
	r.now = r.now.Add(23 * time.Hour)
	r.m.CheckIfDue()
	if r.finder.count() != 1 {
		t.Fatal("checked again within 24 h")
	}
	r.now = r.now.Add(3 * 24 * time.Hour) // slept 3 days: the wall clock counts
	r.m.CheckIfDue()
	if r.finder.count() != 2 {
		t.Fatal("no check after 3 days asleep")
	}
	r.now = r.now.Add(-48 * time.Hour) // the clock was set back
	r.m.CheckIfDue()
	if r.finder.count() != 3 {
		t.Fatal("no check with the last one in the future")
	}

	// Offline: the next hourly tick tries again (lastCheck unchanged).
	r.now = r.now.Add(72 * time.Hour)
	last := r.st.Get().Updates.LastCheck
	r.finder.set("", &Error{Kind: KindNetwork, Err: errors.New("offline")})
	r.m.CheckIfDue()
	if r.st.Get().Updates.LastCheck != last || !r.m.due() {
		t.Fatal("an offline check counted as done")
	}

	// The switch off: no request on its own, but the menu still checks.
	if err := r.st.SetUpdateState(func(*settings.Updates) {}); err != nil {
		t.Fatal(err)
	}
	s := r.st.Get()
	s.Updates.Check = false
	if _, err := r.st.Set(s); err != nil {
		t.Fatal(err)
	}
	n := r.finder.count()
	for range 120 { // an hourly tick, 120 times over
		r.now = r.now.Add(time.Hour)
		r.m.CheckIfDue()
	}
	if r.finder.count() != n {
		t.Fatal("the switch is off but a background check ran")
	}
	r.m.Check(true)
	if r.finder.count() != n+1 {
		t.Fatal("Help › Check for Updates did not check")
	}
}

func TestBusyStatesSkipChecks(t *testing.T) {
	r := newRig(t)
	r.finder.set("0.2.1", nil)
	r.m.Check(false)
	r.install(t)
	if s := r.m.Status(); s.State != StateReady || s.Version != "0.2.1" {
		t.Fatalf("status %+v, want ready 0.2.1", s)
	}
	r.finder.set("0.2.2", nil)
	n := r.finder.count()
	r.now = r.now.Add(48 * time.Hour)
	r.m.CheckIfDue()
	r.m.Check(true)
	if r.finder.count() != n {
		t.Fatal("a check ran while an update is ready")
	}
	if s := r.m.Status(); s.State != StateReady || s.Version != "0.2.1" || !s.Manual {
		t.Fatalf("a manual check while ready: %+v; want ready 0.2.1, shown", s)
	}
	// Restart installs the recorded release, not the newer find.
	if err := r.m.Restart(); err != nil {
		t.Fatal(err)
	}
	if len(r.inst.installs) != 1 || r.inst.installs[0].Version != "0.2.1" || r.inst.installs[0].Staged != "/staged/new" {
		t.Fatalf("installed %+v", r.inst.installs)
	}
}

func TestSkip(t *testing.T) {
	r := newRig(t)
	r.finder.set("0.2.1", nil)
	r.m.Check(false)
	if err := r.m.Skip("0.2.0"); err == nil {
		t.Fatal("skipped a version not on offer")
	}
	if err := r.m.Skip("0.2.1"); err != nil {
		t.Fatal(err)
	}
	if r.st.Get().Updates.Skipped != "0.2.1" || r.m.Status().State != StateIdle {
		t.Fatalf("after Skip: %+v, %+v", r.st.Get().Updates, r.m.Status())
	}
	r.m.Check(false)
	if s := r.m.Status(); s.State != StateUpToDate {
		t.Fatalf("a background check offered the skipped version: %+v", s)
	}
	r.m.Check(true)
	if s := r.m.Status(); s.State != StateAvailable || !s.Skipped || s.Version != "0.2.1" {
		t.Fatalf("a manual check hid the skipped version: %+v", s)
	}
	if err := r.m.ClearSkip(); err != nil {
		t.Fatal(err)
	}
	if s := r.m.Status(); r.st.Get().Updates.Skipped != "" || s.Skipped || s.Manual {
		t.Fatalf("after ClearSkip: %+v; want the offer, not skipped, not a check's answer", s)
	}
}

func TestBackgroundErrors(t *testing.T) {
	r := newRig(t)
	r.finder.set("0.2.1", nil)
	r.m.Check(false)
	r.now = r.now.Add(48 * time.Hour)
	r.finder.set("", &Error{Kind: KindNetwork, Err: errors.New("offline")})
	r.m.Check(false)
	if s := r.m.Status(); s.State != StateAvailable || s.Version != "0.2.1" || s.Error != "" {
		t.Fatalf("offline in the background: %+v; want yesterday's offer, silently", s)
	}
	r.finder.set("", &Error{Kind: KindVerification, Err: manifest.ErrVerification})
	r.m.Check(false)
	if s := r.m.Status(); s.State != StateError || s.ErrorKind != KindVerification || s.Manual {
		t.Fatalf("a verification failure in the background: %+v; want it shown", s)
	}
	r.finder.set("", &Error{Kind: KindNetwork, Err: errors.New("offline")})
	r.m.Check(true)
	if s := r.m.Status(); s.State != StateError || s.ErrorKind != KindNetwork || !s.Manual {
		t.Fatalf("offline on a manual check: %+v; want an answer", s)
	}
}

func TestSeqAndLateProgress(t *testing.T) {
	r := newRig(t)
	r.finder.set("0.2.1", nil)
	r.m.Check(true)
	r.install(t)
	before := len(r.statuses())
	r.m.progress(r.m.attempt, 100, 300) // a straggler after the download ended
	r.m.progress(r.m.attempt-1, 100, 300)
	if len(r.statuses()) != before || r.m.Status().State != StateReady {
		t.Fatal("progress after the download changed the state")
	}
	var last uint64
	sawVerifying := false
	for _, s := range r.statuses() {
		if s.Seq <= last {
			t.Fatalf("seq %d after %d", s.Seq, last)
		}
		last = s.Seq
		sawVerifying = sawVerifying || s.State == StateVerifying
	}
	if !sawVerifying {
		t.Fatal("no verifying state between the download and ready")
	}
}

func TestRestartGuards(t *testing.T) {
	r := newRig(t)
	r.finder.set("0.2.1", nil)
	r.m.Check(true)
	r.install(t)
	r.guard.SetUnsaved(1)

	r.others = 2
	if err := r.m.Restart(); err == nil || len(r.inst.installs) != 0 {
		t.Fatal("restarted with two other windows open")
	}
	if s := r.m.Status(); s.OtherInstances != 2 {
		t.Fatalf("status %+v, want 2 other instances", s)
	}
	r.others = 0
	if err := r.m.Restart(); err != nil {
		t.Fatal(err)
	}
	if r.guard.Hold() {
		t.Fatal("the update's quit is held by the close guard")
	}
	if err := r.m.Restart(); err == nil || len(r.inst.installs) != 1 {
		t.Fatalf("a second Restart handed the update over again: %d installs", len(r.inst.installs))
	}
	// The process is still there 45 s later: the watchdog.
	if len(r.timers) != 1 {
		t.Fatalf("%d timers, want the watchdog", len(r.timers))
	}
	r.timers[0]()
	if !r.guard.Hold() {
		t.Fatal("the watchdog did not restore the close guard")
	}
	if s := r.m.Status(); s.State != StateError || s.ErrorKind != KindInstall || !contains(s.Error, "still on 0.2.0") {
		t.Fatalf("status %+v", s)
	}

	t.Run("a failed hand-off", func(t *testing.T) {
		r := newRig(t)
		r.finder.set("0.2.1", nil)
		r.m.Check(true)
		r.install(t)
		r.guard.SetUnsaved(1)
		r.inst.err = errors.New("no permission")
		if err := r.m.Restart(); err == nil {
			t.Fatal("Restart = nil")
		}
		if !r.guard.Hold() || r.m.Status().ErrorKind != KindInstall {
			t.Fatal("a failed install left the guard open")
		}
	})
}

func TestCannotInstallHere(t *testing.T) {
	r := newRig(t)
	r.inst.can = false
	r.m.st = r.m.base(StateIdle)
	r.finder.set("0.2.1", nil)
	r.m.Check(true)
	if s := r.m.Status(); s.State != StateAvailable || s.CanInstall || s.Reason == "" {
		t.Fatalf("status %+v; want available, with the reason it cannot install", s)
	}
	if err := r.m.Install(); err == nil {
		t.Fatal("Install ran where it cannot")
	}
}

func TestDisabled(t *testing.T) {
	st := settings.Open(sandboxtest.Open(t, t.TempDir()), &emit.Recorder{}, handles.New())
	m := New("dev", st, &emit.Recorder{}, &closeguard.Guard{})
	up := &countingUpdater{}
	if err := m.Attach(AttachOptions{Updater: up}); err != nil {
		t.Fatal(err)
	}
	if m.Status().State != StateDisabled || up.inits != 0 {
		t.Fatalf("a dev build: %+v, %d inits; want disabled, the updater untouched", m.Status(), up.inits)
	}
	// No feed is attached: a check that ran would panic.
	m.Check(true)
	m.CheckIfDue()
	if s := m.Status(); s.State != StateDisabled || !s.Manual || s.Reason == "" {
		t.Fatalf("a manual check in a dev build: %+v; want the reason shown", s)
	}
}

type countingUpdater struct {
	fakeUpdater
	inits int
}

func (u *countingUpdater) Init(updater.Config) error { u.inits++; return nil }
