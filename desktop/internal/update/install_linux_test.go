// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package update

import (
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

func staged(t *testing.T, body []byte) Record {
	t.Helper()
	p := filepath.Join(t.TempDir(), "Sonde-Desktop-0.2.1-linux-x86_64.AppImage")
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(body)
	return Record{Version: "0.2.1", Staged: p, Artifact: manifest.Artifact{Filename: filepath.Base(p), Size: int64(len(body)), SHA512: hex.EncodeToString(sum[:])}}
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "Sonde.AppImage")
	if err := os.WriteFile(cur, []byte("old image"), 0o755); err != nil { //nolint:gosec // an app fixture
		t.Fatal(err)
	}
	// The running app's mount holds the old file open.
	old, err := os.Open(cur)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err := replaceFile(dir, "Sonde.AppImage", staged(t, []byte("new image"))); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cur)
	fi, _ := os.Stat(cur)
	if err != nil || string(b) != "new image" || fi.Mode().Perm() != 0o755 {
		t.Fatalf("replaced: %q %v %v", b, fi.Mode(), err)
	}
	if b, _ := io.ReadAll(old); string(b) != "old image" {
		t.Fatalf("the open old file reads %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestReplaceFileRefusesAChangedStage(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "Sonde.AppImage")
	if err := os.WriteFile(cur, []byte("old image"), 0o755); err != nil { //nolint:gosec // an app fixture
		t.Fatal(err)
	}
	r := staged(t, []byte("new image"))
	if err := os.WriteFile(r.Staged, []byte("evil image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(dir, "Sonde.AppImage", r); kindOf(err) != KindVerification {
		t.Fatalf("replaceFile = %v, want a verification error", err)
	}
	if b, _ := os.ReadFile(cur); string(b) != "old image" {
		t.Fatalf("the old image was replaced: %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("the temporary file stays: %v", entries)
	}
}

func TestRelaunchAfter(t *testing.T) {
	dir := t.TempDir()
	mark := filepath.Join(dir, "started")
	app := filepath.Join(dir, "app with space")
	if err := os.WriteFile(app, []byte("#!/bin/sh\ntouch \"$(dirname \"$0\")/started\"\n"), 0o755); err != nil { //nolint:gosec // an app fixture
		t.Fatal(err)
	}
	sleeper := exec.Command("sleep", "30")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	if err := relaunchAfter(sleeper.Process.Pid, app); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("started before the app quit")
	}
	_ = sleeper.Process.Kill()
	_ = sleeper.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(mark); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("not started after the app quit")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestCheckAppImageRefusesWithoutTheRuntime(t *testing.T) {
	if got := checkAppImage("", ""); got != reasonNotAppImage {
		t.Errorf("no APPIMAGE: %q", got)
	}
	// A forged environment: this test binary does not run from APPDIR.
	if got := checkAppImage("/tmp/x.AppImage", t.TempDir()); got != reasonNotAppImage {
		t.Errorf("a forged APPDIR: %q", got)
	}
}
