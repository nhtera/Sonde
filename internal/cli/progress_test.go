// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestProgressBarGraphic checks progressBarGraphic against the reference
// implementation's own unit test table (parallel/progress.rs,
// test_progress_bar), byte for byte.
func TestProgressBarGraphic(t *testing.T) {
	tests := []struct {
		current, last int
		want          string
	}{
		{1, 20, "[>                       ] 1/20"},
		{2, 20, "[=>                      ] 2/20"},
		{5, 20, "[====>                   ] 5/20"},
		{10, 20, "[==========>             ] 10/20"},
		{15, 20, "[================>       ] 15/20"},
		{20, 20, "[======================> ] 20/20"},
		{1, 3, "[>                       ] 1/3"},
		{2, 3, "[========>               ] 2/3"},
		{3, 3, "[================>       ] 3/3"},
		{1, 1, "[>                       ] 1/1"},
	}
	for _, tt := range tests {
		if got := progressBarGraphic(tt.current, tt.last); got != tt.want {
			t.Errorf("progressBarGraphic(%d, %d) = %q, want %q", tt.current, tt.last, got, tt.want)
		}
	}
}

func TestJobTotalFormat(t *testing.T) {
	tests := []struct {
		total jobTotal
		done  int
		want  string
	}{
		{newJobTotal(3, 1), 0, "Executed files: 0/3 (0%)\n"},
		{newJobTotal(3, 1), 1, "Executed files: 1/3 (33%)\n"},
		{newJobTotal(4, 25), 75, "Executed files: 75/100 (75%)\n"},
		{newJobTotal(5, -1), 2, "Executed files: 2\n"},
	}
	for _, tt := range tests {
		if got := tt.total.format(tt.done); got != tt.want {
			t.Errorf("format(%d) = %q, want %q", tt.done, got, tt.want)
		}
	}
}

// TestProgressBarSequentialRun exercises the same scenario as the
// conformance fixture tests_ok/progress_bar: two sequential jobs, entries
// spaced further apart than the update throttle so every EntryStarted
// redraws, and checks the exact clear/redraw byte sequence, including the
// unconditional clear-and-completion-line on Finished.
func TestProgressBarSequentialRun(t *testing.T) {
	var buf bytes.Buffer
	pb := newProgressBar(progressTestWithBar, false, 0, newJobTotal(2, 1))

	pb.onEntryStarted(&buf, 0, "a.hurl", 1, 2, 0)
	pb.onEntryStarted(&buf, 0, "a.hurl", 2, 2, 0)
	pb.onFinished(&buf, 0)
	pb.onEntryStarted(&buf, 1, "b.hurl", 1, 1, 0)
	pb.onFinished(&buf, 1)

	want := "" +
		"Executed files: 0/2 (0%)\n" +
		"[>                       ] 1/2 Running a.hurl\n" +
		"\x1b[1A\x1b[K\x1b[1A\x1b[K" +
		"Executed files: 0/2 (0%)\n" +
		"[============>           ] 2/2 Running a.hurl\n" +
		"\x1b[1A\x1b[K\x1b[1A\x1b[K" + // onFinished(0): clear
		"Executed files: 1/2 (50%)\n" +
		"[>                       ] 1/1 Running b.hurl\n" +
		"\x1b[1A\x1b[K\x1b[1A\x1b[K" // onFinished(1): clear
	if buf.String() != want {
		t.Errorf("output =\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestProgressBarModeGating(t *testing.T) {
	tests := []struct {
		test, flag, tty bool
		want            progressMode
	}{
		{false, false, false, progressDefault},
		{false, true, true, progressDefault},
		{true, false, false, progressTestNoBar},
		{true, false, true, progressTestWithBar},
		{true, true, false, progressTestWithBar},
	}
	for _, tt := range tests {
		if got := newProgressMode(tt.test, tt.flag, tt.tty); got != tt.want {
			t.Errorf("newProgressMode(%v, %v, %v) = %v, want %v", tt.test, tt.flag, tt.tty, got, tt.want)
		}
	}
}

// TestProgressBarInactiveModeNoOps checks that every method is a safe
// no-op outside progressTestWithBar, so run.go can call them unconditionally.
func TestProgressBarInactiveModeNoOps(t *testing.T) {
	var buf bytes.Buffer
	pb := newProgressBar(progressDefault, false, 0, newJobTotal(1, 1))
	pb.onEntryStarted(&buf, 0, "a.hurl", 1, 3, 0)
	pb.onFinished(&buf, 0)
	if buf.Len() != 0 {
		t.Errorf("inactive mode wrote %q, want nothing", buf.String())
	}
}

// TestE2EProgressBarStillWorksAtJobsOneBuffered is a regression guard for
// switching the CLI's buffered/live-write decision from rc.jobs > 1 to
// rc.parallel (code-reviewer finding #16): `--test --jobs 1` now buffers
// per-job output (it didn't before), and the progress bar must still draw
// and the file's own completion line must still appear. This exercises
// the real run.go wiring end to end, against an in-process test server
// (not the external conformance harness, so it's fine to run without the
// shared fixture ports).
func TestE2EProgressBarStillWorksAtJobsOneBuffered(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl", "GET "+srv.URL+"/hello\nHTTP 200\n")

	code, _, errOut := runArgs(t, "--test", "--jobs", "1", "--progress-bar", file)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if !strings.Contains(errOut, "Success "+file) {
		t.Errorf("stderr missing the file's own completion line:\n%s", errOut)
	}
	if !strings.Contains(errOut, "Executed files:") {
		t.Errorf("stderr missing the progress bar's own header line:\n%s", errOut)
	}
}
