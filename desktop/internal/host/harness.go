// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build e2eharness

package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// HarnessService is the test-only binding set of the e2e harness and the
// spikes: a call, an event, a cancellable call and bodies served by URL.
type HarnessService struct {
	mu     sync.Mutex
	waits  map[string]string
	bodies map[string]*harnessBody
}

type harnessBody struct {
	data     []byte
	kind     string
	written  int64
	finished bool
	aborted  bool
}

// BodyState reports how much of a body was served.
type BodyState struct {
	Written  int64 `json:"written"`
	Finished bool  `json:"finished"`
	Aborted  bool  `json:"aborted"`
}

func init() {
	register("harness", nil, func(*Host) application.Service {
		return application.NewServiceWithOptions(&HarnessService{
			waits:  map[string]string{},
			bodies: map[string]*harnessBody{},
		}, application.ServiceOptions{Name: "harness", Route: spikeBodyPrefix})
	})
}

// Ping answers "pong <name>".
func (s *HarnessService) Ping(name string) string { return "pong " + name }

// spikeBodyPrefix serves the spikes' bodies: the app's own body route is
// the bodies service's, and a page opened by navigation (the sandbox
// check) carries no token, so the route is outside /_sonde/.
const spikeBodyPrefix = "/spike/body/"

// Emit sends the event harness:event with data.
func (s *HarnessService) Emit(data string) {
	application.Get().Event.Emit("harness:event", data)
}

// Wait blocks until the caller cancels it or 30 s pass; WaitState tells
// which ("canceled" or "done").
func (s *HarnessService) Wait(ctx context.Context, id string) error {
	s.setWait(id, "running")
	select {
	case <-ctx.Done():
		s.setWait(id, "canceled")
		return ctx.Err()
	case <-time.After(30 * time.Second):
		s.setWait(id, "done")
		return nil
	}
}

// WaitState is the state of the Wait call id.
func (s *HarnessService) WaitState(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waits[id]
}

func (s *HarnessService) setWait(id, state string) {
	s.mu.Lock()
	s.waits[id] = state
	s.mu.Unlock()
}

// NewBody stores a body of about size bytes: kind "json" (an array of
// objects), "bin" (every byte value), "slow" (bin served at about 12 MB/s,
// so a cancel lands mid-stream), "html" (a page whose script reports to
// its opener or parent when it runs) or "control" (the same page without
// the sandbox policy, proving the check can see a script run). It returns the id to fetch at
// /spike/body/<id>.
func (s *HarnessService) NewBody(size int, kind string) (string, error) {
	if size < 0 || size > 256<<20 {
		return "", errors.New("size out of range")
	}
	var data []byte
	switch kind {
	case "json":
		var b strings.Builder
		b.Grow(size + 128)
		b.WriteByte('[')
		for i := 0; b.Len() < size; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"id":%d,"name":"item %d","price":%d.25,"tags":["a","b"],"big":12345678901234567890}`, i, i, i%1000)
		}
		b.WriteByte(']')
		data = []byte(b.String())
	case "bin", "slow":
		data = make([]byte, size)
		for i := range data {
			data[i] = byte(i)
		}
	case "html", "control":
		data = []byte(`<!doctype html><script>(window.opener||window.parent).postMessage("body-script-ran","*")</script>`)
	default:
		return "", fmt.Errorf("unknown kind %q", kind)
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	key := hex.EncodeToString(id[:])
	s.mu.Lock()
	s.bodies[key] = &harnessBody{data: data, kind: kind}
	s.mu.Unlock()
	return key, nil
}

// BodyState reports how much of body id was served.
func (s *HarnessService) BodyState(id string) BodyState {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bodies[id]
	if b == nil {
		return BodyState{}
	}
	return BodyState{Written: b.written, Finished: b.finished, Aborted: b.aborted}
}

// ServeHTTP serves /spike/body/<id> in 64 KiB chunks, stopping when the
// client goes away. The asset server strips the route, so the path is the
// id.
func (s *HarnessService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path
	s.mu.Lock()
	b := s.bodies[id]
	if b != nil {
		b.written, b.finished, b.aborted = 0, false, false
	}
	s.mu.Unlock()
	if b == nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	switch b.kind {
	case "json":
		h.Set("Content-Type", "application/json")
	case "html", "control":
		h.Set("Content-Type", "text/html; charset=utf-8")
	default:
		h.Set("Content-Type", "application/octet-stream")
	}
	// Add, not Set: the guard's frame-ancestors policy stays in force.
	if b.kind != "control" {
		h.Add("Content-Security-Policy", "sandbox")
	}
	h.Set("Content-Length", strconv.Itoa(len(b.data)))
	flusher, _ := w.(http.Flusher)
	const chunk = 64 << 10
	for off := 0; off < len(b.data); off += chunk {
		if r.Context().Err() != nil {
			s.bodyDone(b, false, true)
			return
		}
		end := min(off+chunk, len(b.data))
		n, err := w.Write(b.data[off:end])
		s.mu.Lock()
		b.written += int64(n)
		s.mu.Unlock()
		if err != nil {
			s.bodyDone(b, false, true)
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		if b.kind == "slow" {
			time.Sleep(5 * time.Millisecond)
		}
	}
	s.bodyDone(b, true, false)
}

func (s *HarnessService) bodyDone(b *harnessBody, finished, aborted bool) {
	s.mu.Lock()
	b.finished, b.aborted = finished, aborted
	s.mu.Unlock()
}

// Spike is the spike the frontend runs on start (DESKTOP_SPIKE), or "".
func (s *HarnessService) Spike() string { return os.Getenv("DESKTOP_SPIKE") }

// Report prints a spike's result and quits.
func (s *HarnessService) Report(result string) {
	fmt.Println("SPIKE RESULT", result)
	go func() {
		time.Sleep(200 * time.Millisecond)
		application.Get().Quit()
	}()
}
