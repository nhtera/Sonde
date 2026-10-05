// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVerifyBundleRefusesUnsigned(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Sonde.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil { //nolint:gosec // an app fixture
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", "sonde-desktop"), []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // an app fixture
		t.Fatal(err)
	}
	if err := verifyBundle(app); kindOf(err) != KindVerification {
		t.Fatalf("an unsigned app: %v, want a verification error", err)
	}
}

// TestVerifyBundleReleased checks the team requirement against a released
// Sonde in Applications (skipped where none is installed), and that one
// edited file fails it.
func TestVerifyBundleReleased(t *testing.T) {
	const released = "/Applications/Sonde.app"
	if _, err := os.Stat(released); err != nil {
		t.Skip("no released Sonde in /Applications")
	}
	if err := verifyBundle(released); err != nil {
		t.Fatalf("the released app: %v", err)
	}
	app := filepath.Join(t.TempDir(), "Sonde.app")
	if out, err := exec.Command("ditto", released, app).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v %s", err, out)
	}
	plist := filepath.Join(app, "Contents", "Info.plist")
	b, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plist, append(b, '\n'), 0o644); err != nil { //nolint:gosec // the copied app
		t.Fatal(err)
	}
	if err := verifyBundle(app); kindOf(err) != KindVerification {
		t.Fatalf("an edited app: %v, want a verification error", err)
	}
}
