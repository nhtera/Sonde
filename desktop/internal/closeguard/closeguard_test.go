// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package closeguard

import "testing"

func TestGuard(t *testing.T) {
	var asked, quit int
	g := &Guard{Quit: func() { quit++ }, Emit: func(topic string, _ any) {
		if topic == Topic {
			asked++
		}
	}}
	if g.Hold() || asked != 0 {
		t.Fatal("nothing unsaved: the close goes on, unasked")
	}
	g.SetUnsaved(2)
	if !g.Hold() || asked != 1 {
		t.Fatal("unsaved edits: the close waits and the page is asked")
	}
	g.SetUnsaved(0)
	if g.Hold() {
		t.Fatal("saved since: the close goes on")
	}
	g.SetUnsaved(1)
	g.Leave()
	if quit != 1 || g.Hold() {
		t.Fatalf("agreed to leave: quit %d, and nothing holds the quit it starts", quit)
	}
}

func TestAllowAndRestoreExit(t *testing.T) {
	g := &Guard{}
	g.SetUnsaved(1)
	AllowExit(g)
	if g.Hold() {
		t.Fatal("AllowExit: the update's quit is held")
	}
	RestoreExit(g)
	if !g.Hold() {
		t.Fatal("RestoreExit: the quit goes through over unsaved edits")
	}
	g.Leave()
	RestoreExit(g)
	if g.Hold() {
		t.Fatal("RestoreExit undid the page's Leave")
	}
}
