// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// uninstallKey is the installer's HKLM Uninstall key (wails_tools.nsh:
// "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}").
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\The Sonde AuthorsSonde`

// winInstaller runs the NSIS installer of the update in its silent update
// mode, for a machine-wide install.
type winInstaller struct {
	deps   InstallerDeps
	reason string
}

// NewInstaller returns the Windows installer for the running app.
func NewInstaller(deps InstallerDeps) Installer {
	i := &winInstaller{deps: deps}
	i.reason = check()
	return i
}

// check is why the running exe is not the machine-wide install, or "".
func check() string {
	exe, err := os.Executable()
	if err != nil {
		return reasonNotInstalled
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	// The machine-wide key only, in the 64-bit view: the user's own keys
	// are never trusted for an installer that runs elevated.
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallKey, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return reasonNotInstalled
	}
	defer k.Close()
	get := func(name string) string {
		v, _, err := k.GetStringValue(name)
		if err != nil {
			return ""
		}
		return v
	}
	if installedHere(get, filepath.Dir(exe)) {
		return ""
	}
	return reasonNotInstalled
}

func (i *winInstaller) CanInstall() (bool, string) { return i.reason == "", i.reason }

// checkNow re-reads the registry in Install; tests turn it off.
var checkNow = true

// shellExecute starts a program the way Explorer does: an installer that
// asks for elevation gets the UAC prompt. A variable for tests.
var shellExecute = func(file, args string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	a, err := windows.UTF16PtrFromString(args)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, f, a, nil, windows.SW_SHOWNORMAL)
}

// Install opens the staged installer so no one else can change it, checks
// it again through that handle, and starts it with /S /UPDATE /WAITPID
// while the handle stays open. The installer waits for Sonde to quit,
// updates the installed folder and starts Sonde again.
func (i *winInstaller) Install(r Record) error {
	// Checked again, unless a test set it up (checkNow false).
	if checkNow {
		i.reason = check()
	}
	if i.reason != "" {
		return &Error{Kind: KindInstall, Err: errors.New(i.reason)}
	}
	p, err := windows.UTF16PtrFromString(r.Staged)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), r.Staged)
	defer f.Close()
	if err := rehash(f, r); err != nil {
		return err
	}
	err = shellExecute(r.Staged, fmt.Sprintf("/S /UPDATE /WAITPID=%d", os.Getpid()))
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return &Error{Kind: KindInstall, Err: errors.New("the installation was canceled")}
	}
	if err != nil {
		return &Error{Kind: KindInstall, Err: err}
	}
	i.deps.Quit()
	return nil
}
