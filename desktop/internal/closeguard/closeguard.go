// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package closeguard holds the window app's close and quit back while the
// page has unsaved edits: the page reports how many tabs are unsaved, and
// closing the window or quitting (the close button, ⇧⌘W, ⌘Q, the menu)
// asks the page to confirm first. A page's beforeunload does not run for
// a native close.
package closeguard

import "sync/atomic"

// Topic is the event that asks the page to confirm leaving with unsaved
// edits; it calls Leave when the user agrees.
const Topic = "app:leave"

// Guard is the window-only binding. Quit and Emit are set by the window
// app before it runs.
type Guard struct {
	unsaved atomic.Int32
	leaving atomic.Bool
	// exiting is set by AllowExit (Go only): an update is quitting the app
	// after the user agreed.
	exiting atomic.Bool
	// Quit quits the app.
	Quit func()
	// Emit sends an event to the page.
	Emit func(topic string, data any)
}

// SetUnsaved records how many tabs have unsaved edits.
func (g *Guard) SetUnsaved(n int) {
	g.unsaved.Store(int32(max(n, 0))) //nolint:gosec // a count of tabs
}

// Leave quits without saving, once the user agreed to.
func (g *Guard) Leave() {
	g.leaving.Store(true)
	if g.Quit != nil {
		g.Quit()
	}
}

// Hold reports whether a close or a quit must wait for the page: there
// are unsaved edits and the user has not agreed to leave. When it holds,
// it asks the page to confirm.
func (g *Guard) Hold() bool {
	if g.leaving.Load() || g.exiting.Load() || g.unsaved.Load() == 0 {
		return false
	}
	if g.Emit != nil {
		g.Emit(Topic, nil)
	}
	return true
}

// AllowExit lets the next close or quit through, for an update that quits
// the app once the user agreed. A package function, not a method: the page
// binds Guard's methods and must never call it.
func AllowExit(g *Guard) { g.exiting.Store(true) }

// RestoreExit holds the close and the quit again after an update did not
// quit after all. A Leave the page asked for stays.
func RestoreExit(g *Guard) { g.exiting.Store(false) }
