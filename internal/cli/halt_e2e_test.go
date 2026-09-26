// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestE2EParseErrorHaltsInFlightSiblings checks code-reviewer finding High
// #2/#3 (upstream parity): a parse error in one file, running alongside a
// slow one, must end the slow file's run at its next entry boundary — not
// let it run to completion — instead of merely stopping new files from
// being scheduled.
//
// The delay is on the server side (holding the first entry's response),
// not a `delay:` option: that option sleeps through
// `u.io.stop`-aware code and would itself be interrupted by the halt,
// confounding "entry 1 ran to completion" with "entry 1 never started".
// An in-flight HTTP request, once sent, is not interrupted mid-flight —
// only the next entry boundary is — so holding entry 1's response for
// longer than the near-instant parse error takes to detect reliably lets
// entry 1 complete and leaves only entry 2 to prove was actually halted.
func TestE2EParseErrorHaltsInFlightSiblings(t *testing.T) {
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			time.Sleep(300 * time.Millisecond)
		}
		_, _ = w.Write([]byte("Hello World!"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	slow := writeTemp(t, "slow.hurl",
		"GET "+srv.URL+"/hello\nHTTP 200\n\nGET "+srv.URL+"/hello\nHTTP 200\n")
	bad := writeTemp(t, "bad.hurl", "not a hurl file at all {{{")

	code, _, errOut := runArgs(t, "--test", "--jobs", "2", slow, bad)
	if code != ExitParse {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitParse, errOut)
	}
	if got := atomic.LoadInt32(&hits); got >= 2 {
		t.Errorf("server saw %d requests, want at most 1: the slow file's second entry ran after the parse error instead of being halted", got)
	}
}
