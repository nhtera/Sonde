// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/internal/runplan"
)

func store(t *testing.T) (*Store, string, *emit.Recorder, *handles.Table) {
	t.Helper()
	dir := t.TempDir()
	root := sandboxtest.Open(t, dir)
	rec, h := &emit.Recorder{}, handles.New()
	return Open(root, rec, h), dir, rec, h
}

func TestDefaultsAndRoundTrip(t *testing.T) {
	st, dir, rec, _ := store(t)
	if got := st.Get(); got.Appearance.Theme != "system" || !got.History.Enabled || got.Version != Version {
		t.Fatalf("defaults %+v", got)
	}
	s := st.Get()
	s.Appearance.Theme = "dark"
	s.Network = Network{Proxy: "http://p:3128", ConnectTimeout: "5s", Retry: 2}
	s.Cookies.Keep = true
	s.Appearance.SideWidth, s.Appearance.ResultsWidth = 300, 5000 // results clamped
	s.Appearance.ResultsTop = 20                                  // the request list's height, clamped
	s.TLS.CACert = "/etc/evil"                                    // ignored: TLS files come from dialogs
	if _, err := st.Set(s); err != nil {
		t.Fatal(err)
	}
	if len(rec.Events()) != 1 || rec.Events()[0].Topic != TopicChanged {
		t.Errorf("events %+v", rec.Events())
	}
	root := sandboxtest.Open(t, dir)
	again := Open(root, &emit.Recorder{}, handles.New()).Get()
	if again.Appearance.Theme != "dark" || again.Network.Retry != 2 || !again.Cookies.Keep || again.TLS.CACert != "" {
		t.Errorf("reloaded %+v", again)
	}
	if a := again.Appearance; a.SideWidth != 300 || a.ResultsWidth != 900 || a.ResultsTop != 80 {
		t.Errorf("pane sizes %d %d %d, want 300, 900 and 80", a.SideWidth, a.ResultsWidth, a.ResultsTop)
	}
	if fi, err := os.Stat(filepath.Join(dir, File)); err != nil || runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("settings file %v %v", fi, err)
	}
}

