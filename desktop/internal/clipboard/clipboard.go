// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package clipboard writes revealed secrets (a curl or sonde command with
// its credentials) to the system clipboard from Go, so they never pass
// through the page: marked concealed and transient where the system
// supports it (clipboard managers and history skip them), and cleared
// after ClearAfter unless something else was copied since. The window app
// only: server mode has no reveal.
package clipboard

import (
	"sync"
	"time"
)

// ClearAfter is how long a secret stays on the clipboard.
const ClearAfter = 60 * time.Second

var (
	mu    sync.Mutex
	timer *time.Timer
)

// WriteSecret puts text on the clipboard, concealed, and clears it after
// ClearAfter if it is still there.
func WriteSecret(text string) error {
	token, err := writeConcealed(text)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	timer = time.AfterFunc(ClearAfter, func() { clearIf(token) })
	return nil
}
