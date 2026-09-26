// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// testTimeout bounds how long a single assertion waits for a channel event
// before failing, so a stuck twoStageCtrlC hangs the test loudly instead of
// forever.
const testTimeout = 2 * time.Second

// noopReset is passed wherever a test doesn't care about resetSignals:
// the real one (signal.Reset) is a process-wide side effect that must
// never run in a test that isn't specifically checking for it.
func noopReset() {}

func TestTwoStageCtrlCFirstSignalStopsOnly(t *testing.T) {
	sig := make(chan os.Signal, 2)
	done := make(chan struct{})
	defer close(done)
	stop := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go twoStageCtrlC(sig, done, time.Hour, stop, cancel, noopReset)

	sig <- os.Interrupt
	select {
	case <-stop:
	case <-time.After(testTimeout):
		t.Fatal("stop did not close after the first signal")
	}

	// A long grace and no second signal: ctx must still be live.
	select {
	case <-ctx.Done():
		t.Fatal("ctx was canceled after only one signal")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTwoStageCtrlCSecondSignalCancels(t *testing.T) {
	sig := make(chan os.Signal, 2)
	done := make(chan struct{})
	defer close(done)
	stop := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go twoStageCtrlC(sig, done, time.Hour, stop, cancel, noopReset)

	sig <- os.Interrupt
	select {
	case <-stop:
	case <-time.After(testTimeout):
		t.Fatal("stop did not close after the first signal")
	}

	sig <- os.Interrupt
	select {
	case <-ctx.Done():
	case <-time.After(testTimeout):
		t.Fatal("ctx was not canceled after the second signal")
	}
}

func TestTwoStageCtrlCGraceTimeoutCancels(t *testing.T) {
	sig := make(chan os.Signal, 2)
	done := make(chan struct{})
	defer close(done)
	stop := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const grace = 20 * time.Millisecond
	go twoStageCtrlC(sig, done, grace, stop, cancel, noopReset)

	sig <- os.Interrupt
	select {
	case <-stop:
	case <-time.After(testTimeout):
		t.Fatal("stop did not close after the first signal")
	}

	// No second signal: the grace timeout alone must cancel ctx.
	select {
	case <-ctx.Done():
	case <-time.After(testTimeout):
		t.Fatal("ctx was not canceled once the grace period elapsed")
	}
}

func TestTwoStageCtrlCDoneBeforeAnySignal(t *testing.T) {
	sig := make(chan os.Signal, 2)
	done := make(chan struct{})
	stop := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	finished := make(chan struct{})
	go func() {
		twoStageCtrlC(sig, done, time.Hour, stop, cancel, noopReset)
		close(finished)
	}()

	close(done)
	select {
	case <-finished:
	case <-time.After(testTimeout):
		t.Fatal("twoStageCtrlC did not return once done closed")
	}

	select {
	case <-stop:
		t.Error("stop was closed even though the run finished before any signal")
	default:
	}
	if ctx.Err() != nil {
		t.Error("ctx was canceled even though the run finished before any signal")
	}
}

func TestTwoStageCtrlCDoneDuringGraceStillCancels(t *testing.T) {
	// done closing during the grace window is itself one of the select's
	// wake-up cases (a run that finishes on its own after Ctrl-C but
	// before the grace elapses): cancel still runs unconditionally so the
	// context is always torn down, matching Execute's own deferred cancel.
	sig := make(chan os.Signal, 2)
	done := make(chan struct{})
	stop := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	finished := make(chan struct{})
	go func() {
		twoStageCtrlC(sig, done, time.Hour, stop, cancel, noopReset)
		close(finished)
	}()

	sig <- os.Interrupt
	select {
	case <-stop:
	case <-time.After(testTimeout):
		t.Fatal("stop did not close after the first signal")
	}
	close(done)

	select {
	case <-finished:
	case <-time.After(testTimeout):
		t.Fatal("twoStageCtrlC did not return once done closed during the grace window")
	}
	if ctx.Err() == nil {
		t.Error("ctx was not canceled when done closed during the grace window")
	}
}

// TestTwoStageCtrlCResetSignalsTiming checks resetSignals runs exactly
// where the default SIGINT disposition needs restoring: once the run is
// already being forcefully aborted (second signal, or the grace timeout
// alone), never on the first signal by itself, and never when the run
// simply finished on its own before any signal arrived.
func TestTwoStageCtrlCResetSignalsTiming(t *testing.T) {
	t.Run("not called after only the first signal", func(t *testing.T) {
		sig := make(chan os.Signal, 2)
		done := make(chan struct{})
		defer close(done)
		stop := make(chan struct{})
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		var reset atomicBool

		go twoStageCtrlC(sig, done, time.Hour, stop, cancel, reset.set)

		sig <- os.Interrupt
		select {
		case <-stop:
		case <-time.After(testTimeout):
			t.Fatal("stop did not close after the first signal")
		}
		time.Sleep(20 * time.Millisecond)
		if reset.get() {
			t.Error("resetSignals ran after only one signal, want not yet")
		}
	})

	t.Run("called on the second signal", func(t *testing.T) {
		sig := make(chan os.Signal, 2)
		done := make(chan struct{})
		defer close(done)
		stop := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var reset atomicBool

		go twoStageCtrlC(sig, done, time.Hour, stop, cancel, reset.set)

		sig <- os.Interrupt
		<-stop
		sig <- os.Interrupt
		select {
		case <-ctx.Done():
		case <-time.After(testTimeout):
			t.Fatal("ctx was not canceled after the second signal")
		}
		if !reset.get() {
			t.Error("resetSignals did not run after the second signal")
		}
	})

	t.Run("called on the grace timeout alone", func(t *testing.T) {
		sig := make(chan os.Signal, 2)
		done := make(chan struct{})
		defer close(done)
		stop := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var reset atomicBool

		go twoStageCtrlC(sig, done, 20*time.Millisecond, stop, cancel, reset.set)

		sig <- os.Interrupt
		<-stop
		select {
		case <-ctx.Done():
		case <-time.After(testTimeout):
			t.Fatal("ctx was not canceled once the grace period elapsed")
		}
		if !reset.get() {
			t.Error("resetSignals did not run once the grace period elapsed")
		}
	})

	t.Run("never called when the run finishes before any signal", func(t *testing.T) {
		sig := make(chan os.Signal, 2)
		done := make(chan struct{})
		stop := make(chan struct{})
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		var reset atomicBool

		finished := make(chan struct{})
		go func() {
			twoStageCtrlC(sig, done, time.Hour, stop, cancel, reset.set)
			close(finished)
		}()
		close(done)
		select {
		case <-finished:
		case <-time.After(testTimeout):
			t.Fatal("twoStageCtrlC did not return once done closed")
		}
		if reset.get() {
			t.Error("resetSignals ran even though the run finished before any signal")
		}
	})
}

// atomicBool is a race-free boolean flag for a test goroutine to signal
// the main one, without pulling in sync/atomic's typed helpers for a
// single flag.
type atomicBool struct {
	mu  sync.Mutex
	val bool
}

func (a *atomicBool) set()      { a.mu.Lock(); a.val = true; a.mu.Unlock() }
func (a *atomicBool) get() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.val }
