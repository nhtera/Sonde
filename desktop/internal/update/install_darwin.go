// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package update

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// teamRequirement is the code signature a staged update must carry: the
// app's Developer ID team, anchored at Apple.
const teamRequirement = `anchor apple generic and certificate leaf[subject.OU] = "QCLXRD7V9M"`

// macInstaller swaps the bundle through the Wails helper.
type macInstaller struct {
	deps   InstallerDeps
	bundle string // the running Sonde.app
	reason string // why not, "" when it can
}

// NewInstaller returns the macOS installer for the running app.
func NewInstaller(deps InstallerDeps) Installer {
	i := &macInstaller{deps: deps}
	exe, err := os.Executable()
	if err == nil {
		i.bundle = bundleOf(exe)
	}
	i.reason = i.check()
	return i
}

// check is why this copy cannot replace itself, or "".
func (i *macInstaller) check() string {
	switch {
	case i.bundle == "":
		return reasonUnsupported
	case translocated(i.bundle):
		return reasonTranslocated
	}
	// The helper renames the staged app (in the temp folder) over the
	// bundle: one volume.
	var a, b unix.Stat_t
	if unix.Stat(i.bundle, &a) != nil || unix.Stat(os.TempDir(), &b) != nil || a.Dev != b.Dev {
		return reasonVolume
	}
	uid := uint32(os.Getuid()) //nolint:gosec // a uid
	bad, err := ownedTree(os.DirFS(i.bundle), func(fi fs.FileInfo) bool {
		st, ok := fi.Sys().(*syscall.Stat_t)
		return ok && st.Uid == uid && fi.Mode().Perm()&0o200 != 0
	})
	if err != nil || bad != "" {
		return reasonNotOwned
	}
	// The swap moves the bundle aside in its folder (Applications).
	if unix.Access(filepath.Dir(i.bundle), unix.W_OK) != nil {
		return reasonNotOwned
	}
	return ""
}

func (i *macInstaller) CanInstall() (bool, string) { return i.reason == "", i.reason }

// Install checks the staged app's signature (Apple-anchored, Sonde's team),
// then hands it to the Wails helper, which swaps it in after Sonde quits
// and opens it.
func (i *macInstaller) Install(r Record) error {
	// Checked again: the bundle may have moved or changed hands since
	// launch.
	if i.reason = i.check(); i.reason != "" {
		return &Error{Kind: KindInstall, Err: errors.New(i.reason)}
	}
	if err := verifyBundle(r.Staged); err != nil {
		return err
	}
	return i.deps.Restart(context.Background())
}

// verifyBundle runs codesign on the staged app, with fixed arguments and
// the path as one of them.
func verifyBundle(app string) error {
	out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", "-R="+teamRequirement, "--", app).CombinedOutput() //nolint:gosec // fixed arguments; app is one argument
	if err != nil {
		return &Error{Kind: KindVerification, Err: fmt.Errorf("the downloaded app's signature did not verify: %s", strings.TrimSpace(string(out)))}
	}
	return nil
}
