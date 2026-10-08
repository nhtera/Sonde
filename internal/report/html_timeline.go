// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"time"

	"github.com/nhtera/sonde/engine"
)

// htmlTimelineRow is one call of a unit page's timeline: what it was, and
// its phases, both as figures and as the bars of a waterfall laid out on
// the file's run (x and widths in thousandths of it).
type htmlTimelineRow struct {
	Entry  int
	Anchor string // the id of the attempt's section
	Label  string // "METHOD url"
	Status int
	Phases []htmlPhase
	Total  string
}

// htmlPhase is a phase of a call: DNS lookup, TCP handshake, SSL
// handshake, wait, data transfer (the reference report's breakdown).
type htmlPhase struct {
	Name     string
	Color    string
	Duration string
	X, Width float64
}

// timelinePhases are the phases of a call: their name, color, and their
// bounds in the call's timings.
var timelinePhases = []struct {
	name, color string
	from, to    func(t callTimings) time.Duration
}{
	{"DNS lookup", "#1d9688", func(callTimings) time.Duration { return 0 }, func(t callTimings) time.Duration { return t.NameLookup }},
	{"TCP handshake", "#fa7f03", func(t callTimings) time.Duration { return t.NameLookup }, func(t callTimings) time.Duration { return t.Connect }},
	{"SSL handshake", "#9933ff", func(t callTimings) time.Duration { return t.Connect }, func(t callTimings) time.Duration { return t.AppConnect }},
	// Without a pre-transfer mark (no connection event), the wait starts
	// after the handshakes.
	{"Wait", "#18c852", func(t callTimings) time.Duration { return max(t.PreTransfer, t.AppConnect, t.Connect, t.NameLookup) }, func(t callTimings) time.Duration { return t.StartTransfer }},
	{"Data transfer", "#36a9f9", func(t callTimings) time.Duration { return t.StartTransfer }, func(t callTimings) time.Duration { return t.Total }},
}

// callTimings are the phase timestamps of a call, from its start.
type callTimings struct {
	NameLookup, Connect, AppConnect, PreTransfer, StartTransfer, Total time.Duration
}

// htmlTimeline lays out every call of res on the run of the file. Labels
// go through redact; the template escapes them.
func htmlTimeline(res *engine.UnitResult, redact func(string) string) []htmlTimelineRow {
	var first, last time.Time
	for _, e := range res.Entries {
		for _, c := range e.Calls {
			if c.Timings.Begin.IsZero() {
				continue
			}
			if b := c.Timings.Begin; first.IsZero() || b.Before(first) {
				first = b
			}
			if end := c.Timings.Begin.Add(c.Timings.Total); end.After(last) {
				last = end
			}
		}
	}
	span := last.Sub(first)
	if first.IsZero() || span <= 0 {
		span = time.Millisecond
	}
	scale := func(d time.Duration) float64 { return float64(d) / float64(span) * 1000 }
	var rows []htmlTimelineRow
	anchors := entryAnchors(res)
	for i, e := range res.Entries {
		for _, c := range e.Calls {
			if c.Response == nil || c.Timings.Begin.IsZero() {
				continue
			}
			t := callTimings{c.Timings.NameLookup, c.Timings.Connect, c.Timings.AppConnect, c.Timings.PreTransfer, c.Timings.StartTransfer, c.Timings.Total}
			offset := c.Timings.Begin.Sub(first)
			row := htmlTimelineRow{Entry: e.Index, Anchor: anchors[i], Label: redact(c.Request.Method + " " + c.Request.URL), Status: c.Response.Status, Total: ms(t.Total)}
			for _, p := range timelinePhases {
				from, to := p.from(t), p.to(t)
				if to < from {
					to = from
				}
				row.Phases = append(row.Phases, htmlPhase{Name: p.name, Color: p.color, Duration: ms(to - from),
					X: scale(offset + from), Width: max(scale(to-from), 0)})
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// ms writes a duration in milliseconds with one decimal.
func ms(d time.Duration) string {
	return fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond))
}

// entryAnchors are the ids of the attempts' sections: entry-N for the
// first attempt of entry N, entry-N-K for its K-th (a retry or a repeat).
func entryAnchors(res *engine.UnitResult) []string {
	seen := map[int]int{}
	out := make([]string, len(res.Entries))
	for i, e := range res.Entries {
		seen[e.Index]++
		out[i] = fmt.Sprintf("entry-%d", e.Index)
		if n := seen[e.Index]; n > 1 {
			out[i] = fmt.Sprintf("entry-%d-%d", e.Index, n)
		}
	}
	return out
}
