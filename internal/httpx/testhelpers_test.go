// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"testing"

	"github.com/nhtera/sonde/internal/sandbox"
)

// newTestClient returns a client rooted at a fresh temp sandbox, closed
// automatically at the end of the test.
func newTestClient(t *testing.T, cfg ClientConfig) *Client {
	t.Helper()
	box, err := sandbox.Open(t.TempDir())
	if err != nil {
		t.Fatalf("sandbox.Open: %v", err)
	}
	t.Cleanup(func() { _ = box.Close() })
	cfg.Sandbox = box
	if cfg.Version == "" {
		cfg.Version = "test"
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
