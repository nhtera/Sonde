// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package handles

import (
	"errors"
	"testing"
	"time"
)

func TestTakeOnce(t *testing.T) {
	tb := New()
	id, err := tb.Put("/p/a.hurl", OpenFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Errorf("id %q is not 128 bits of hex", id)
	}
	if p, err := tb.Take(id, OpenFile); err != nil || p != "/p/a.hurl" {
		t.Fatalf("take: %q %v", p, err)
	}
	if _, err := tb.Take(id, OpenFile); !errors.Is(err, ErrInvalid) {
		t.Errorf("second take: %v", err)
	}
}

func TestKindAndExpiry(t *testing.T) {
	tb := New()
	now := time.Now()
	tb.now = func() time.Time { return now }
	id, _ := tb.Put("/p", OpenDir)
	if _, err := tb.Take(id, SaveFile); !errors.Is(err, ErrInvalid) {
		t.Errorf("wrong kind: %v", err)
	}
	if _, err := tb.Take(id, OpenDir); !errors.Is(err, ErrInvalid) {
		t.Errorf("a mismatched take must still consume the handle: %v", err)
	}
	id, _ = tb.Put("/p", OpenDir)
	now = now.Add(TTL + time.Second)
	if _, err := tb.Take(id, OpenDir); !errors.Is(err, ErrInvalid) {
		t.Errorf("expired: %v", err)
	}
	if _, err := tb.Take("nope", OpenDir); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown: %v", err)
	}
}

func TestPutDropsExpired(t *testing.T) {
	tb := New()
	now := time.Now()
	tb.now = func() time.Time { return now }
	_, _ = tb.Put("/a", OpenFile)
	now = now.Add(TTL + time.Second)
	_, _ = tb.Put("/b", OpenFile)
	if len(tb.m) != 1 {
		t.Errorf("%d handles kept, want 1", len(tb.m))
	}
}
