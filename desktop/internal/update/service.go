// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package update keeps the window app up to date: it finds the newest
// desktop/v* release on the user's channel, verifies its signed manifest
// against the pinned keys, downloads the update file through the Wails
// updater (which checks its signed SHA-512), and hands it to the OS's
// installer when the user restarts. One state machine, here, owns every
// state the page sees.
package update

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/closeguard"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/settings"
	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

// TopicState carries each Status to the page, with a rising Seq.
const TopicState = "update:state"

// State is where the update flow is.
type State string

// States.
const (
	StateDisabled    State = "disabled" // a dev build, or no pinned key: Reason says
	StateIdle        State = "idle"
	StateChecking    State = "checking"
	StateUpToDate    State = "up-to-date"
	StateAvailable   State = "available"
	StateDownloading State = "downloading"
	StateVerifying   State = "verifying"
	StateReady       State = "ready" // downloaded and verified: Restart installs it
	StateError       State = "error"
)

// Status is the state the page shows. Seq rises with every change: the page
// drops an older one that arrives late.
type Status struct {
	Seq       uint64    `json:"seq"`
	State     State     `json:"state"`
	Version   string    `json:"version"` // the version offered, downloading or ready
	Notes     string    `json:"notes"`
	Written   int64     `json:"written"`
	Total     int64     `json:"total"`
	Error     string    `json:"error"`
	ErrorKind ErrorKind `json:"errorKind"`
	// Manual is set when the user asked for this check: its outcome is
	// shown, even "up to date" or offline.
	Manual bool `json:"manual"`
	// Skipped: Version is the one the user skipped.
	Skipped bool `json:"skipped"`
	// CanInstall is false where this copy cannot update itself in place;
	// Reason says why, and the page offers the release page instead.
	CanInstall bool   `json:"canInstall"`
	Reason     string `json:"reason"`
	// OtherInstances is how many other Sonde windows run, when Restart
	// found any.
	OtherInstances int `json:"otherInstances"`
}

// AppInfo is the app's version, its engine and where updates come from.
type AppInfo struct {
	Version    string `json:"version"`
	Engine     string `json:"engine"`
	Channel    string `json:"channel"`
	TestServer bool   `json:"testServer"`
}

// Updater is the part of the Wails updater the Manager drives: only to
// download and check the digest of a release it already verified.
type Updater interface {
	Init(updater.Config) error
	Check(context.Context) (*updater.Release, error)
	DownloadAndInstall(context.Context) error
	DownloadedPath() string
}

// Installer puts a staged update in place on this OS.
type Installer interface {
	// CanInstall reports whether this copy can update itself in place, and
	// why not.
	CanInstall() (bool, string)
	// Install hands the staged update over and starts the app's exit; the
	// close guard already lets it through.
	Install(r Record) error
}

// Record is a staged update: the release Install downloaded and where it
// is. Restart installs this one, never a later find.
type Record struct {
	Version  string
	Artifact manifest.Artifact
	Staged   string
}

// releaseFinder is the feed (a fake in tests).
type releaseFinder interface {
	latest(ctx context.Context, channel string, current version) (*found, error)
}

// Timeouts and intervals.
const (
	checkTimeout    = 30 * time.Second
	downloadTimeout = 30 * time.Minute
	checkEvery      = 24 * time.Hour
	firstCheckAfter = 5 * time.Second
	tickEvery       = time.Hour
	watchdogAfter   = 45 * time.Second
)

// Manager is the update state machine. It is not bound: the page reaches
// it through Service.
type Manager struct {
	version  string
	current  version
	settings *settings.Store
	emit     emit.Emitter
	guard    *closeguard.Guard
	now      func() time.Time
	// after runs f after d (time.AfterFunc; immediate in tests).
	after func(d time.Duration, f func())

	// Set by Attach.
	engine     string
	feed       releaseFinder
	prov       *provider
	up         Updater
	inst       Installer
	instances  func() int
	openURL    func(string) error
	testServer bool

	mu      sync.Mutex
	st      Status
	found   *found  // the verified release on offer
	record  *Record // the staged release
	attempt uint64  // the install attempt progress belongs to
	// restarting: Restart handed the update over; another Restart waits
	// for the watchdog.
	restarting bool
	stop       chan struct{}
}

// New returns a Manager, disabled until Attach.
func New(appVersion string, st *settings.Store, e emit.Emitter, g *closeguard.Guard) *Manager {
	m := &Manager{
		version: appVersion, settings: st, emit: e, guard: g,
		now: time.Now, after: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		stop: make(chan struct{}),
	}
	m.st = Status{State: StateDisabled, Reason: "Updates start with the app."}
	return m
}

