// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
)

// TestRecordRunEvents writes the events of shop-api runs, as the page
// receives them, into $SONDE_RECORD_DIR (make desktop-record): the
// frontend's run component tests replay them.
func TestRecordRunEvents(t *testing.T) {
	out := os.Getenv("SONDE_RECORD_DIR")
	if out == "" {
		t.Skip("SONDE_RECORD_DIR not set")
	}
	s := newShop(t)
	record := func(name string, run func()) {
		t.Helper()
		before := len(s.rec.Events())
		run()
		data, err := json.MarshalIndent(s.rec.Events()[before:], "", " ")
		if err != nil {
			t.Fatal(err)
		}
		// The fixture's port and the project's folder change each run.
		data = []byte(strings.NewReplacer(s.url, "http://localhost:8080", s.dir, "/project").Replace(string(data)))
		if err := os.WriteFile(filepath.Join(out, name+".json"), append(data, '\n'), 0o600); err != nil { //nolint:gosec // G703: the test author's own folder
			t.Fatal(err)
		}
	}
	record("checkout", func() { s.run(t, "checkout", "checkout.hurl") })
	record("data-login", func() {
		// Row 2's password is wrong: its capture fails.
		csv := filepath.Join(s.dir, "data", "rows.csv")
		if err := os.WriteFile(csv, []byte("user,pass\nada,fixture-password-3141\nada,wrong\nada,fixture-password-3141\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		hd, err := s.h.Handles.Put(csv, handles.OpenFile)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.h.Runs.RunData(context.Background(), runsvc.DataRequest{RunID: "data-login", File: "data-login.hurl",
			Source: s.source(t, "data-login.hurl"), Env: "local", DataHandle: hd, Secrets: []string{"pass"}}); err != nil {
			t.Fatal(err)
		}
	})
}
