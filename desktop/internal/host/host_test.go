// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
	"github.com/nhtera/sonde/desktop/internal/emit"
)

// windowOnly are the services only the window app registers: they open
// folders, reveal and trash files, export reports and reveal secrets, and
// measure the shipped app.
var windowOnly = []string{"workspaceDesktop", "bodiesDesktop", "copyasReveal", "dialogs", "reports", "perf", "update"}

func TestServicesPerMode(t *testing.T) {
	names := func(m Mode) []string {
		var out []string
		for _, r := range registry {
			if r.modes == nil || slices.Contains(r.modes, m) {
				out = append(out, r.name)
			}
		}
		return out
	}
	for _, m := range []Mode{ModeServer, ModeHarness} {
		for _, name := range windowOnly {
			// The harness runs the real update service on scripted parts
			// (harness_update.go, e2eharness builds only).
			if m == ModeHarness && name == "update" {
				continue
			}
			if slices.Contains(names(m), name) {
				t.Errorf("mode %d registers the window-only service %s", m, name)
			}
		}
	}
	for _, name := range windowOnly {
		if !slices.Contains(names(ModeDesktop), name) {
			t.Errorf("the window app lacks %s", name)
		}
	}
}

// TestBindingsPerMode lists every bound method of each mode: server mode
// and the harness have none of the window app's file, clipboard and
// dialog methods, and no mode binds a Go-side core (raw bodies, opening a
// folder by path, the stores).
func TestBindingsPerMode(t *testing.T) {
	windowMethods := []string{
		"WorkspaceDesktop.OpenFolder", "WorkspaceDesktop.OpenFolderToImport", "WorkspaceDesktop.OpenRecent", "WorkspaceDesktop.Reveal", "WorkspaceDesktop.Trash",
		"WorkspaceDesktop.CopyIntoProject", "BodiesDesktop.SaveResponse", "BodiesDesktop.OpenExternally",
		"CopyasReveal.Curl", "CopyasReveal.Sonde", "Dialogs.OpenFile",
		"Update.State", "Update.Check", "Update.Install", "Update.Skip", "Update.ClearSkip",
		"Update.Restart", "Update.OpenReleasePage", "Update.Info",
	}
	for _, m := range []Mode{ModeDesktop, ModeServer, ModeHarness} {
		h := &Host{Mode: m, Dirs: testDirs(t), Emit: &emit.Recorder{}}
		if err := h.Setup(); err != nil {
			t.Fatal(err)
		}
		var bound []string
		for _, s := range Services(h) {
			v := reflect.ValueOf(s.Instance())
			typ := v.Type().Elem()
			if strings.HasSuffix(typ.PkgPath(), "internal/bodies") && typ.Name() == "Store" ||
				typ.Name() == "Workspace" || typ.Name() == "Runs" || typ.Name() == "Envs" ||
				strings.HasSuffix(typ.PkgPath(), "internal/settings") && typ.Name() == "Store" {
				t.Errorf("mode %d binds the core %s.%s", m, typ.PkgPath(), typ.Name())
			}
			name := serviceLabel(s.Instance())
			for i := range v.NumMethod() {
				bound = append(bound, name+"."+v.Type().Method(i).Name)
			}
		}
		for _, wm := range windowMethods {
			has := slices.Contains(bound, wm)
			if m == ModeDesktop && !has {
				t.Errorf("the window app lacks %s", wm)
			}
			harnessUpdate := m == ModeHarness && strings.HasPrefix(wm, "Update.")
			if m != ModeDesktop && has && !harnessUpdate {
				t.Errorf("mode %d binds the window-only %s", m, wm)
			}
		}
		for _, b := range bound {
			if strings.HasSuffix(b, ".Raw") || strings.HasSuffix(b, ".Put") || strings.HasSuffix(b, ".Root") {
				t.Errorf("mode %d binds %s", m, b)
			}
			// The update core and the guard's exit switch are Go's only.
			if strings.HasPrefix(b, "Manager.") || strings.HasSuffix(b, ".Attach") || strings.HasSuffix(b, "Exit") {
				t.Errorf("mode %d binds %s", m, b)
			}
		}
		if m == ModeDesktop {
			var guard []string
			for _, b := range bound {
				if strings.HasPrefix(b, "Guard.") {
					guard = append(guard, b)
				}
			}
			if want := []string{"Guard.Hold", "Guard.Leave", "Guard.SetUnsaved"}; !slices.Equal(guard, want) {
				t.Errorf("close guard bindings %v, want %v", guard, want)
			}
		}
	}
}

// serviceLabel names a service for TestBindingsPerMode: the window-only
// ones by their registration name, the others by type.
func serviceLabel(inst any) string {
	typ := reflect.TypeOf(inst).Elem()
	switch typ.PkgPath() + "." + typ.Name() {
	case "github.com/nhtera/sonde/desktop/internal/workspace.Desktop":
		return "WorkspaceDesktop"
	case "github.com/nhtera/sonde/desktop/internal/bodies.Desktop":
		return "BodiesDesktop"
	case "github.com/nhtera/sonde/desktop/internal/copyas.Reveal":
		return "CopyasReveal"
	case "github.com/nhtera/sonde/desktop/internal/update.Service":
		return "Update"
	}
	return typ.Name()
}

func testDirs(t *testing.T) *appdirs.Dirs {
	t.Helper()
	d, err := appdirs.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
