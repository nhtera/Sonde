// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/nhtera/sonde/exchange"
)

// streamServer serves /events (an endless event stream), /finite (three
// events, then close), /ws (echo; "close-me" closes with 4000) and
// /private (401).
func streamServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	events := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for i := range 3 {
			fmt.Fprintf(w, "event: tick\nid: %d\ndata: {\"n\": %d, \"secret\": \"s3cr3t\"}\n\n", i, i)
			f.Flush()
		}
	}
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		events(w)
		<-r.Context().Done()
	})
	mux.HandleFunc("/finite", func(w http.ResponseWriter, _ *http.Request) { events(w) })
	mux.HandleFunc("/private", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow() //nolint:errcheck // test
		for {
			typ, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			if string(data) == "close-me" {
				_ = c.Close(4000, "bye")
				return
			}
			if c.Write(r.Context(), typ, data) != nil {
				return
			}
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type streamRecorder struct {
	recorder
	sent, received int
}

func (r *streamRecorder) on(ev Event) {
	switch ev.(type) {
	case MessageSent:
		r.sent++
	case MessageReceived:
		r.received++
	}
	r.recorder.on(ev)
}

// runSonde runs src as a .sonde file with {{base}} and {{ws}} set.
func runSonde(t *testing.T, name, src string, opt Options) (*UnitResult, *streamRecorder) {
	t.Helper()
	srv := streamServer(t)
	if opt.Variables == nil {
		opt.Variables = map[string]any{}
	}
	opt.Variables["base"] = srv.URL
	opt.Variables["ws"] = "ws" + strings.TrimPrefix(srv.URL, "http")
	rec := &streamRecorder{}
	opt.OnEvent = rec.on
	res, err := NewRunner(opt).RunSource(context.Background(), filepath.Join(t.TempDir(), name), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if res.ParseError != nil {
		t.Fatal(res.ParseError)
	}
	return res, rec
}

func TestRunSSE(t *testing.T) {
	res, rec := runSonde(t, "t.sonde", `GET {{base}}/events
[Options]
sonde-stream-count: 2
HTTP 200
[Captures]
first: sondeStream nth 0 jsonpath "$.n"
[Asserts]
sondeStream count == 2
sondeStream "event" nth 1 == "tick"
sondeStream "id" nth 1 == "1"
sondeStream "retry" nth 0 == null
sondeStream nth 1 jsonpath "$.n" == 1
variable "first" == 0

GET {{base}}/finite
HTTP 200
[Asserts]
sondeStream count == 3
sondeStream "event" contains "tick"
`, Options{Verbosity: Verbose})
	if !res.Success {
		t.Fatalf("errors: %v", res.Errors())
	}
	s := res.Entries[0].Calls[0].Response.Stream
	if s == nil || s.StopReason != exchange.StopCount || s.Protocol != exchange.ProtocolSSE {
		t.Errorf("stream = %+v", s)
	}
	if s := res.Entries[1].Calls[0].Response.Stream; s == nil || s.StopReason != exchange.StopClosed {
		t.Errorf("parsed stream = %+v", s)
	}
	if rec.received != 2 || rec.sent != 0 {
		t.Errorf("events: %d received, %d sent", rec.received, rec.sent)
	}
	if !strings.HasPrefix(res.Entries[0].Curl, "curl --no-buffer ") {
		t.Errorf("curl = %q", res.Entries[0].Curl)
	}
	if debug := rec.text(LogDebug); !strings.Contains(debug, "<< event: tick, id: 1, data:") || !strings.Contains(debug, "Stream stopped: count") {
		t.Errorf("debug output:\n%s", debug)
	}
}

func TestRunSSETimeoutIsSoft(t *testing.T) {
	res, _ := runSonde(t, "t.sonde", `GET {{base}}/events
[Options]
sonde-stream-timeout: 200ms
HTTP 200
[Asserts]
sondeStream count == 4
`, Options{})
	errs := res.Errors()
	if res.Success || len(errs) != 1 || errs[0].Kind() != ErrorAssertFailure || !strings.Contains(errs[0].Error(), "actual:   integer <3>") {
		t.Fatalf("errors: %v", errs)
	}
}

func TestRunWebSocket(t *testing.T) {
	res, rec := runSonde(t, "t.sonde", `GET {{ws}}/ws
X-Test: 1
[SondeMessages]
send: {"type": "ping", "color": "#fff"}
send: `+"`hello`"+`
send: hex,00ff;
receive: {{n}}
close
HTTP 101
[Captures]
kind: sondeStream nth 0 jsonpath "$.type"
[Asserts]
sondeStream count == 3
sondeStream nth 1 == "hello"
sondeStream "type" nth 2 == "binary"
sondeStream nth 2 == hex,00ff;
variable "kind" == "ping"
header "Upgrade" == "websocket"

GET {{base}}/private
[SondeMessages]
receive
HTTP 401
`, Options{Variables: map[string]any{"n": 3}})
	if !res.Success {
		t.Fatalf("errors: %v", res.Errors())
	}
	e := res.Entries[0]
	s := e.Calls[0].Response.Stream
	if s == nil || s.StopReason != exchange.StopScript || len(s.Messages) != 6 {
		t.Fatalf("stream = %+v", s)
	}
	if rec.sent != 3 || rec.received != 3 {
		t.Errorf("events: %d sent, %d received", rec.sent, rec.received)
	}
	if e.Curl != "" {
		t.Errorf("curl = %q", e.Curl)
	}
	var upgrade bool
	for _, h := range e.Calls[0].Request.Headers {
		upgrade = upgrade || h.Name == "Upgrade" && h.Value == "websocket"
	}
	if !upgrade {
		t.Errorf("request headers = %v", e.Calls[0].Request.Headers)
	}
}

func TestRunWebSocketErrors(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		kind      ErrorKind
		line      int
		msg       string
	}{
		{"timeout", "GET {{ws}}/ws\n[Options]\nsonde-stream-timeout: 200ms\n[SondeMessages]\nsend: `a`\nreceive: 2\nHTTP 101\n",
			ErrorStream, 6, "timeout waiting for message 2 of 2"},
		{"server close", "GET {{ws}}/ws\n[SondeMessages]\nsend: `close-me`\nreceive\nHTTP 101\n",
			ErrorStream, 4, "the server closed the connection (code 4000) while waiting for message 1 of 1"},
		{"ws without section", "GET {{ws}}/ws\nHTTP 101\n",
			ErrorInvalidURL, 1, "needs a [SondeMessages] section"},
		{"field of another protocol", "GET {{ws}}/ws\n[SondeMessages]\nsend: `a`\nreceive\nHTTP 101\n[Asserts]\nsondeStream \"event\" count == 1\n",
			ErrorStream, 7, "the field <event> does not apply to a WebSocket stream"},
		{"receive zero", "GET {{ws}}/ws\n[SondeMessages]\nreceive: {{zero}}\nHTTP 101\n",
			ErrorExpressionInvalidType, 3, "integer >= 1"},
		{"reserved close code", "GET {{ws}}/ws\n[SondeMessages]\nclose: {{reserved}}\nHTTP 101\n",
			ErrorExpressionInvalidType, 3, "close code 1000-4999"},
		{"POST", "POST {{ws}}/ws\n[SondeMessages]\nreceive\nHTTP 101\n",
			ErrorStream, 1, "a WebSocket entry must use GET"},
		{"body", "GET {{ws}}/ws\n[SondeMessages]\nreceive\n`body`\nHTTP 101\n",
			ErrorStream, 4, "a WebSocket entry has no request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := runSonde(t, "t.sonde", tc.src, Options{Variables: map[string]any{"zero": 0, "reserved": 1006}})
			errs := res.Errors()
			if len(errs) != 1 || errs[0].Kind() != tc.kind || errs[0].Span().Start.Line != tc.line || !strings.Contains(errs[0].Error(), tc.msg) {
				for _, e := range errs {
					t.Logf("%s line %d: %v", e.Kind(), e.Span().Start.Line, e)
				}
				t.Fatalf("want %s at line %d: %q", tc.kind, tc.line, tc.msg)
			}
		})
	}
}

// TestRunWebSocketRetry checks a retried entry replays its whole script.
func TestRunWebSocketRetry(t *testing.T) {
	res, rec := runSonde(t, "t.sonde", "GET {{ws}}/ws\n[Options]\nretry: 2\n[SondeMessages]\nsend: `a`\nreceive\nHTTP 101\n[Asserts]\nsondeStream nth 0 == \"b\"\n", Options{})
	if res.Success || len(res.Entries) != 3 || rec.sent != 3 {
		t.Errorf("success %v, %d attempts, %d sent", res.Success, len(res.Entries), rec.sent)
	}
}

// TestWebSocketURLInHurl checks a .hurl file keeps the http(s)-only
// message: the WebSocket advice would be a parse error there.
func TestWebSocketURLInHurl(t *testing.T) {
	res, err := NewRunner(Options{}).RunSource(context.Background(), filepath.Join(t.TempDir(), "t.hurl"), []byte("GET ws://localhost/\n"))
	if err != nil {
		t.Fatal(err)
	}
	if errs := res.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "Only <http://> and <https://> schemes are supported") {
		t.Errorf("errors: %v", errs)
	}
}

