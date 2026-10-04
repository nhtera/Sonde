// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package settings keeps the app's settings in a versioned settings.json
// in the app's config folder. Settings that are CLI flags (network, TLS,
// cookies) apply to every run as those flags, and are listed among the
// run's overrides.
package settings

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Version is the settings file's current version.
const Version = 1

// File is the settings file in the app's config folder.
const File = "settings.json"

// TopicChanged is sent with the new Settings after a change.
const TopicChanged = "settings:changed"

// Settings are the app's settings.
type Settings struct {
	Version    int               `json:"version"`
	Appearance Appearance        `json:"appearance"`
	Shortcuts  map[string]string `json:"shortcuts"`
	Network    Network           `json:"network"`
	TLS        TLS               `json:"tls"`
	Cookies    Cookies           `json:"cookies"`
	History    History           `json:"history"`
	Contract   Contract          `json:"contract"`
	Updates    Updates           `json:"updates"`
}

// Appearance is the look of the app.
type Appearance struct {
	// Theme is "system" (DayTheme while the OS is light, NightTheme
	// while it is dark) or a theme id (Themes), always used.
	Theme        string `json:"theme"`
	DayTheme     string `json:"dayTheme"`
	NightTheme   string `json:"nightTheme"`
	UIFontSize   int    `json:"uiFontSize"`
	CodeFontSize int    `json:"codeFontSize"`
	// SideWidth and ResultsWidth are the split panes' widths in CSS pixels.
	SideWidth    int `json:"sideWidth"`
	ResultsWidth int `json:"resultsWidth"`
	// ResultsTop is the height of the results' request list in CSS
	// pixels, dragged from its lower edge; 0 sizes it to its rows (at
	// most about half the panel).
	ResultsTop int `json:"resultsTop"`
	// UIFontSize scales the window app's page from 13 (the webview's own
	// zoom); Ligatures draws the code font's ligatures (off: "==" stays
	// "==").
	Ligatures bool `json:"ligatures"`
}

// Network settings are --proxy, --connect-timeout and --retry.
type Network struct {
	Proxy          string `json:"proxy"`
	ConnectTimeout string `json:"connectTimeout"` // a duration ("10s"), "" for the default
	Retry          int    `json:"retry"`
}

// TLS settings are --cacert, --cert and --key, set only from files the
// user picked (SetTLSFile; the page sees their names), and --insecure:
// SkipVerify turns certificate verification off for every run.
type TLS struct {
	CACert     string `json:"cacert"`
	Cert       string `json:"cert"`
	Key        string `json:"key"`
	SkipVerify bool   `json:"skipVerify"`
}

// Cookies: Keep keeps a file's cookie jar between full runs.
type Cookies struct {
	Keep bool `json:"keep"`
}

// History settings.
type History struct {
	Enabled   bool   `json:"enabled"`
	Retention string `json:"retention"` // "7d", "30d" or "forever"
}

// Contract: Check validates responses against the project's OpenAPI spec.
type Contract struct {
	Check bool `json:"check"`
}

// Update channels: stable takes releases without a prerelease part;
// prerelease takes them all.
const (
	ChannelStable     = "stable"
	ChannelPrerelease = "prerelease"
)

// Updates: Check looks for a new release once a day. The rest is the
// update service's, set with SetUpdateState: a page's Set never changes it.
type Updates struct {
	Check bool `json:"check"`
	// Skipped is the version the user skipped; background checks do not
	// offer it.
	Skipped string `json:"skipped"`
	Channel string `json:"channel"`
	// LastCheck is when the last check completed (RFC 3339), wall clock.
	LastCheck string `json:"lastCheck"`
}

// Defaults are the settings of a new install.
func Defaults() Settings {
	return Settings{
		Version:    Version,
		Appearance: Appearance{Theme: "system", DayTheme: "light", NightTheme: "dark", UIFontSize: 13, CodeFontSize: 13, SideWidth: 248, ResultsWidth: 440},
		Shortcuts:  map[string]string{},
		History:    History{Enabled: true, Retention: "30d"},
		Updates:    Updates{Check: true, Channel: ChannelStable},
	}
}

// Store is the settings, loaded once and saved on each change.
type Store struct {
	config  *sandbox.Root
	emit    emit.Emitter
	handles *handles.Table

	// order serializes changes, so their events and Changed calls come
	// in the order the changes were saved; mu guards s.
	order sync.Mutex
	mu    sync.Mutex
	s     Settings
	// refused: a settings file is there but was not read (a later
	// version's, or damaged). Only a user's change replaces it.
	refused bool

	// Changed, when set, is called with the settings after each change
	// (the window app sets its zoom from the UI font size).
	Changed func(Settings)
}

