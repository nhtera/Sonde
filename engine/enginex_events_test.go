// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/enginex"
)

// hostTrace is the sequence of events of a run, host events named.
type hostTrace struct {
	names []string
	sent  []requestSent
}

func (h *hostTrace) on(ev Event) {
	switch e := ev.(type) {
	case EntryStarted:
		h.names = append(h.names, fmt.Sprintf("started %d", e.Index))
	case EntryFinished:
		h.names = append(h.names, fmt.Sprintf("finished %d", e.Result.Index))
	}
	if _, ok := enginex.UnitStarted(ev); ok {
		h.names = append(h.names, "unit")
	}
	if index, call, req, ok := enginex.RequestSent(ev); ok {
		h.names = append(h.names, fmt.Sprintf("sent %d.%d", index, call))
		h.sent = append(h.sent, requestSent{index: index, call: call, req: req})
	}
	if index, ok := enginex.EntryRedacts(ev); ok {
		h.names = append(h.names, fmt.Sprintf("redacts %d", index))
	}
	if index, reason, ok := enginex.EntrySkipped(ev); ok {
		h.names = append(h.names, fmt.Sprintf("skipped %d %s", index, reason))
	}
}

// runHost runs src with host events on, in dir (a temp dir when empty).
func runHost(t *testing.T, dir, name, src string, opt Options) (*UnitResult, *hostTrace) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	h := &hostTrace{}
	opt.OnEvent = h.on
	r := NewRunner(opt)
	enginex.EnableHostEvents(r)
	res, err := r.RunSource(context.Background(), filepath.Join(dir, name), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return res, h
}