// TestSondeOnlyInHurl checks a .hurl file can not use Sonde constructs.
func TestSondeOnlyInHurl(t *testing.T) {
	res, err := NewRunner(Options{}).RunSource(context.Background(),
		filepath.Join(t.TempDir(), "t.hurl"), []byte("GET http://localhost/\nHTTP 200\n[Asserts]\nsondeStream count == 3\n"))
	if err != nil || res.ParseError == nil || !strings.Contains(res.ParseError.Error(), "query `sondeStream` requires a .sonde file") {
		t.Errorf("err = %v, parse error = %v", err, res.ParseError)
	}
}

func TestRunSSEMaxBytesWarns(t *testing.T) {
	res, rec := runSonde(t, "t.sonde", "GET {{base}}/events\n[Options]\nsonde-stream-max-bytes: 60\nHTTP 200\n[Asserts]\nsondeStream count == 1\n", Options{})
	if !res.Success {
		t.Fatalf("errors: %v", res.Errors())
	}
	if w := rec.text(LogWarning); !strings.Contains(w, "reached sonde-stream-max-bytes after 1 event(s)") {
		t.Errorf("warnings: %q", w)
	}
}

// TestRunSSEMaxTimeKeepsStream checks max-time fails a stream but keeps
// the events read before it, as a failed WebSocket keeps its messages.
func TestRunSSEMaxTimeKeepsStream(t *testing.T) {
	res, _ := runSonde(t, "t.sonde", "GET {{base}}/events\n[Options]\nsonde-stream-timeout: 1m\nmax-time: 300ms\nHTTP 200\n", Options{})
	errs := res.Errors()
	if len(errs) != 1 || errs[0].Kind() != ErrorHTTP || !strings.Contains(errs[0].Error(), "timed out") {
		t.Fatalf("errors: %v", errs)
	}
	calls := res.Entries[0].Calls
	if len(calls) != 1 || calls[0].Response.Stream == nil || len(calls[0].Response.Stream.Messages) != 3 || calls[0].Response.Stream.StopReason != "" {
		t.Errorf("calls = %+v", calls)
	}
}