// AttachOptions are what the window app gives Attach.
type AttachOptions struct {
	Updater   Updater
	Installer Installer
	Engine    string
	// API, when set (--update-api), is a test release server for this run.
	API string
	// OpenURL opens a page in the default browser.
	OpenURL func(string) error
	// Others counts the other running windows.
	Others func() int
}

// Attach configures the Wails updater with Sonde's own provider and makes
// the Manager ready to check. A dev build, or one without a pinned key,
// stays disabled and makes no request.
func (m *Manager) Attach(o AttachOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.engine, m.openURL, m.instances, m.inst = o.Engine, o.OpenURL, o.Others, o.Installer
	ep := githubEndpoints()
	if o.API != "" {
		var err error
		if ep, err = testEndpoints(o.API); err != nil {
			return err
		}
		m.testServer = true
	}
	cur, ok := parseVersion(m.version)
	if !ok {
		m.st = Status{State: StateDisabled, Reason: "Updates are off in a development build."}
		return nil
	}
	keys, err := pinnedKeys(m.version)
	if err != nil {
		m.st = Status{State: StateDisabled, Reason: "Updates are off: " + err.Error() + "."}
		return nil
	}
	client := newClient(ep, func() string { return m.settings.Get().Network.Proxy })
	m.current = cur
	m.feed = &feed{ep: ep, client: client, keys: keys, goos: runtime.GOOS, goarch: runtime.GOARCH}
	m.prov = &provider{ep: ep, client: client}
	m.up = o.Updater
	if err := m.up.Init(updater.Config{CurrentVersion: m.version, Providers: []updater.Provider{m.prov}, Window: updater.WindowNone}); err != nil {
		return err
	}
	m.st = m.base(StateIdle)
	m.emitLocked()
	return nil
}

// base is a fresh Status in state s, with what holds in every state.
func (m *Manager) base(s State) Status {
	st := Status{State: s}
	if m.inst != nil {
		st.CanInstall, st.Reason = m.inst.CanInstall()
	} else {
		st.Reason = "Sonde cannot install updates on this system yet."
	}
	return st
}

// emitLocked sends the status with the next Seq; m.mu is held.
func (m *Manager) emitLocked() {
	m.st.Seq++
	m.emit.Emit(TopicState, m.st)
}

// Status returns the current status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

// Info returns the app's version, engine and update channel.
func (m *Manager) Info() AppInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return AppInfo{Version: m.version, Engine: m.engine, Channel: m.settings.Get().Updates.Channel, TestServer: m.testServer}
}

// Start runs the schedule: a check 5 s after the window shows, then every
// hour one when due. Wake checks one when due after the computer slept.
func (m *Manager) Start() {
	go func() {
		first := time.NewTimer(firstCheckAfter)
		defer first.Stop()
		tick := time.NewTicker(tickEvery)
		defer tick.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-first.C:
			case <-tick.C:
			}
			m.CheckIfDue()
		}
	}()
}

// Stop ends the schedule.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

// Wake checks when a check is due, after the computer woke.
func (m *Manager) Wake() { go m.CheckIfDue() }

// due reports whether a background check is due: the switch is on and the
// last check is 24 h old by the wall clock (sleep counts), or in the future
// (the clock was set back).
func (m *Manager) due() bool {
	u := m.settings.Get().Updates
	if !u.Check {
		return false
	}
	last, err := time.Parse(time.RFC3339, u.LastCheck)
	if err != nil {
		return true
	}
	now := m.now()
	return now.Sub(last) >= checkEvery || last.After(now)
}

// CheckIfDue runs a background check when one is due.
func (m *Manager) CheckIfDue() {
	if m.due() {
		m.Check(false)
	}
}

