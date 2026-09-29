// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package envsvc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/sandbox"
)

// journalFile records a project edit in progress, in the app's config
// folder: each file's previous bytes (or its absence). An edit that the
// app does not finish (a crash, a kill) is rolled back from it at the next
// start, so the project is either wholly before or wholly after an edit.
const journalFile = "edit-journal.json"

type journal struct {
	Dir   string         `json:"dir"`
	Files []journalEntry `json:"files"`
}

type journalEntry struct {
	Rel     string `json:"rel"`
	Existed bool   `json:"existed"`
	Data    []byte `json:"data,omitempty"`
	Perm    uint32 `json:"perm"`
	// Written is the hash of the bytes the edit writes: a file that no
	// longer holds them was changed by hand since, and is left alone.
	Written string `json:"written"`
}

// hook lets tests stop an edit at a write point: it runs before write n
// (0: the journal, 1…: the files, then the journal's removal).
var hook func(n int)

// apply writes edits into root as one transaction: the journal first
// (0600: it may hold secrets), then each file atomically, then the
// journal is removed. A failed write rolls back the files already written.
func apply(appCfg, root *sandbox.Root, edits []config.FileEdit) error {
	j := journal{Dir: root.Dir()}
	for _, e := range edits {
		rel, err := filepath.Rel(root.Dir(), e.Path)
		if err != nil {
			return err
		}
		entry := journalEntry{Rel: filepath.ToSlash(rel), Perm: uint32(e.Perm), Written: hashOf(e.Data)}
		if data, err := root.ReadFile(rel); err == nil {
			entry.Existed, entry.Data = true, data
			if fi, err := root.Stat(rel); err == nil {
				entry.Perm = uint32(fi.Mode().Perm())
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		j.Files = append(j.Files, entry)
	}
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	step := 0
	point := func() {
		if hook != nil {
			hook(step)
		}
		step++
	}
	point()
	if err := appCfg.WriteFileAtomic(journalFile, data, 0o600); err != nil {
		return err
	}
	for i, e := range edits {
		point()
		if err := config.WriteEdits(root, []config.FileEdit{e}); err != nil {
			rollback(root, j.Files[:i])
			_ = appCfg.Remove(journalFile)
			return err
		}
	}
	point()
	return appCfg.Remove(journalFile)
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// recoverEdit rolls back an edit the app did not finish, if the journal
// names root's project.
func recoverEdit(cfg, root *sandbox.Root) {
	data, err := cfg.ReadFile(journalFile)
	if err != nil {
		return
	}
	var j journal
	if json.Unmarshal(data, &j) != nil || j.Dir != root.Dir() {
		return
	}
	rollback(root, j.Files)
	_ = cfg.Remove(journalFile)
}

func rollback(root *sandbox.Root, files []journalEntry) {
	for _, f := range files {
		rel := filepath.FromSlash(f.Rel)
		current, err := root.ReadFile(rel)
		switch {
		case err == nil && hashOf(current) != f.Written:
			continue // not the edit's bytes: the edit never wrote it, or the user changed it since
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil && !f.Existed:
			continue // never created
		}
		if f.Existed {
			_ = root.WriteFileAtomic(rel, f.Data, fs.FileMode(f.Perm))
		} else {
			_ = root.Remove(rel)
		}
	}
}
