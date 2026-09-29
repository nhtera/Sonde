// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runsvc

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/nhtera/sonde/desktop/internal/emit"
)

// Bridge limits.
const (
	flushEvery   = 16 * time.Millisecond
	maxBatchLen  = 2000
	maxBatchSize = 8 << 20
)

// Batch is the events of a run since the previous batch, on topic
// "run:<runId>". Seq numbers are consecutive over the run; a gap never
// happens (dropped stream messages are counted, not numbered).
type Batch struct {
	RunID string `json:"runId"`
	Items []Item `json:"items"`
	// Dropped counts stream messages dropped since the previous batch
	// because the frontend could not keep up.
	Dropped int `json:"dropped"`
}

// Item is one event of a run.
type Item struct {
	Seq int64 `json:"seq"`
	// Unit is the job the event belongs to (0-based, in start order).
	Unit  int             `json:"unit"`
	Event json.RawMessage `json:"event"`
}

// Done ends a run, on topic "run:<runId>:done", after its last batch: the
// frontend applies it once it has seen LastSeq.
type Done struct {
	RunID   string   `json:"runId"`
	LastSeq int64    `json:"lastSeq"`
	Summary *Summary `json:"summary"`
}

// Topics of a run.
func topic(runID string) string     { return "run:" + runID }
func doneTopic(runID string) string { return "run:" + runID + ":done" }

// bridge queues a run's events and sends them in batches.
type bridge struct {
	emit  emit.Emitter
	runID string

	mu      sync.Mutex
	seq     int64
	pending []Item
	size    int
	dropped int
	stop    chan struct{}
	stopped chan struct{}
}

func newBridge(e emit.Emitter, runID string) *bridge {
	b := &bridge{emit: e, runID: runID, stop: make(chan struct{}), stopped: make(chan struct{})}
	go b.loop()
	return b
}

func (b *bridge) loop() {
	defer close(b.stopped)
	t := time.NewTicker(flushEvery)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.flush()
		}
	}
}

// add queues an event of unit. A droppable event (a stream message) is
// dropped, and counted, while the queue is over its limits.
func (b *bridge) add(unit int, dto any, droppable bool) {
	data, err := json.Marshal(dto)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if droppable && (len(b.pending) >= maxBatchLen || b.size+len(data) > maxBatchSize) {
		b.dropped++
		return
	}
	b.seq++
	b.pending = append(b.pending, Item{Seq: b.seq, Unit: unit, Event: data})
	b.size += len(data)
}

// flush sends the queued events.
func (b *bridge) flush() {
	b.mu.Lock()
	items, dropped := b.pending, b.dropped
	b.pending, b.size, b.dropped = nil, 0, 0
	b.mu.Unlock()
	if len(items) == 0 && dropped == 0 {
		return
	}
	b.emit.Emit(topic(b.runID), Batch{RunID: b.runID, Items: items, Dropped: dropped})
}

// done flushes the last events, then sends Done.
func (b *bridge) done(s *Summary) {
	close(b.stop)
	<-b.stopped
	b.flush()
	b.mu.Lock()
	last := b.seq
	b.mu.Unlock()
	b.emit.Emit(doneTopic(b.runID), Done{RunID: b.runID, LastSeq: last, Summary: s})
}
