// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// TestHTMLTimeline checks the timeline of a unit page: each call's phases
// as the reference report breaks them down (DNS lookup, TCP handshake, SSL
// handshake, wait, data transfer, total), the waterfall bars laid out on
// the run, and hostile labels escaped.
func TestHTMLTimeline(t *testing.T) {
	ms := time.Millisecond
	call := func(begin time.Duration, url string) engine.Call {
		return engine.Call{
			Request:  exchange.Request{Method: "GET", URL: url},
			Response: &exchange.Response{Status: 200, Version: "HTTP/1.1"},
			Timings: exchange.Timings{Begin: fixedTime.Add(begin), NameLookup: 1 * ms, Connect: 3 * ms, AppConnect: 6 * ms,
				PreTransfer: 6 * ms, StartTransfer: 9 * ms, Total: 10 * ms},
		}
	}
	res := &engine.UnitResult{File: "t.hurl", Success: true, Timestamp: fixedTime, Entries: []*engine.EntryResult{
		{Index: 1, Line: 1, Calls: []engine.Call{call(0, "https://a.test/")}},
		{Index: 2, Line: 3, Calls: []engine.Call{call(10*ms, `https://<script>alert(1)</script>.test/"onmouseover="x`)}},
	}}
	rows := htmlTimeline(res, identityRedact)
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	want := []string{"1.0 ms", "2.0 ms", "3.0 ms", "3.0 ms", "1.0 ms"}
	for i, p := range rows[0].Phases {
		if p.Duration != want[i] {
			t.Errorf("%s: %s, want %s", p.Name, p.Duration, want[i])
		}
	}
	// The run lasts 20 ms: the second call's wait starts at 16 ms.
	if wait := rows[1].Phases[3]; wait.X != 800 || wait.Width != 150 {
		t.Errorf("second call's wait bar at %v, width %v", wait.X, wait.Width)
	}
	if rows[0].Total != "10.0 ms" {
		t.Errorf("total %s", rows[0].Total)
	}

	dir := t.TempDir()
	if err := WriteHTML(dir, []*engine.UnitResult{res}, identityRedact); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "store", "t.hurl.html"))
	if err != nil {
		matches, _ := filepath.Glob(filepath.Join(dir, "store", "*.html"))
		if len(matches) != 1 {
			t.Fatal(err)
		}
		page, _ = os.ReadFile(matches[0])
	}
	html := string(page)
	for _, s := range []string{"<h2>Timeline</h2>", `<svg class="waterfall"`, `<a href="#entry-2">2</a>`, "SSL handshake: 3.0 ms"} {
		if !strings.Contains(html, s) {
			t.Errorf("page lacks %q", s)
		}
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, `"onmouseover="`) {
		t.Error("a hostile label is not escaped")
	}
}

// TestHTMLTimelineEdges checks repeated entries (an anchor per attempt), a
// call without a start time (skipped), and a wait without a pre-transfer
// mark (after the handshakes).
func TestHTMLTimelineEdges(t *testing.T) {
	ms := time.Millisecond
	at := func(begin time.Duration) exchange.Timings {
		return exchange.Timings{Begin: fixedTime.Add(begin), NameLookup: ms, Connect: 2 * ms, AppConnect: 3 * ms, StartTransfer: 5 * ms, Total: 6 * ms}
	}
	resp := &exchange.Response{Status: 200}
	res := &engine.UnitResult{File: "t.hurl", Timestamp: fixedTime, Entries: []*engine.EntryResult{
		{Index: 1, Calls: []engine.Call{{Request: exchange.Request{Method: "GET", URL: "u"}, Response: resp, Timings: at(0)}}},
		{Index: 1, Calls: []engine.Call{{Request: exchange.Request{Method: "GET", URL: "u"}, Response: resp, Timings: at(6 * ms)}}},
		{Index: 2, Calls: []engine.Call{{Request: exchange.Request{Method: "GET", URL: "u"}, Response: resp}}},
	}}
	rows := htmlTimeline(res, identityRedact)
	if len(rows) != 2 || rows[0].Anchor != "entry-1" || rows[1].Anchor != "entry-1-2" {
		t.Fatalf("rows %+v", rows)
	}
	if wait := rows[0].Phases[3]; wait.Duration != "2.0 ms" || wait.X < rows[0].Phases[2].X {
		t.Errorf("wait %+v", wait)
	}
}