// Open loads the settings from config (the defaults when there are none,
// or the file is unreadable: it is then replaced on the next change). A
// group the file lacks (an older version's) keeps its defaults; a file
// without a known "version" is not read at all.
func Open(config *sandbox.Root, e emit.Emitter, h *handles.Table) *Store {
	st := &Store{config: config, emit: e, handles: h, s: Defaults()}
	if data, err := config.ReadFile(File); err == nil {
		// The version from the file itself: the defaults carry one.
		var probe struct {
			Version *int `json:"version"`
		}
		s := Defaults()
		if json.Unmarshal(data, &probe) == nil && probe.Version != nil && *probe.Version >= 1 && *probe.Version <= Version &&
			json.Unmarshal(data, &s) == nil {
			st.s = normalize(s)
		} else {
			st.refused = true
		}
	}
	return st
}

// Get returns the settings.
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return clone(st.s)
}

// Set replaces the settings, except the TLS files (SetTLSFile).
func (st *Store) Set(s Settings) (Settings, error) {
	if err := validate(s); err != nil {
		return Settings{}, err
	}
	st.order.Lock()
	defer st.order.Unlock()
	st.mu.Lock()
	// The files only through SetTLSFile; verification here.
	skip := s.TLS.SkipVerify
	s.TLS = st.s.TLS
	s.TLS.SkipVerify = skip
	// The update state only through SetUpdateState; the switch here.
	check := s.Updates.Check
	s.Updates = st.s.Updates
	s.Updates.Check = check
	s.Version = Version
	s = normalize(s)
	prev := st.s
	st.s = s
	err := st.save()
	if err != nil {
		st.s = prev // not saved: the stored settings stay
	} else {
		st.refused = false
	}
	st.mu.Unlock()
	if err != nil {
		return Settings{}, err
	}
	st.emit.Emit(TopicChanged, forPage(s))
	if st.Changed != nil {
		st.Changed(clone(s))
	}
	return clone(s), nil
}

// TLS file kinds.
const (
	CACert = "cacert"
	Cert   = "cert"
	Key    = "key"
)

// SetTLSFile sets a TLS file from a file the user picked (handle); an
// empty handle clears it.
func (st *Store) SetTLSFile(kind, handle string) (Settings, error) {
	path := ""
	if handle != "" {
		p, err := st.handles.Take(handle, handles.OpenFile)
		if err != nil {
			return Settings{}, apperr.Wrap(apperr.Expired, err)
		}
		path = p
	}
	st.order.Lock()
	defer st.order.Unlock()
	st.mu.Lock()
	prev := st.s
	switch kind {
	case CACert:
		st.s.TLS.CACert = path
	case Cert:
		st.s.TLS.Cert = path
	case Key:
		st.s.TLS.Key = path
	default:
		st.mu.Unlock()
		return Settings{}, apperr.New(apperr.Invalid, "unknown TLS file "+kind)
	}
	s := clone(st.s)
	err := st.save()
	if err != nil {
		st.s = prev
	} else {
		st.refused = false
	}
	st.mu.Unlock()
	if err != nil {
		return Settings{}, err
	}
	st.emit.Emit(TopicChanged, forPage(s))
	return s, nil
}

// SetUpdateState changes the update service's settings (the skipped
// version, the channel, the last check) with f: the update service's only
// way to write them. While the settings file is one Open refused, the
// change stays in memory: an unattended check never replaces that file.
func (st *Store) SetUpdateState(f func(u *Updates)) error {
	st.order.Lock()
	defer st.order.Unlock()
	st.mu.Lock()
	prev := st.s
	f(&st.s.Updates)
	st.s = normalize(st.s)
	s := clone(st.s)
	var err error
	if !st.refused {
		err = st.save()
	}
	if err != nil {
		st.s = prev
	}
	st.mu.Unlock()
	if err != nil {
		return err
	}
	st.emit.Emit(TopicChanged, forPage(s))
	if st.Changed != nil {
		st.Changed(clone(s))
	}
	return nil
}

func (st *Store) save() error {
	data, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	return st.config.WriteFileAtomic(File, data, 0o600)
}

// Apply adds the settings that are CLI flags to a run's invocation, as if
// given on the command line.
func (st *Store) Apply(inv *runplan.Invocation) {
	s := st.Get()
	set := func(flag string) {
		if inv.Set == nil {
			inv.Set = map[string]bool{}
		}
		inv.Set[flag] = true
	}
	if s.Network.Proxy != "" {
		inv.Proxy = s.Network.Proxy
		set("proxy")
	}
	if s.Network.ConnectTimeout != "" {
		inv.ConnectTimeout = s.Network.ConnectTimeout
		set("connect-timeout")
	}
	if s.Network.Retry != 0 {
		inv.Retry = strconv.Itoa(s.Network.Retry)
		set("retry")
	}
	if s.TLS.CACert != "" {
		inv.CACert = s.TLS.CACert
		set("cacert")
	}
	if s.TLS.Cert != "" {
		inv.Cert = s.TLS.Cert
		set("cert")
	}
	if s.TLS.Key != "" {
		inv.Key = s.TLS.Key
		set("key")
	}
	if s.TLS.SkipVerify {
		inv.Insecure = true
		set("insecure")
	}
}