// redirectServer redirects /r/N to /r/N-1, and serves /r/0.
func redirectServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/r/"))
		if n > 0 {
			http.Redirect(w, r, fmt.Sprintf("/r/%d", n-1), http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "done")
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestRequestSentRedirects checks one RequestSent per call, equal to the
// request each Call records, in the order unit, started, sent, finished.
func TestRequestSentRedirects(t *testing.T) {
	srv := redirectServer(t)
	res, h := runHost(t, "", "t.hurl", "GET {{base}}/r/2\n[Options]\nlocation: true\nHTTP 200\n",
		Options{Variables: map[string]any{"base": srv.URL}})
	if !res.Success {
		t.Fatal(res.Errors())
	}
	want := []string{"unit", "started 1", "sent 1.1", "sent 1.2", "sent 1.3", "finished 1"}
	if !reflect.DeepEqual(h.names, want) {
		t.Fatalf("events %v, want %v", h.names, want)
	}
	calls := res.Entries[0].Calls
	for i, s := range h.sent {
		if !reflect.DeepEqual(s.req, beforeSend(calls[i].Request)) {
			t.Errorf("call %d: sent %+v, recorded %+v", i+1, s.req, calls[i].Request)
		}
	}
}

// TestRequestSentRetry numbers the calls of each attempt from 1.
func TestRequestSentRetry(t *testing.T) {
	res, h := runHost(t, "", "t.hurl", "GET {{base}}/flaky\n[Options]\nretry: 5\nretry-interval: 1ms\nHTTP 200\n",
		Options{Variables: map[string]any{"base": server(t).URL}})
	if !res.Success {
		t.Fatal(res.Errors())
	}
	want := []string{"unit", "started 1", "sent 1.1", "finished 1", "started 1", "sent 1.1", "finished 1", "started 1", "sent 1.1", "finished 1"}
	if !reflect.DeepEqual(h.names, want) {
		t.Fatalf("events %v, want %v", h.names, want)
	}
}

// TestRequestSentGRPCReflection reports the call, not the reflection
// calls made to find its descriptors.
func TestRequestSentGRPCReflection(t *testing.T) {
	srv := newGRPCServer(t, false, "v1")
	res, h := runHost(t, "", "t.sonde", "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\n{\"name\": \"x\"}\nHTTP 200\n",
		Options{Variables: map[string]any{"base": srv.URL}})
	if !res.Success {
		t.Fatal(res.Errors())
	}
	want := []string{"unit", "started 1", "sent 1.1", "finished 1"}
	if !reflect.DeepEqual(h.names, want) {
		t.Fatalf("events %v, want %v", h.names, want)
	}
	if !reflect.DeepEqual(h.sent[0].req, beforeSend(res.Entries[0].Calls[0].Request)) {
		t.Errorf("sent %+v, recorded %+v", h.sent[0].req, res.Entries[0].Calls[0].Request)
	}
}

// TestRequestSentWebSocket reports the handshake as sent.
func TestRequestSentWebSocket(t *testing.T) {
	srv := streamServer(t)
	res, h := runHost(t, "", "t.sonde", "GET {{ws}}/ws\n[SondeMessages]\nsend: `hi`\nreceive\nHTTP 101\n",
		Options{Variables: map[string]any{"ws": "ws" + strings.TrimPrefix(srv.URL, "http")}})
	if !res.Success {
		t.Fatal(res.Errors())
	}
	if len(h.sent) != 1 || !reflect.DeepEqual(h.sent[0].req, res.Entries[0].Calls[0].Request) {
		t.Fatalf("sent %+v, recorded %+v", h.sent, res.Entries[0].Calls[0].Request)
	}
}

func TestEntrySkipped(t *testing.T) {
	src := "GET {{base}}/hello\n[Options]\nskip: true\n\nGET {{base}}/hello\n[Options]\nrepeat: 0\n\nGET {{base}}/hello\nHTTP 200\n"
	res, h := runHost(t, "", "t.hurl", src, Options{Variables: map[string]any{"base": server(t).URL}})
	if !res.Success {
		t.Fatal(res.Errors())
	}
	want := []string{"unit", "skipped 1 option", "skipped 2 repeat-zero", "started 3", "sent 3.1", "finished 3"}
	if !reflect.DeepEqual(h.names, want) {
		t.Fatalf("events %v, want %v", h.names, want)
	}
}

// TestNoHostEventsByDefault checks that a runner without EnableHostEvents
// sends exactly the events it always did.
func TestNoHostEventsByDefault(t *testing.T) {
	base, stream := server(t).URL, streamServer(t).URL
	grpc := newGRPCServer(t, false, "v1")
	for name, src := range map[string]string{
		"t.hurl":  "GET {{base}}/hello\n[Options]\nskip: true\n\nGET {{base}}/hello\n[Options]\nrepeat: 0\n\nGET {{base}}/flaky\n[Options]\nretry: 5\nretry-interval: 1ms\nHTTP 200\n",
		"w.sonde": "GET {{ws}}/ws\n[SondeMessages]\nsend: `hi`\nreceive\nHTTP 101\n",
		"r.hurl":  "GET {{base}}/json\nHTTP 200\n[Captures]\ntok: jsonpath \"$.token\" redact\n",
		"g.sonde": "POST {{grpc}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\n{\"name\": \"x\"}\nHTTP 200\n",
	} {
		var events []Event
		r := NewRunner(Options{
			Variables:    map[string]any{"base": base, "ws": "ws" + strings.TrimPrefix(stream, "http"), "grpc": grpc.URL},
			OnEvent:      func(ev Event) { events = append(events, ev) },
			Verbosity:    VeryVerbose,
			BufferedLogs: true,
		})
		res, err := r.RunSource(context.Background(), filepath.Join(t.TempDir(), name), []byte(src))
		if err != nil || !res.Success {
			t.Fatalf("%s: %v %v", name, err, res.Errors())
		}
		for _, ev := range events {
			switch ev.(type) {
			case unitStarted, requestSent, entrySkipped, entryRedacts:
				t.Errorf("%s: host event %T without EnableHostEvents", name, ev)
			}
		}
	}
}

// beforeSend is a recorded request as the sent event has it: its version
// is known once it is sent.
func beforeSend(r exchange.Request) exchange.Request {
	r.Version = ""
	return r
}
