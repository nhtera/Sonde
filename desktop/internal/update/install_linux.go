// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package update

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// appImageInstaller replaces the running AppImage file.
type appImageInstaller struct {
	deps     InstallerDeps
	appImage string // $APPIMAGE, proven to be the running image
	appDir   string
	reason   string
}

// NewInstaller returns the Linux installer for the running app.
func NewInstaller(deps InstallerDeps) Installer {
	i := &appImageInstaller{deps: deps, appImage: os.Getenv("APPIMAGE"), appDir: os.Getenv("APPDIR")}
	i.reason = checkAppImage(i.appImage, i.appDir)
	return i
}

func (i *appImageInstaller) CanInstall() (bool, string) { return i.reason == "", i.reason }

// checkAppImage is why $APPIMAGE cannot be replaced in place, or "": the
// app must run from APPDIR, a FUSE mount of that very file, which the user
// owns in a folder only they can write. An inherited or forged APPIMAGE
// fails one of these.
func checkAppImage(appImage, appDir string) string {
	if appImage == "" || appDir == "" || !filepath.IsAbs(appImage) {
		return reasonNotAppImage
	}
	exe, err := os.Executable()
	if err != nil || !insideDir(exe, appDir) {
		return reasonNotAppImage
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return reasonNotAppImage
	}
	mounts, err := parseMountinfo(f)
	_ = f.Close()
	if err != nil {
		return reasonNotAppImage
	}
	fuse, named := appImageMount(mounts, appDir, appImage)
	if !fuse || !named && !openedByRuntime(appImage) {
		return reasonNotAppImage
	}
	uid := uint32(os.Getuid()) //nolint:gosec // a uid
	var st unix.Stat_t
	if unix.Lstat(appImage, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uid {
		return reasonShared
	}
	dir := filepath.Dir(appImage)
	fi, err := os.Lstat(dir) //nolint:gosec // G703: the AppImage's own folder, checked here
	ds, ok := sysStat(fi)
	if err != nil || !ok || ds.Uid != uid || !privateDir(fi.Mode()) {
		return reasonShared
	}
	return ""
}

func sysStat(fi os.FileInfo) (*syscall.Stat_t, bool) {
	if fi == nil {
		return nil, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return st, ok
}

// openedByRuntime is the fallback where mountinfo does not name the image
// (best effort): the AppImage runtime forks its FUSE server before it
// starts the app, so that server, a child or a sibling of this process,
// holds the very file $APPIMAGE names open.
func openedByRuntime(appImage string) bool {
	var want unix.Stat_t
	if unix.Stat(appImage, &want) != nil {
		return false
	}
	for _, pid := range runtimeCandidates() {
		fds, err := os.ReadDir("/proc/" + pid + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			var st unix.Stat_t
			if unix.Stat("/proc/"+pid+"/fd/"+fd.Name(), &st) == nil && st.Dev == want.Dev && st.Ino == want.Ino {
				return true
			}
		}
	}
	return false
}

// runtimeCandidates are this process and the processes whose parent is
// this process or its parent.
func runtimeCandidates() []string {
	self, parent := os.Getpid(), os.Getppid()
	out := []string{"self"}
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil || pid == self {
			continue
		}
		stat, err := os.ReadFile("/proc/" + p.Name() + "/stat")
		if err != nil {
			continue
		}
		// pid (comm) state ppid ...: comm may hold spaces and parentheses.
		rest := string(stat)
		if i := strings.LastIndexByte(rest, ')'); i >= 0 {
			rest = rest[i+1:]
		}
		f := strings.Fields(rest)
		if len(f) < 2 {
			continue
		}
		if ppid, err := strconv.Atoi(f[1]); err == nil && (ppid == self || ppid == parent) {
			out = append(out, p.Name())
		}
	}
	return out
}

// Install writes the staged AppImage next to the running one, checks it
// again through the descriptor it wrote, renames it over the running one
// (this process keeps its mount of the old file), and leaves a waiter that
// starts the new one once Sonde has quit.
func (i *appImageInstaller) Install(r Record) error {
	// Checked again: the image may have moved or changed since launch.
	if i.reason = checkAppImage(i.appImage, i.appDir); i.reason != "" {
		return &Error{Kind: KindInstall, Err: errors.New(i.reason)}
	}
	if err := replaceFile(filepath.Dir(i.appImage), filepath.Base(i.appImage), r); err != nil {
		return err
	}
	if err := relaunchAfter(os.Getpid(), i.appImage); err != nil {
		return &Error{Kind: KindInstall, Err: fmt.Errorf("the update is installed, but Sonde could not start it: quit Sonde and start it again (%v)", err)}
	}
	//nolint:forbidigo // the Wails updater's own temp folder
	_ = os.RemoveAll(filepath.Dir(r.Staged))
	i.deps.Quit()
	return nil
}

// replaceFile puts r's staged file in dir as name: copied to a temporary
// name through a directory handle, synced, read back through the same
// descriptor against the signed digest, then renamed over name.
func replaceFile(dir, name string, r Record) (err error) {
	dfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(dfd)
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	tmp := ".Sonde-update-" + hex.EncodeToString(rnd[:]) + ".AppImage"
	fd, err := unix.Openat(dfd, tmp, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o755)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), tmp) //nolint:gosec // a file descriptor
	renamed := false
	defer func() {
		_ = f.Close()
		if !renamed {
			_ = unix.Unlinkat(dfd, tmp, 0)
		}
	}()
	src, err := os.Open(r.Staged)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, src)
	_ = src.Close()
	if err != nil {
		return err
	}
	if err := unix.Fchmod(fd, 0o755); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := rehash(f, r); err != nil {
		return err
	}
	if err := unix.Renameat(dfd, tmp, dfd, name); err != nil {
		return err
	}
	renamed = true
	return unix.Fsync(dfd)
}

// relaunchAfter starts a detached waiter that runs path once pid has
// exited. The path is an argument of the script, never part of it; the
// AppImage runtime's variables are dropped so the new image sets its own.
func relaunchAfter(pid int, path string) error {
	//nolint:gosec // G204: a fixed script; the pid and the path are its arguments
	cmd := exec.Command("/bin/sh", "-c", `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; exec "$2"`, "sh", strconv.Itoa(pid), path)
	for _, kv := range os.Environ() {
		switch k, _, _ := strings.Cut(kv, "="); k {
		case "APPDIR", "APPIMAGE", "ARGV0", "OWD":
		default:
			cmd.Env = append(cmd.Env, kv)
		}
	}
	// Not in the old mount (AppRun's folder): the waiter outlives it.
	cmd.Dir = os.Getenv("OWD")
	if fi, err := os.Stat(cmd.Dir); cmd.Dir == "" || err != nil || !fi.IsDir() {
		cmd.Dir, _ = os.UserHomeDir()
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