func validate(s Settings) error {
	// The theme may also be "system"; empty ones are the defaults.
	known := func(id string) bool { _, ok := ThemeByID(id); return ok || id == "" }
	if a := s.Appearance; !known(a.Theme) && a.Theme != "system" {
		return apperr.New(apperr.Invalid, "unknown theme "+a.Theme)
	}
	for _, id := range []string{s.Appearance.DayTheme, s.Appearance.NightTheme} {
		if !known(id) {
			return apperr.New(apperr.Invalid, "unknown theme "+id)
		}
	}
	if s.Network.ConnectTimeout != "" {
		if _, err := time.ParseDuration(s.Network.ConnectTimeout); err != nil {
			if _, err := strconv.Atoi(s.Network.ConnectTimeout); err != nil {
				return apperr.New(apperr.Invalid, "connect timeout: a duration like 10s")
			}
		}
	}
	if s.Network.Retry < -1 {
		return apperr.New(apperr.Invalid, "retries: -1 (forever) or more")
	}
	switch s.History.Retention {
	case "", "7d", "30d", "forever":
	default:
		return apperr.New(apperr.Invalid, "history retention: 7d, 30d or forever")
	}
	return nil
}

func normalize(s Settings) Settings {
	d := Defaults()
	// Unknown themes (a later version's) are the defaults.
	if _, ok := ThemeByID(s.Appearance.Theme); !ok && s.Appearance.Theme != "system" {
		s.Appearance.Theme = d.Appearance.Theme
	}
	if _, ok := ThemeByID(s.Appearance.DayTheme); !ok {
		s.Appearance.DayTheme = d.Appearance.DayTheme
	}
	if _, ok := ThemeByID(s.Appearance.NightTheme); !ok {
		s.Appearance.NightTheme = d.Appearance.NightTheme
	}
	if s.Appearance.UIFontSize <= 0 {
		s.Appearance.UIFontSize = d.Appearance.UIFontSize
	}
	if s.Appearance.CodeFontSize <= 0 {
		s.Appearance.CodeFontSize = d.Appearance.CodeFontSize
	}
	s.Appearance.SideWidth = paneWidth(s.Appearance.SideWidth, d.Appearance.SideWidth, 180, 480)
	s.Appearance.ResultsWidth = paneWidth(s.Appearance.ResultsWidth, d.Appearance.ResultsWidth, 320, 900)
	s.Appearance.ResultsTop = paneWidth(s.Appearance.ResultsTop, 0, 80, 1200)
	if s.History.Retention == "" {
		s.History.Retention = d.History.Retention
	}
	if s.Shortcuts == nil {
		s.Shortcuts = map[string]string{}
	}
	if s.Updates.Channel != ChannelPrerelease {
		s.Updates.Channel = ChannelStable
	}
	s.Version = Version
	return s
}

// paneWidth is w within [lo, hi], or def when unset.
func paneWidth(w, def, lo, hi int) int {
	if w <= 0 {
		return def
	}
	return min(max(w, lo), hi)
}

// forPage masks the proxy URL's password: the page never gets it back.
func forPage(s Settings) Settings {
	c := clone(s)
	c.Network.Proxy = maskProxy(c.Network.Proxy)
	// The TLS files by name: their folders stay in Go, as every picked
	// file's do.
	for _, f := range []*string{&c.TLS.CACert, &c.TLS.Cert, &c.TLS.Key} {
		if *f != "" {
			*f = filepath.Base(*f)
		}
	}
	return c
}

func maskProxy(proxy string) string {
	u, err := url.Parse(proxy)
	if err != nil || u.User == nil {
		return proxy
	}
	if _, ok := u.User.Password(); !ok {
		return proxy
	}
	return u.Redacted()
}

func clone(s Settings) Settings {
	c := s
	c.Shortcuts = make(map[string]string, len(s.Shortcuts))
	for k, v := range s.Shortcuts {
		c.Shortcuts[k] = v
	}
	return c
}

// Service is the settings bindings.
type Service struct{ st *Store }

// NewService returns the bindings over st.
func NewService(st *Store) *Service { return &Service{st: st} }

// Get returns the settings, the proxy password masked.
func (s *Service) Get() Settings { return forPage(s.st.Get()) }

// Set replaces the settings (TLS files excepted) and returns them. A proxy
// URL sent back with its password masked keeps the stored password.
func (s *Service) Set(v Settings) (Settings, error) {
	if stored := s.st.Get().Network.Proxy; v.Network.Proxy != "" && v.Network.Proxy == maskProxy(stored) {
		v.Network.Proxy = stored
	}
	out, err := s.st.Set(v)
	return forPage(out), err
}

// SetTLSFile sets the cacert, cert or key file from a picked file's
// handle ("" clears it).
func (s *Service) SetTLSFile(kind, handle string) (Settings, error) {
	out, err := s.st.SetTLSFile(kind, handle)
	return forPage(out), err
}