// Check looks for an update. A manual check (the menu, the page) always
// runs and always gets an answer; a background one is silent unless an
// update is found or a manifest failed to verify. It returns when the
// check is done.
func (m *Manager) Check(manual bool) {
	m.mu.Lock()
	switch m.st.State {
	case StateDisabled, StateChecking, StateDownloading, StateVerifying, StateReady:
		// Nothing to start: a manual check shows the current state (the
		// reason, the check in flight, the update ready to install).
		if manual {
			m.st.Manual = true
			m.emitLocked()
		}
		m.mu.Unlock()
		return
	}
	prev := m.st
	next := m.base(StateChecking)
	next.Seq, next.Manual = m.st.Seq, manual
	m.st = next
	m.emitLocked()
	m.mu.Unlock()

	u := m.settings.Get().Updates
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	f, err := m.feed.latest(ctx, u.Channel, m.current)
	cancel()
	kind := kindOf(err)
	if err == nil || kind != KindNetwork {
		// An answer from the release source: the next one is due in 24 h.
		// Offline, the hourly tick tries again.
		_ = m.settings.SetUpdateState(func(u *settings.Updates) { u.LastCheck = m.now().UTC().Format(time.RFC3339) })
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	manual = m.st.Manual // a manual check may have joined this one
	next = m.base(StateIdle)
	next.Seq, next.Manual = m.st.Seq, manual
	switch {
	case err != nil && !manual && kind != KindVerification:
		// Silent: what was shown before stays (an offer from yesterday).
		next = prev
		next.Seq, next.Manual = m.st.Seq, false
	case err != nil:
		next.State, next.Error, next.ErrorKind = StateError, err.Error(), kind
	case f == nil:
		m.found = nil
		next.State = StateUpToDate
	case !manual && f.manifest.Version == u.Skipped:
		m.found = nil
		next.State = StateUpToDate
	default:
		m.found = f
		next.State, next.Version, next.Notes = StateAvailable, f.manifest.Version, f.manifest.Notes
		next.Total, next.Skipped = f.artifact.Size, f.manifest.Version == u.Skipped
	}
	m.st = next
	m.emitLocked()
}

// Install downloads and verifies the offered update in the background; the
// page follows its progress. Restart then installs it.
func (m *Manager) Install() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b := m.base(StateIdle); !b.CanInstall {
		return apperr.New(apperr.Invalid, b.Reason)
	}
	switch {
	case m.found == nil:
		return apperr.New(apperr.Invalid, "No update to install: check for updates first.")
	case m.st.State == StateChecking:
		return apperr.New(apperr.Busy, "Wait for the check to finish.")
	case m.st.State == StateDownloading || m.st.State == StateVerifying || m.st.State == StateReady:
		return apperr.New(apperr.Busy, "The update is already "+string(m.st.State)+".")
	}
	m.attempt++
	a, f := m.attempt, m.found
	next := m.base(StateDownloading)
	next.Seq, next.Version, next.Notes, next.Total = m.st.Seq, f.manifest.Version, f.manifest.Notes, f.artifact.Size
	m.st = next
	m.emitLocked()
	go m.install(a, f)
	return nil
}

