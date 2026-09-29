// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce is how long the watcher collects changes before reporting them.
const debounce = 100 * time.Millisecond

// A project with more entries than maxWatched is polled every pollEvery
// instead: on macOS every watched file holds a file descriptor, and a
// large folder would exhaust them (and break runs).
const (
	maxWatched = 2000
	pollEvery  = 2 * time.Second
)

// watcher reports changed project paths (slash-separated, relative),
// batched.
type watcher struct {
	fs       *fsnotify.Watcher // nil when polling
	stopPoll chan struct{}
	root     string
	onChange func(paths []string)

	mu      sync.Mutex
	pending map[string]struct{}
	timer   *time.Timer
	closed  bool
	done    chan struct{}
}

// watch watches every folder of root the tree shows, or polls it when it
// is large or the system watcher is unavailable.
func watch(root string, onChange func(paths []string)) (*watcher, error) {
	w := &watcher{root: root, onChange: onChange, pending: map[string]struct{}{}, done: make(chan struct{})}
	if snap := snapshot(root, maxWatched+1); len(snap) <= maxWatched {
		if fw, err := fsnotify.NewWatcher(); err == nil {
			w.fs = fw
			w.addTree(root)
			go w.loop()
			return w, nil
		}
	}
	w.stopPoll = make(chan struct{})
	go w.poll()
	return w, nil
}

// entry is a file's size and modification time, for polling.
type entry struct {
	size int64
	mod  time.Time
}

// snapshot lists up to limit entries of root the tree shows.
func snapshot(root string, limit int) map[string]entry {
	out := map[string]entry{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || len(out) >= limit {
			return filepath.SkipDir
		}
		if p != root && strings.HasPrefix(d.Name(), ".") || d.IsDir() && d.Name() == "node_modules" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if p == root {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = entry{size: info.Size(), mod: info.ModTime()}
		return nil
	})
	return out
}

// poll compares snapshots and reports what differs.
func (w *watcher) poll() {
	defer close(w.done)
	prev := snapshot(w.root, maxTreeFiles)
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		select {
		case <-w.stopPoll:
			return
		case <-t.C:
			cur := snapshot(w.root, maxTreeFiles)
			for p, e := range cur {
				if old, ok := prev[p]; !ok || old != e {
					w.mark(p)
				}
			}
			for p := range prev {
				if _, ok := cur[p]; !ok {
					w.mark(p)
				}
			}
			prev = cur
		}
	}
}

// addTree watches dir and its folders, skipping ignored ones.
func (w *watcher) addTree(dir string) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != w.root && ignoredDir(d.Name()) {
			return filepath.SkipDir
		}
		_ = w.fs.Add(p) // a folder the watcher cannot add is left out
		return nil
	})
}

func (w *watcher) loop() {
	defer close(w.done)
	for {
		select {
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			rel, err := filepath.Rel(w.root, ev.Name)
			if err != nil || rel == "." || inIgnored(rel) {
				continue
			}
			if ev.Has(fsnotify.Create) {
				w.addTree(ev.Name) // a new folder: watch it (no-op for a file)
			}
			w.mark(filepath.ToSlash(rel))
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
		}
	}
}

// inIgnored reports whether rel is inside, or is, an ignored folder.
func inIgnored(rel string) bool {
	dir := rel
	for dir != "." && dir != string(filepath.Separator) && dir != "" {
		if ignoredDir(filepath.Base(dir)) {
			return true
		}
		dir = filepath.Dir(dir)
	}
	return false
}

func (w *watcher) mark(rel string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.pending[rel] = struct{}{}
	if w.timer == nil {
		w.timer = time.AfterFunc(debounce, w.flush)
	}
}

func (w *watcher) flush() {
	w.mu.Lock()
	paths := make([]string, 0, len(w.pending))
	for p := range w.pending {
		paths = append(paths, p)
	}
	w.pending = map[string]struct{}{}
	w.timer = nil
	closed := w.closed
	w.mu.Unlock()
	if closed || len(paths) == 0 {
		return
	}
	slices.Sort(paths)
	w.onChange(paths)
}

func (w *watcher) close() {
	w.mu.Lock()
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
	if w.fs != nil {
		_ = w.fs.Close()
	} else {
		close(w.stopPoll)
	}
	<-w.done
}