func TestInvalidAndFuture(t *testing.T) {
	st, dir, _, _ := store(t)
	for _, bad := range []func(*Settings){
		func(s *Settings) { s.Appearance.Theme = "neon" },
		func(s *Settings) { s.Network.ConnectTimeout = "soon" },
		func(s *Settings) { s.Network.Retry = -2 },
		func(s *Settings) { s.History.Retention = "1y" },
	} {
		s := st.Get()
		bad(&s)
		if _, err := st.Set(s); err == nil {
			t.Errorf("accepted %+v", s)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, File), []byte(`{"version": 99, "appearance": {"theme": "dark"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	root := sandboxtest.Open(t, dir)
	if got := Open(root, &emit.Recorder{}, handles.New()).Get(); got.Appearance.Theme != "system" {
		t.Errorf("a newer file's settings were read: %+v", got)
	}
}

func TestTLSAndApply(t *testing.T) {
	st, _, _, h := store(t)
	id, _ := h.Put("/certs/ca.pem", handles.OpenFile)
	if _, err := st.SetTLSFile(CACert, id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetTLSFile(CACert, id); err == nil {
		t.Error("a used handle set a file again")
	}
	if _, err := st.SetTLSFile("nope", ""); err == nil {
		t.Error("unknown kind")
	}
	s := st.Get()
	s.Network = Network{Proxy: "http://p:3128", ConnectTimeout: "5s", Retry: 3}
	if _, err := st.Set(s); err != nil {
		t.Fatal(err)
	}
	var inv runplan.Invocation
	st.Apply(&inv)
	if inv.Proxy != "http://p:3128" || inv.ConnectTimeout != "5s" || inv.Retry != "3" || inv.CACert != "/certs/ca.pem" ||
		!inv.Set["proxy"] || !inv.Set["connect-timeout"] || !inv.Set["retry"] || !inv.Set["cacert"] || inv.Set["cert"] {
		t.Errorf("invocation %+v", inv)
	}
	if inv.Set["insecure"] {
		t.Error("verification is on by default")
	}
	// Verification off is --insecure; Set never changes the files.
	s = st.Get()
	s.TLS = TLS{CACert: "/elsewhere.pem", SkipVerify: true}
	if _, err := st.Set(s); err != nil {
		t.Fatal(err)
	}
	inv = runplan.Invocation{}
	st.Apply(&inv)
	if !inv.Insecure || !inv.Set["insecure"] || inv.CACert != "/certs/ca.pem" {
		t.Errorf("skip verify %+v", inv)
	}
	if _, err := st.SetTLSFile(CACert, ""); err != nil || st.Get().TLS.CACert != "" || !st.Get().TLS.SkipVerify {
		t.Errorf("clear: %v", err)
	}
}

func TestProxyPasswordNeverSent(t *testing.T) {
	st, _, rec, _ := store(t)
	svc := NewService(st)
	s := svc.Get()
	s.Network.Proxy = "http://u:proxy-pw-sentinel@p.example:3128"
	got, err := svc.Set(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{got, svc.Get(), rec.Events()} {
		if b, _ := json.Marshal(v); strings.Contains(string(b), "proxy-pw-sentinel") {
			t.Errorf("proxy password sent: %s", b)
		}
	}
	// The page sends the masked form back: the password stays.
	again := svc.Get()
	again.Appearance.Theme = "dark"
	if _, err := svc.Set(again); err != nil {
		t.Fatal(err)
	}
	if st.Get().Network.Proxy != "http://u:proxy-pw-sentinel@p.example:3128" { //nolint:gosec // G101: test sentinel
		t.Errorf("stored proxy %q", st.Get().Network.Proxy)
	}
}

func TestThemes(t *testing.T) {
	st, dir, _, _ := store(t)
	load := func(file string) Appearance {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, File), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
		root := sandboxtest.Open(t, dir)
		return Open(root, &emit.Recorder{}, handles.New()).Get().Appearance
	}
	// A file from before the Day and Night themes.
	if a := load(`{"version": 1, "appearance": {"theme": "dark"}}`); a.Theme != "dark" || a.DayTheme != "light" || a.NightTheme != "dark" {
		t.Errorf("old file: %+v", a)
	}
	// A later version's themes are the defaults, and the next change saves.
	a := load(`{"version": 1, "appearance": {"theme": "future-theme", "dayTheme": "future-day", "nightTheme": "dracula"}}`)
	if a.Theme != "system" || a.DayTheme != "light" || a.NightTheme != "dracula" {
		t.Errorf("unknown themes: %+v", a)
	}
	root := sandboxtest.Open(t, dir)
	later := Open(root, &emit.Recorder{}, handles.New())
	if _, err := later.Set(later.Get()); err != nil {
		t.Errorf("set after unknown themes: %v", err)
	}

	s := st.Get()
	s.Appearance.Theme, s.Appearance.DayTheme, s.Appearance.NightTheme = "monokai", "solarized-light", "dracula"
	if _, err := st.Set(s); err != nil {
		t.Fatal(err)
	}
	root = sandboxtest.Open(t, dir)
	if a := Open(root, &emit.Recorder{}, handles.New()).Get().Appearance; a.Theme != "monokai" || a.DayTheme != "solarized-light" || a.NightTheme != "dracula" {
		t.Errorf("round trip: %+v", a)
	}
	for _, bad := range []func(*Appearance){
		func(a *Appearance) { a.DayTheme = "neon" },
		func(a *Appearance) { a.NightTheme = "system" },
	} {
		s := st.Get()
		bad(&s.Appearance)
		if _, err := st.Set(s); err == nil {
			t.Errorf("accepted %+v", s.Appearance)
		}
	}
}

// Concurrent changes reach the page and Changed in the order they were
// saved: the last of each is the stored settings.
func TestSetOrder(t *testing.T) {
	st, _, rec, _ := store(t)
	var mu sync.Mutex
	var last Settings
	st.Changed = func(s Settings) {
		mu.Lock()
		last = s
		mu.Unlock()
	}
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := st.Get()
			s.Appearance.NightTheme = Themes[i%len(Themes)].ID
			s.Network.Retry = i
			if _, err := st.Set(s); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	want := st.Get()
	events := rec.Events()
	if got := events[len(events)-1].Data.(Settings); got.Network.Retry != want.Network.Retry || got.Appearance.NightTheme != want.Appearance.NightTheme {
		t.Errorf("last event %+v, stored %+v", got.Appearance, want.Appearance)
	}
	if last.Network.Retry != want.Network.Retry {
		t.Errorf("last Changed retry %d, stored %d", last.Network.Retry, want.Network.Retry)
	}
}

// A change that is not saved is not kept either.
func TestFailedSave(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a read-only folder")
	}
	st, dir, rec, _ := store(t)
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // a folder, read-only
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // a folder, writable again
	s := st.Get()
	s.Appearance.Theme = "dracula"
	if _, err := st.Set(s); err == nil {
		t.Fatal("saved in a read-only folder")
	}
	if got := st.Get().Appearance.Theme; got != "system" || len(rec.Events()) != 0 {
		t.Errorf("theme %q, events %d after a failed save", got, len(rec.Events()))
	}
}

// TestTLSFilesByName: the page sees a picked TLS file's name, never its
// folder, and a settings save it sends back keeps the stored path.
func TestTLSFilesByName(t *testing.T) {
	st, _, _, h := store(t)
	svc := NewService(st)
	id, _ := h.Put("/Users/me/certs/ca.pem", handles.OpenFile)
	got, err := svc.SetTLSFile(CACert, id)
	if err != nil || got.TLS.CACert != "ca.pem" || svc.Get().TLS.CACert != "ca.pem" {
		t.Fatalf("page sees %q (%v)", got.TLS.CACert, err)
	}
	if _, err := svc.Set(svc.Get()); err != nil {
		t.Fatal(err)
	}
	if st.Get().TLS.CACert != "/Users/me/certs/ca.pem" {
		t.Errorf("a save from the page changed the path: %q", st.Get().TLS.CACert)
	}
	if got, err := svc.SetTLSFile(CACert, ""); err != nil || got.TLS.CACert != "" {
		t.Errorf("clear: %q %v", got.TLS.CACert, err)
	}
}

func TestUpdates(t *testing.T) {
	writeFile := func(t *testing.T, dir string, v any) {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, File), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	open := func(t *testing.T, dir string) Settings {
		return Open(sandboxtest.Open(t, dir), &emit.Recorder{}, handles.New()).Get()
	}

	t.Run("a 0.1.0 file keeps every value and checks for updates", func(t *testing.T) {
		old := Defaults()
		old.Appearance.Theme, old.Network.Retry, old.History.Enabled, old.Contract.Check = "dark", 3, false, true
		old.Shortcuts = map[string]string{"file.run": "Alt+KeyR"}
		data, err := json.Marshal(old)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		delete(m, "updates") // 0.1.0 had no such group
		dir := t.TempDir()
		writeFile(t, dir, m)
		got := open(t, dir)
		want := old
		want.Updates = Updates{Check: true, Channel: ChannelStable}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("loaded\n%+v\nwant\n%+v", got, want)
		}
	})
	t.Run("a file without a version is not read", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, map[string]any{"appearance": map[string]any{"theme": "dark"}})
		if got := open(t, dir); got.Appearance.Theme != "system" {
			t.Fatalf("a versionless file was read: %+v", got.Appearance)
		}
	})
	t.Run("the switch off stays off", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, map[string]any{"version": 1, "updates": map[string]any{"check": false}})
		if got := open(t, dir); got.Updates.Check || got.Updates.Channel != ChannelStable {
			t.Fatalf("updates %+v, want off on the stable channel", got.Updates)
		}
	})
	t.Run("a page's Set changes the switch only", func(t *testing.T) {
		st, dir, rec, _ := store(t)
		stale := st.Get() // the page's copy, before the service writes
		if err := st.SetUpdateState(func(u *Updates) {
			u.Skipped, u.Channel, u.LastCheck = "0.2.1", ChannelPrerelease, "2026-10-05T01:00:00Z"
		}); err != nil {
			t.Fatal(err)
		}
		if n := len(rec.Events()); n != 1 {
			t.Errorf("%d events after SetUpdateState, want 1", n)
		}
		stale.Updates = Updates{Check: false, Skipped: "", Channel: ChannelStable, LastCheck: ""}
		if _, err := st.Set(stale); err != nil {
			t.Fatal(err)
		}
		want := Updates{Check: false, Skipped: "0.2.1", Channel: ChannelPrerelease, LastCheck: "2026-10-05T01:00:00Z"}
		if got := st.Get().Updates; got != want {
			t.Fatalf("updates %+v, want %+v", got, want)
		}
		if got := open(t, dir).Updates; got != want {
			t.Fatalf("saved updates %+v, want %+v", got, want)
		}
	})
	t.Run("a refused file is replaced by a user's change only", func(t *testing.T) {
		dir := t.TempDir()
		newer := []byte(`{"version": 99, "network": {"proxy": "http://p:3128"}}`)
		if err := os.WriteFile(filepath.Join(dir, File), newer, 0o600); err != nil {
			t.Fatal(err)
		}
		st := Open(sandboxtest.Open(t, dir), &emit.Recorder{}, handles.New())
		if err := st.SetUpdateState(func(u *Updates) { u.LastCheck = "2026-10-05T01:00:00Z" }); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join(dir, File)); string(got) != string(newer) {
			t.Fatalf("an unattended update check replaced a refused file: %s", got)
		}
		if st.Get().Updates.LastCheck == "" {
			t.Fatal("the update state was not kept in memory")
		}
		if _, err := st.Set(st.Get()); err != nil {
			t.Fatal(err)
		}
		if got := open(t, dir); got.Updates.LastCheck != "2026-10-05T01:00:00Z" {
			t.Fatalf("after the user's change: %+v", got.Updates)
		}
	})
	t.Run("an unknown channel is stable", func(t *testing.T) {
		st, _, _, _ := store(t)
		if err := st.SetUpdateState(func(u *Updates) { u.Channel = "nightly" }); err != nil {
			t.Fatal(err)
		}
		if got := st.Get().Updates.Channel; got != ChannelStable {
			t.Fatalf("channel %q", got)
		}
	})
}
