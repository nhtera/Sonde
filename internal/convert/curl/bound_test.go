// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/syntax"
)

// TestImportBoundsPathologicalDollarExpansions is the C1 regression test
// (phase 8 review): a word built from many thousands of unterminated "$("
// runs followed by one real ")" used to make tokenize re-scan almost the
// whole remaining input once per "$(" (O(N²) time and memory, since each
// full span was kept as its own warning snippet), reproduced by the
// reviewer with "$(" repeated 5000+ times. It must now finish in a small,
// input-proportional amount of time no matter how many repeats there are.
func TestImportBoundsPathologicalDollarExpansions(t *testing.T) {
	const repeats = 40000 // the reviewer's worst measured case (~80 KB, 3.5 GB, 45s unbounded)
	var b strings.Builder
	b.WriteString(`curl http://a/ "`)
	for i := 0; i < repeats; i++ {
		b.WriteString("$(")
	}
	b.WriteString(`)"`)
	data := []byte(b.String())

	start := time.Now()
	done := make(chan struct{})
	var res Result
	var err error
	go func() {
		res, err = Import(data, syntax.DialectSonde)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Import took longer than 5s for %d repeats (was O(N²) before the C1 fix)", repeats)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Import took %s for %d repeats, want well under 5s", elapsed, repeats)
	}
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// The warning list must stay small (capped, not one entry per "$(").
	if n := len(res.Warnings); n > maxUnevaluatedWarnings+5 {
		t.Errorf("got %d warnings, want at most around maxUnevaluatedWarnings (%d)", n, maxUnevaluatedWarnings)
	}
}

// TestImportCapsCommandCount is the command-count half of C1: an input
// with more curl commands than maxCommands must still return promptly,
// importing only the first maxCommands and warning about the rest, rather
// than doing unbounded work per call.
func TestImportCapsCommandCount(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxCommands+50; i++ {
		b.WriteString("curl http://a/\n")
	}
	res, err := Import([]byte(b.String()), syntax.DialectSonde)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if got := len(res.File.Entries); got != maxCommands {
		t.Errorf("got %d entries, want %d (the cap)", got, maxCommands)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "were not imported") {
			found = true
		}
	}
	if !found {
		t.Error("missing a warning about the truncated commands")
	}
}
