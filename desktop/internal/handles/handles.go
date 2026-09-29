// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package handles gives the frontend opaque ids for paths the user picked
// in a native dialog. Bindings never take a raw path from the page: they
// take a handle, which is valid once, for a few minutes, and only for the
// kind of dialog that made it.
package handles

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// TTL is how long a handle stays valid.
const TTL = 5 * time.Minute

// Kind is the dialog a handle came from.
type Kind int

// Kinds.
const (
	OpenFile Kind = iota + 1 // an existing file to read
	OpenDir                  // an existing folder
	SaveFile                 // a file to write
)

// ErrInvalid is returned for an unknown, used, expired or mismatched handle.
var ErrInvalid = errors.New("handles: the selection expired; choose the file again")

// Table holds the pending handles.
type Table struct {
	mu  sync.Mutex
	m   map[string]entry
	now func() time.Time
}

type entry struct {
	path    string
	kind    Kind
	expires time.Time
}

// New returns an empty table.
func New() *Table { return &Table{m: map[string]entry{}, now: time.Now} }

// Put stores path and returns its handle.
func (t *Table) Put(path string, kind Kind) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b[:])
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for k, e := range t.m { // drop expired handles
		if now.After(e.expires) {
			delete(t.m, k)
		}
	}
	t.m[id] = entry{path: path, kind: kind, expires: now.Add(TTL)}
	return id, nil
}

// Take returns the path of handle id and invalidates it.
func (t *Table) Take(id string, kind Kind) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m[id]
	if !ok {
		return "", ErrInvalid
	}
	delete(t.m, id)
	if e.kind != kind || t.now().After(e.expires) {
		return "", ErrInvalid
	}
	return e.path, nil
}
