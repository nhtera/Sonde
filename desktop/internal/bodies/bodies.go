// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package bodies keeps the response bodies of runs and serves them to the
// page at /_sonde/body/<id>. What it serves, and what it may write to
// disk, is the redacted body; the raw decoded body stays in memory, for
// the user's own Save response. Bodies stay in memory up to a limit, then
// the oldest spill to the cache folder.
package bodies

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/nhtera/sonde/internal/sandbox"
)

// Limits.
const (
	MemoryLimit = 512 << 20 // redacted bytes kept in memory
	DiskLimit   = 2 << 30   // spilled bytes kept on disk
	spillDir    = "bodies"
	chunk       = 64 << 10
)

// Store keeps bodies by id.
type Store struct {
	spill *sandbox.Root // the app's cache folder; nil: no spilling

	mu       sync.Mutex
	items    map[string]*list.Element
	lru      *list.List // front: most recently used
	memory   int64
	disk     int64
	memLimit int64
	dskLimit int64
}

type item struct {
	id          string
	contentType string
	size        int64
	redacted    []byte // nil once spilled
	raw         []byte // nil once spilled: raw bytes never reach the disk
}

// New returns a store that spills to spill (the app's cache folder;
// nil: bodies past the memory limit are dropped). Earlier spill files are
// removed.
func New(spill *sandbox.Root) *Store {
	s := &Store{spill: spill, items: map[string]*list.Element{}, lru: list.New(), memLimit: MemoryLimit, dskLimit: DiskLimit}
	if spill != nil {
		if entries, err := spill.ReadDir(spillDir); err == nil {
			for _, e := range entries {
				_ = spill.Remove(spillDir + "/" + e.Name())
			}
		}
		_ = spill.MkdirAll(spillDir, 0o700)
	}
	return s
}

// Put stores a body and returns its id (128 random bits).
func (s *Store) Put(redacted, raw []byte, contentType string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	it := &item{id: id, contentType: contentType, size: int64(len(redacted)), redacted: redacted, raw: raw}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = s.lru.PushFront(it)
	s.memory += it.size
	s.evict()
	return id
}

// evict spills or drops the least recently used bodies over the limits.
func (s *Store) evict() {
	for e := s.lru.Back(); e != nil && s.memory > s.memLimit; {
		prev := e.Prev()
		it := e.Value.(*item)
		if it.redacted != nil {
			s.memory -= it.size
			if s.spill != nil && s.spill.WriteFileAtomic(spillDir+"/"+it.id, it.redacted, 0o600) == nil {
				s.disk += it.size
				it.redacted, it.raw = nil, nil
			} else {
				s.drop(e)
			}
		}
		e = prev
	}
	for e := s.lru.Back(); e != nil && s.disk > s.dskLimit; {
		prev := e.Prev()
		if it := e.Value.(*item); it.redacted == nil {
			s.drop(e)
		}
		e = prev
	}
}

func (s *Store) drop(e *list.Element) {
	it := e.Value.(*item)
	s.lru.Remove(e)
	delete(s.items, it.id)
	if it.redacted == nil && s.spill != nil {
		s.disk -= it.size
		_ = s.spill.Remove(spillDir + "/" + it.id)
	} else {
		s.memory -= it.size
	}
}

// get returns a body's redacted bytes and content type.
func (s *Store) get(id string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[id]
	if !ok {
		return nil, "", false
	}
	s.lru.MoveToFront(e)
	it := e.Value.(*item)
	if it.redacted != nil {
		return it.redacted, it.contentType, true
	}
	data, err := s.spill.ReadFile(spillDir + "/" + it.id)
	return data, it.contentType, err == nil
}

// Raw returns a body's raw decoded bytes, while it is still in memory.
func (s *Store) Raw(id string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[id]
	if !ok {
		return nil, "", false
	}
	it := e.Value.(*item)
	return it.raw, it.contentType, it.raw != nil
}

// Redacted returns a body's redacted bytes.
func (s *Store) Redacted(id string) ([]byte, string, bool) { return s.get(id) }

// ServeHTTP serves the body whose id is the request path (the asset
// server removes the route): sandboxed, never sniffed, in chunks, and it
// stops when the page cancels the fetch.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	data, ct, ok := s.get(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	// Add, not Set: server mode's own policy (frame-ancestors) stays.
	h.Add("Content-Security-Policy", "sandbox; default-src 'none'; img-src data:; style-src 'unsafe-inline'")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		return
	}
	rc := http.NewResponseController(w)
	// A large body through a slow tunnel may outlast the server's
	// timeout; the page's cancel still ends it.
	_ = rc.SetWriteDeadline(time.Time{})
	for off := 0; off < len(data); off += chunk {
		if r.Context().Err() != nil {
			return
		}
		if _, err := w.Write(data[off:min(off+chunk, len(data))]); err != nil {
			return
		}
		_ = rc.Flush()
	}
}
