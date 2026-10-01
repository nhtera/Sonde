// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/enginex"
)

const (
	sessionCookie  = "session-cookie-sentinel-58"  //nolint:gosec // G101: test sentinel
	declaredCookie = "declared-cookie-sentinel-91" //nolint:gosec // G101: test sentinel
)

// TestNoCookieValueInAnyDTO: a session cookie that is not a secret, set by
// one response, sent by the next, printed by the verbose log (its header
// lines and the cookie store) and echoed in a body, never reaches a DTO,
// nor does one a [Cookies] section declares (its log lines and the curl
// command); a short cookie value is masked in the cookie headers and
// lists, and nowhere else (a body's "7" stays).
// maskedPairs is a Cookie header whose every value is masked.
var maskedPairs = regexp.MustCompile(`^\w+=\*\*\*(; \w+=\*\*\*)*$`)

func TestNoCookieValueInAnyDTO(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: sessionCookie, Path: "/", HttpOnly: true}) //nolint:gosec // G124: a test cookie over plain HTTP
			http.SetCookie(w, &http.Cookie{Name: "n", Value: "7", Path: "/", HttpOnly: true})             //nolint:gosec // G124: a test cookie over plain HTTP
			_, _ = io.WriteString(w, `{"id":17,"total":70}`)
		case "/me":
			c, _ := r.Cookie("sid")
			_, _ = io.WriteString(w, `{"echo":"`+c.Value+`"}`) //nolint:gosec // G705: a test echo
		}
	}))
	t.Cleanup(srv.Close)
	src := "GET {{base}}/login\nHTTP 200\n\nGET {{base}}/me\nHTTP 200\n\nGET {{base}}/me\n[Cookies]\nsid: " + declaredCookie + "\nHTTP 200\n"
	r := engine.NewRunner(engine.Options{Variables: map[string]any{"base": srv.URL}, Verbosity: engine.VeryVerbose, BufferedLogs: true})
	enginex.EnableHostEvents(r)
	var (
		mu   sync.Mutex
		dtos []any
		conv *Converter
	)
	bodies := &memBodies{m: map[string][]byte{}}
	r.RunAll(context.Background(), slices.Values([]engine.Job{{Name: filepath.Join(t.TempDir(), "c.hurl"), Source: []byte(src)}}), engine.RunAllOptions{
		Started: func(int, engine.Job) (func(engine.Event), io.Writer) {
			conv = NewConverter("c.hurl", r.Redact, bodies, func(d any) {
				mu.Lock()
				dtos = append(dtos, d)
				mu.Unlock()
			})
			return conv.Handle, nil
		},
		Finished: func(int, engine.Job, *engine.UnitResult, error) bool {
			conv.Flush()
			return true
		},
	})
	var sawSent, sawCookieLine, sawSection, sawCurl bool
	for _, d := range dtos {
		redactcheck.AssertNoSecret(t, "event", d, sessionCookie, declaredCookie)
		switch e := d.(type) {
		case EntryFinished:
			for _, c := range e.Entry.Calls {
				for _, h := range c.Request.Headers {
					if strings.EqualFold(h.Name, "Cookie") {
						sawSent = true
						if !maskedPairs.MatchString(h.Value) {
							t.Errorf("Cookie header %q", h.Value)
						}
					}
				}
				for _, ck := range c.Response.Cookies {
					if ck.Value != "***" {
						t.Errorf("response cookie %s=%q", ck.Name, ck.Value)
					}
				}
			}
		case Log:
			sawSection = sawSection || e.Text == "sid=***"
			sawCurl = sawCurl || strings.Contains(e.Text, "--cookie 'sid=***")
			if strings.HasPrefix(e.Text, "Set-Cookie:") || strings.HasPrefix(e.Text, "Cookie:") {
				sawCookieLine = true
				if strings.Contains(e.Text, "=7") {
					t.Errorf("short cookie value in %q", e.Text)
				}
			}
		}
	}
	if !sawSent || !sawCookieLine || !sawSection || !sawCurl {
		t.Fatalf("sent a cookie %v, logged one %v, its [Cookies] line %v, its curl --cookie %v", sawSent, sawCookieLine, sawSection, sawCurl)
	}
	var sawNumbers bool
	for id, b := range bodies.m {
		redactcheck.AssertNoSecretBytes(t, "body "+id, b, sessionCookie, declaredCookie)
		sawNumbers = sawNumbers || strings.Contains(string(b), `{"id":17,"total":70}`)
	}
	if !sawNumbers {
		t.Error("a short cookie value masked a body")
	}
}
