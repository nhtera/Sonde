// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package update

import (
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

func TestWindowsInstall(t *testing.T) {
	body := []byte("setup.exe bytes")
	p := filepath.Join(t.TempDir(), "Sonde-Desktop-0.2.1-windows-amd64-setup.exe")
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(body)
	r := Record{Version: "0.2.1", Staged: p, Artifact: manifest.Artifact{Filename: filepath.Base(p), Size: int64(len(body)), SHA512: hex.EncodeToString(sum[:])}}

	old, oldCheck := shellExecute, checkNow
	t.Cleanup(func() { shellExecute, checkNow = old, oldCheck })
	checkNow = false
	var file, args string
	var result error
	shellExecute = func(f, a string) error {
		file, args = f, a
		// The staged file is locked against writes while it starts.
		if err := os.WriteFile(p, []byte("swapped"), 0o600); err == nil {
			t.Error("the staged installer could be changed while it started")
		}
		return result
	}
	quits := 0
	i := &winInstaller{deps: InstallerDeps{Quit: func() { quits++ }}}

	result = windows.ERROR_CANCELLED
	if err := i.Install(r); kindOf(err) != KindInstall || quits != 0 {
		t.Fatalf("UAC declined: %v, %d quits; want an install error and no quit", err, quits)
	}
	result = nil
	if err := i.Install(r); err != nil || quits != 1 {
		t.Fatalf("Install = %v, %d quits", err, quits)
	}
	if file != p || args != "/S /UPDATE /WAITPID="+strconv.Itoa(os.Getpid()) {
		t.Fatalf("started %s %s", file, args)
	}
	if err := os.WriteFile(p, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := i.Install(r); kindOf(err) != KindVerification || quits != 1 || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("a changed installer: %v", err)
	}
}
