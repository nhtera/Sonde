// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/enginex"
)

// seedFile logs in (a cookie and two captures, one secret), then entry 5
// uses the cookie, both captures, a project value and an [Options]
// variable that overrides a capture.
const seedFile = `GET {{base}}/login
HTTP 200
[Captures]
id: jsonpath "$.id"
over: jsonpath "$.over"

GET {{base}}/token
HTTP 200
[Captures]
tok: jsonpath "$.token" redact

GET {{base}}/ok

GET {{base}}/ok

POST {{base}}/record?id={{id}}
Authorization: Bearer {{tok}}
X-Project: {{proj}}
X-Over: {{over}}
[Options]
variable: over=from-entry
{"id": "{{id}}", "over": "{{over}}"}
HTTP 200
`

func seedServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "cookie-sentinel", Path: "/", HttpOnly: true}) //nolint:gosec // G124: plain-HTTP test server
		_, _ = io.WriteString(w, `{"id": "7", "over": "captured"}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"token": "tok-sentinel"}`)
	})
	mux.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSendSeedParity reruns entry 5 alone the way the desktop's Send does
// (project values in the Job, plain captures in Options.Variables, `redact`
// captures in Options.Secrets, cookies seeded) and checks that the request
// on the wire is the full run's.
func TestSendSeedParity(t *testing.T) {
	srv := seedServer(t)
	name := filepath.Join(t.TempDir(), "t.hurl")
	job := Job{Name: name, Source: []byte(seedFile), Variables: map[string]any{"proj": "p1"}}
	base := map[string]any{"base": srv.URL}
	full := runJobWith(t, NewRunner(Options{Variables: base}), job, nil)
	before := runJobWith(t, NewRunner(Options{Variables: base, ToEntry: 4}), job, nil)

	vars, secrets := map[string]any{"base": srv.URL}, map[string]string{}
	for i, e := range before.Entries {
		for j, c := range e.Captures {
			if enginex.CaptureRedacted(before, i, j) {
				s, _ := c.Value.Text()
				secrets[c.Name] = s
			} else {
				vars[c.Name] = c.Value
			}
		}
	}
	if !reflect.DeepEqual(secrets, map[string]string{"tok": "tok-sentinel"}) || len(vars) != 3 {
		t.Fatalf("captures: vars %v, secrets %v", vars, secrets)
	}
	var logs []string
	r := NewRunner(Options{Variables: vars, Secrets: secrets, FromEntry: 5, ToEntry: 5, Verbosity: Verbose})
	enginex.SeedCookies(r, before.Cookies)
	send := runJobWith(t, r, job, func(ev Event) {
		if l, ok := ev.(Log); ok {
			logs = append(logs, l.Text)
		}
	})

	want, got := full.Entries[len(full.Entries)-1].Calls[0].Request, send.Entries[0].Calls[0].Request
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("wire request differs:\nfull %+v\nsend %+v", want, got)
	}
	text := string(got.Body) + got.URL
	for _, h := range got.Headers {
		text += h.Name + ": " + h.Value + "\n"
	}
	for _, s := range []string{"cookie-sentinel", "tok-sentinel", "p1", "from-entry", "id=7"} {
		if !strings.Contains(text, s) {
			t.Errorf("the request lacks %q:\n%s", s, text)
		}
	}
	all := strings.Join(logs, "\n")
	if strings.Contains(all, "tok-sentinel") || !strings.Contains(all, "***") {
		t.Errorf("logs are not redacted:\n%s", all)
	}
}

// runJobWith runs one job on r, its events sent to onEvent.
func runJobWith(t *testing.T, r *Runner, job Job, onEvent func(Event)) *UnitResult {
	t.Helper()
	var res *UnitResult
	r.RunAll(context.Background(), slices.Values([]Job{job}), RunAllOptions{
		Started: func(int, Job) (func(Event), io.Writer) { return onEvent, nil },
		Finished: func(_ int, _ Job, u *UnitResult, err error) bool {
			if err != nil {
				t.Error(err)
			}
			res = u
			return true
		},
	})
	if res == nil {
		t.FailNow()
	}
	if !res.Success {
		t.Fatalf("errors: %v parse=%v interrupted=%v entries=%d", res.Errors(), res.ParseError, res.Interrupted, len(res.Entries))
	}
	return res
}
