// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package history

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/report"
	"github.com/nhtera/sonde/internal/sandbox"
)

const (
	accessToken   = "access-sentinel-5521"                      //nolint:gosec // G101: test sentinel
	jwt           = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI3In0.c2ln" //nolint:gosec // G101: test sentinel
	redactedValue = "redacted-sentinel-908"                     //nolint:gosec // G101: test sentinel
	cookieValue   = "cookie-sentinel-771"
	basic         = "dXNlcjpzZW50aW5lbA=="
	orderID       = "order-4242"
)

func result(t *testing.T) *engine.UnitResult {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: cookieValue, Path: "/"}) //nolint:gosec // test cookie
		w.Header().Set("X-Redact", redactedValue)
		_, _ = io.WriteString(w, `{"access_token":"`+accessToken+`","id_value":"`+jwt+`","order":"`+orderID+`"}`)
	}))
	t.Cleanup(srv.Close)
	src := `POST ` + srv.URL + `/login
Authorization: Basic ` + basic + `
HTTP 200
[Captures]
access_token: jsonpath "$.access_token"
id_value: jsonpath "$.id_value"
order_id: jsonpath "$.order"
hidden: header "X-Redact" redact
plain_sid: cookie "sid"

GET ` + srv.URL + `/orders/{{order_id}}?t={{access_token}}
Authorization: Bearer {{id_value}}
HTTP 200
`
	r := engine.NewRunner(engine.Options{})
	res, err := r.RunSource(context.Background(), filepath.Join(t.TempDir(), "a.hurl"), []byte(src))
	if err != nil || !res.Success {
		t.Fatalf("run: %v %v", err, res.Errors())
	}
	return res
}

func newHistory(t *testing.T, p Policy) (*History, string) {
	t.Helper()
	cfgDir := t.TempDir()
	cfg, _ := sandbox.Open(cfgDir)
	proj, _ := sandbox.Open(t.TempDir())
	return New(cfg, func() *sandbox.Root { return proj }, func() Policy { return p }, nil), cfgDir
}

func summary(id string, at time.Time) *runsvc.Summary {
	return &runsvc.Summary{RunID: id, Kind: "run", Env: "local", Outcome: runsvc.Passed, StartedAt: at,
		Units: []runsvc.Unit{{File: "a.hurl", Success: true, Requests: 2}}, Requests: 2, Succeeded: 1, Files: 1}
}

func TestStoredRunIsRedacted(t *testing.T) {
	h, cfgDir := newHistory(t, Policy{Enabled: true})
	res := result(t)
	h.Add(summary("r1", time.Now()), []*engine.UnitResult{res})
	var files []string
	_ = filepath.Walk(filepath.Join(cfgDir, Dir), func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 1 {
		t.Fatalf("%d history files", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	redactcheck.AssertNoSecretBytes(t, "history", data, accessToken, jwt, redactedValue, cookieValue, basic)
	if !strings.Contains(string(data), orderID) {
		t.Error("a plain capture (an id) must stay readable")
	}
	if fi, _ := os.Stat(files[0]); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	items, err := h.List()
	if err != nil || len(items) != 1 || items[0].Files[0] != "a.hurl" || items[0].Env != "local" {
		t.Fatalf("list %+v %v", items, err)
	}
	// Each request, last first, from the record: never a secret, the
	// query's token included.
	calls := items[0].Calls
	if len(calls) != 2 || calls[0].Entry != 2 || calls[0].Method != "GET" || calls[1].Method != "POST" || calls[0].Status != 200 || calls[0].File != "a.hurl" || calls[0].Result != 0 {
		t.Fatalf("calls %+v", calls)
	}
	if !strings.Contains(calls[0].URL, "/orders/"+orderID) {
		t.Errorf("url %q", calls[0].URL)
	}
	listed, _ := json.Marshal(items)
	redactcheck.AssertNoSecretBytes(t, "history list", listed, accessToken, jwt, redactedValue, cookieValue, basic)
	rec, err := h.Get(items[0].ID)
	if err != nil || len(rec.Results) != 1 || len(rec.Results[0].Entries) != 2 {
		t.Fatalf("get %+v %v", rec, err)
	}
	if _, err := h.Get("../x"); err == nil {
		t.Error("a path as id")
	}
}

func TestPolicy(t *testing.T) {
	h, _ := newHistory(t, Policy{Enabled: false})
	h.Add(summary("r1", time.Now()), nil)
	if items, _ := h.List(); len(items) != 0 {
		t.Error("stored with history off")
	}
	h, _ = newHistory(t, Policy{Enabled: true, Retention: 7 * 24 * time.Hour})
	h.Add(summary("old", time.Now().Add(-8*24*time.Hour)), nil)
	h.Add(summary("new", time.Now()), nil)
	items, _ := h.List()
	if len(items) != 1 || !strings.HasSuffix(items[0].ID, "new") {
		t.Errorf("after retention %+v", items)
	}
	if err := h.Clear(); err != nil {
		t.Fatal(err)
	}
	if items, _ := h.List(); len(items) != 0 {
		t.Errorf("after clear %+v", items)
	}
}

// A request whose assert failed is marked, whatever its status.
func TestCallsFailedAssert(t *testing.T) {
	rec := &Record{Summary: &runsvc.Summary{}, Results: []report.Result{{Filename: "a.hurl", Entries: []report.Entry{
		{Index: 1, Calls: []report.Call{{Response: report.Response{Status: 200}}}, Asserts: []report.Assert{{Success: true}}},
		{Index: 2, Calls: []report.Call{{Response: report.Response{Status: 200}}}, Asserts: []report.Assert{{Success: true}, {Success: false}}},
	}}}}
	got := calls(rec)
	if len(got) != 2 || got[0].Entry != 2 || !got[0].Failed || got[1].Failed {
		t.Fatalf("calls %+v", got)
	}
}
