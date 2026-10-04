// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
	"github.com/nhtera/sonde/internal/sandbox"
)

// instancesDir holds one lock file per running window app,
// <user config>/Sonde/instances/<pid>.lock, locked by its process until
// it exits.
const instancesDir = "instances"

// startGrace: a lock file this new may belong to a process that created it
// and is about to lock it; it is never taken for a stale one.
const startGrace = 10 * time.Second

// Instances is this window app's place among the running ones: an update
// must not restart while another one runs the same installed copy.
type Instances struct {
	root *os.Root
	own  *os.File
	name string
}

// LockInstance takes this process's instance lock in the user's own app
// data folder: windows started with other --data folders run the same
// installed copy.
func LockInstance() (*Instances, error) {
	dirs, err := appdirs.Default()
	if err != nil {
		return nil, err
	}
	defer dirs.Close()
	return lockInstance(dirs.Config(), os.Getpid())
}

func lockInstance(config *sandbox.Root, pid int) (*Instances, error) {
	if err := config.MkdirAll(instancesDir, 0o700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Join(config.Dir(), instancesDir))
	if err != nil {
		return nil, err
	}
	name := strconv.Itoa(pid) + ".lock"
	f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o600)
	if err == nil {
		if err = tryLock(f); err != nil {
			_ = f.Close()
		}
	}
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &Instances{root: root, own: f, name: name}, nil
}

// Others counts the other running window apps. A lock file no process
// holds any more is removed.
func (i *Instances) Others() int {
	entries, err := fsReadDir(i.root)
	if err != nil {
		return 0
	}
	n := 0
	for _, name := range entries {
		if name == i.name || !strings.HasSuffix(name, ".lock") {
			continue
		}
		f, err := i.root.OpenFile(name, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		if tryLock(f) != nil {
			n++ // held: that process runs
			_ = f.Close()
			continue
		}
		fi, err := f.Stat()
		_ = unlock(f)
		_ = f.Close()
		if err == nil && time.Since(fi.ModTime()) < startGrace {
			n++ // just created: its process is starting
			continue
		}
		_ = i.root.Remove(name) // stale
	}
	return n
}

// Close releases the lock and removes its file.
func (i *Instances) Close() error {
	_ = unlock(i.own)
	err := i.own.Close()
	return errors.Join(err, i.root.Remove(i.name), i.root.Close())
}

func fsReadDir(root *os.Root) ([]string, error) {
	d, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.Readdirnames(-1)
}
