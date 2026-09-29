// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package testsummary

import (
	"testing"
	"time"

	"github.com/nhtera/sonde/engine"
)

func TestWorst(t *testing.T) {
	for _, tc := range []struct{ a, b, want Outcome }{
		{OK, Assert, Assert}, {Assert, Runtime, Runtime}, {Parse, Runtime, Parse}, {Runtime, OK, Runtime}, {OK, OK, OK},
	} {
		if got := Worst(tc.a, tc.b); got != tc.want {
			t.Errorf("Worst(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLineAndSummary(t *testing.T) {
	res := &engine.UnitResult{File: "a.hurl", Success: true, Duration: 12 * time.Millisecond,
		Entries: []*engine.EntryResult{{Calls: make([]engine.Call, 2)}}}
	if got, want := Line(res, false), "Success a.hurl (2 request(s) in 12 ms)\n"; got != want {
		t.Errorf("Line = %q, want %q", got, want)
	}
	if got, want := Line(res, true), "\x1b[1;32mSuccess\x1b[0m \x1b[1ma.hurl\x1b[0m (2 request(s) in 12 ms)\n"; got != want {
		t.Errorf("colored Line = %q", got)
	}
	want := "--------------------------------------------------------------------------------\n" +
		"Executed files:    2\nExecuted requests: 3 (0.0/s)\nSucceeded files:   1 (50.0%)\nFailed files:      1 (50.0%)\n" +
		"Duration:          3723004 ms (1h:2m:3s:4ms)\n\n"
	if got := Summary(2, 1, 3, time.Hour+2*time.Minute+3*time.Second+4*time.Millisecond); got != want {
		t.Errorf("Summary =\n%q\nwant\n%q", got, want)
	}
	if Classify(res) != OK {
		t.Error("Classify success")
	}
}
