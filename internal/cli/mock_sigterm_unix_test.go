// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cli

import (
	"context"
	"io"
	"syscall"
	"testing"
	"time"
)

// TestMockSIGTERM stops a running mock with SIGTERM: a graceful stop that
// exits 0, unlike Ctrl-C.
func TestMockSIGTERM(t *testing.T) {
	spec := petSpec(t)
	var errOut lockedBuffer
	done := make(chan int, 1)
	go func() { done <- run(context.Background(), []string{"mock", "--port", "0", spec}, io.Discard, &errOut) }()
	for !mockAddrRe.MatchString(errOut.String()) {
		select {
		case code := <-done:
			t.Fatalf("mock exited %d:\n%s", code, errOut.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
	// The mock has caught SIGTERM since before it printed its address.
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != ExitOK {
			t.Errorf("exit %d, want %d\n%s", code, ExitOK, errOut.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the mock did not stop on SIGTERM")
	}
}
