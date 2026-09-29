// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce is how long the watcher collects changes before reporting them.
const debounce = 100 * time.Millisecond

// watcher reports changed project paths (slash-separated, relative),
// batched.
type watcher struct {
	fs       *fsnotify.Watcher
	root     string
	onChange func(paths []string)

	mu      sync.Mutex
	pending map[string]struct{}
	timer   *time.Timer
	closed  bool
	done    chan struct{}
}

// watch watches every folder of root the tree shows.
func watch(root string, onChange func(paths []string)) (*watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &watcher{fs: fw, root: root, onChange: onChange, pending: map[string]struct{}{}, done: make(chan struct{})}
	w.addTree(root)
	go w.loop()
	return w, nil
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
	_ = w.fs.Close()
	<-w.done
}