// install runs the Wails updater on the verified release: its Check takes
// the release from Sonde's provider, its download checks the signed digest.
// The state comes from their return values, here.
func (m *Manager) install(a uint64, f *found) {
	m.prov.offer(f, func(w, t int64) { m.progress(a, w, t) })
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	_, err := m.up.Check(ctx)
	if err == nil {
		err = m.up.DownloadAndInstall(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if a != m.attempt {
		return
	}
	next := m.base(StateReady)
	next.Seq, next.Version, next.Notes, next.Total, next.Written = m.st.Seq, f.manifest.Version, f.manifest.Notes, f.artifact.Size, f.artifact.Size
	if err != nil {
		kind := kindOf(err)
		// Wails' own digest check.
		if strings.Contains(err.Error(), "digest mismatch") {
			kind = KindVerification
		}
		next.State, next.Error, next.ErrorKind, next.Manual = StateError, "The download failed: "+err.Error(), kind, true
	} else {
		m.record = &Record{Version: f.manifest.Version, Artifact: f.artifact, Staged: m.up.DownloadedPath()}
	}
	m.st = next
	m.emitLocked()
}

// progress records a download's progress, from the downloading goroutine
// in order. Progress of an earlier attempt, or after the download, is
// dropped.
func (m *Manager) progress(a uint64, written, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a != m.attempt || m.st.State != StateDownloading || total <= 0 {
		return
	}
	pct := func(w int64) int64 { return w * 100 / total }
	done := written >= total
	// One event per percent, not per 64 KiB.
	if !done && pct(written) == pct(m.st.Written) && m.st.Written > 0 {
		m.st.Written = written
		return
	}
	m.st.Written, m.st.Total = written, total
	if done {
		m.st.State = StateVerifying
	}
	m.emitLocked()
}

// Restart installs the staged update: refused while other Sonde windows
// run; the page has already asked about unsaved edits. The close guard
// lets the app quit; if it is still running 45 s later, the guard holds
// again and the page offers the release page.
func (m *Manager) Restart() error {
	m.mu.Lock()
	if m.st.State != StateReady || m.record == nil {
		m.mu.Unlock()
		return apperr.New(apperr.Invalid, "No update is ready to install.")
	}
	if m.restarting {
		m.mu.Unlock()
		return apperr.New(apperr.Busy, "Sonde is already restarting to update.")
	}
	if n := m.others(); n > 0 {
		m.st.OtherInstances = n
		m.emitLocked()
		m.mu.Unlock()
		return apperr.New(apperr.Busy, fmt.Sprintf("Quit the other Sonde windows first (%d open).", n))
	}
	rec := *m.record
	m.st.OtherInstances = 0
	m.restarting = true
	m.mu.Unlock()

	closeguard.AllowExit(m.guard)
	if err := m.inst.Install(rec); err != nil {
		closeguard.RestoreExit(m.guard)
		m.failInstall("The update could not be installed: " + err.Error())
		return apperr.Wrap(apperr.Invalid, err)
	}
	m.after(watchdogAfter, func() {
		closeguard.RestoreExit(m.guard)
		m.failInstall(fmt.Sprintf("The update did not finish. Sonde is still on %s.", m.version))
	})
	return nil
}

func (m *Manager) others() int {
	if m.instances == nil {
		return 0
	}
	return m.instances()
}

// failInstall shows an install failure; the staged update stays, and the
// page offers the release page.
func (m *Manager) failInstall(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restarting = false
	m.st.State, m.st.Error, m.st.ErrorKind, m.st.Manual = StateError, msg, KindInstall, true
	m.emitLocked()
}

// Skip stops background checks from offering version (the one on offer).
func (m *Manager) Skip(v string) error {
	onOffer := func() bool { return m.found != nil && m.found.manifest.Version == v && m.st.State == StateAvailable }
	m.mu.Lock()
	ok := onOffer()
	m.mu.Unlock()
	if !ok {
		return apperr.New(apperr.Invalid, "Only the update on offer can be skipped.")
	}
	// Saved outside m.mu: a settings change runs the window's callbacks.
	if err := m.settings.SetUpdateState(func(u *settings.Updates) { u.Skipped = v }); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if onOffer() {
		seq := m.st.Seq
		m.st = m.base(StateIdle)
		m.st.Seq = seq
		m.emitLocked()
	}
	return nil
}

// ClearSkip offers a skipped version again.
func (m *Manager) ClearSkip() error {
	if err := m.settings.SetUpdateState(func(u *settings.Updates) { u.Skipped = "" }); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.st.Skipped {
		m.st.Skipped = false
		m.emitLocked()
	}
	return nil
}

// OpenReleasePage opens the GitHub release page of the update on offer (or
// staged), or of this version: a URL built here from a version that parsed.
func (m *Manager) OpenReleasePage() error {
	m.mu.Lock()
	v := m.version
	switch {
	case m.record != nil:
		v = m.record.Version
	case m.found != nil:
		v = m.found.manifest.Version
	}
	open := m.openURL
	m.mu.Unlock()
	if _, ok := parseVersion(v); !ok || open == nil {
		return apperr.New(apperr.Invalid, "No release page for this build.")
	}
	return open(at(githubEndpoints().download, repoOwner, repoName, "releases", "tag", "desktop", "v"+v))
}

// Service is the update bindings, for the window app only.
type Service struct{ m *Manager }

// NewService returns the bindings over m.
func NewService(m *Manager) *Service { return &Service{m: m} }

// State returns the current status; later ones arrive as update:state.
func (s *Service) State() Status { return s.m.Status() }

// Check looks for an update now; the outcome arrives as update:state.
func (s *Service) Check() error {
	go s.m.Check(true)
	return nil
}

// Install downloads and verifies the update on offer.
func (s *Service) Install() error { return s.m.Install() }

// Skip stops background checks from offering version.
func (s *Service) Skip(version string) error { return s.m.Skip(version) }

// ClearSkip offers the skipped version again.
func (s *Service) ClearSkip() error { return s.m.ClearSkip() }

// Restart installs the downloaded update and restarts Sonde. The page asks
// about unsaved edits first.
func (s *Service) Restart() error { return s.m.Restart() }

// OpenReleasePage opens the release page in the default browser.
func (s *Service) OpenReleasePage() error { return s.m.OpenReleasePage() }

// Info returns the app's version, engine and update channel.
func (s *Service) Info() AppInfo { return s.m.Info() }
