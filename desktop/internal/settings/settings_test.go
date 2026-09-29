// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/sandbox"
)

func store(t *testing.T) (*Store, string, *emit.Recorder, *handles.Table) {
	t.Helper()
	dir := t.TempDir()
	root, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
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
	s.TLS.CACert = "/etc/evil" // ignored: TLS files come from dialogs
	if _, err := st.Set(s); err != nil {
		t.Fatal(err)
	}
	if len(rec.Events()) != 1 || rec.Events()[0].Topic != TopicChanged {
		t.Errorf("events %+v", rec.Events())
	}
	root, _ := sandbox.Open(dir)
	again := Open(root, &emit.Recorder{}, handles.New()).Get()
	if again.Appearance.Theme != "dark" || again.Network.Retry != 2 || !again.Cookies.Keep || again.TLS.CACert != "" {
		t.Errorf("reloaded %+v", again)
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
	root, _ := sandbox.Open(dir)
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
	if _, err := st.SetTLSFile(CACert, ""); err != nil || st.Get().TLS.CACert != "" {
		t.Errorf("clear: %v", err)
	}
}
