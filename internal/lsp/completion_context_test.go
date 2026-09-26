// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import "testing"

// isMethodLine itself is internal/syntax.IsMethodLine (exported so both
// packages share one heuristic); its behavior is tested there.

func TestIsStatusLine(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"HTTP/1.1 200", true},
		{"HTTP 200", true},
		{"HTTP", true},
		{"HTTP2 foo", false},
		{"GET /", false},
	}
	for _, tt := range tests {
		if got := isStatusLine(tt.s); got != tt.want {
			t.Errorf("isStatusLine(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestSectionHeaderName(t *testing.T) {
	if name, ok := sectionHeaderName("[Asserts]"); !ok || name != "Asserts" {
		t.Errorf("sectionHeaderName = %q, %v", name, ok)
	}
	if _, ok := sectionHeaderName("[Asserts"); ok {
		t.Error("sectionHeaderName should reject an unclosed bracket")
	}
	if _, ok := sectionHeaderName("status == 200"); ok {
		t.Error("sectionHeaderName should reject a non-bracket line")
	}
}

func TestAtFirstToken(t *testing.T) {
	tests := []struct {
		text  string
		col   int
		start int
		ok    bool
	}{
		{"GET ", 3, 0, true},              // still finishing "GET"
		{"GET /foo", 5, 0, false},         // past the method into the URL
		{"  Con", 5, 2, true},             // indented, mid first token
		{"Content-Type: x", 14, 0, false}, // past the colon
		{"", 0, 0, true},
	}
	for _, tt := range tests {
		start, ok := atFirstToken(tt.text, tt.col)
		if ok != tt.ok || (ok && start != tt.start) {
			t.Errorf("atFirstToken(%q, %d) = %d, %v; want %d, %v", tt.text, tt.col, start, ok, tt.start, tt.ok)
		}
	}
}

func TestBracketContext(t *testing.T) {
	tests := []struct {
		text  string
		col   int
		start int
		ok    bool
	}{
		{"[", 1, 1, true},
		{"[Opt", 4, 1, true},
		{"[Options]", 9, 1, false}, // closed before the cursor
		{"[Options]", 8, 1, true},  // just before the closing bracket
		{"  [Query]", 3, 3, true},
		{"status == 200", 6, 0, false},
	}
	for _, tt := range tests {
		start, ok := bracketContext(tt.text, tt.col)
		if ok != tt.ok || (ok && start != tt.start) {
			t.Errorf("bracketContext(%q, %d) = %d, %v; want %d, %v", tt.text, tt.col, start, ok, tt.start, tt.ok)
		}
	}
}

func TestSegments(t *testing.T) {
	line := `header "X-Foo" contains "bar baz"`
	segs := segments(line, 0, len(line))
	want := []string{"header", `"X-Foo"`, "contains", `"bar baz"`}
	if len(segs) != len(want) {
		t.Fatalf("segments = %+v, want %v", segs, want)
	}
	for i, s := range segs {
		if s.text != want[i] {
			t.Errorf("segs[%d].text = %q, want %q", i, s.text, want[i])
		}
	}
}

func TestSegmentsPlaceholder(t *testing.T) {
	line := `status == {{expected status}}`
	segs := segments(line, 0, len(line))
	want := []string{"status", "==", "{{expected status}}"}
	if len(segs) != len(want) {
		t.Fatalf("segments = %+v, want %v", segs, want)
	}
	for i, s := range segs {
		if s.text != want[i] {
			t.Errorf("segs[%d].text = %q, want %q", i, s.text, want[i])
		}
	}
}

func TestSegmentPosition(t *testing.T) {
	line := "status == 200"
	segs := segments(line, 0, len(line))
	tests := []struct {
		offset int
		want   int
	}{
		{0, 0},         // "s|tatus"
		{6, 0},         // "status|"
		{7, 1},         // "status |==" (whitespace before ==, counts as position of next segment)
		{len(line), 2}, // at the very end
	}
	for _, tt := range tests {
		if got := segmentPosition(segs, tt.offset); got != tt.want {
			t.Errorf("segmentPosition(%d) = %d, want %d", tt.offset, got, tt.want)
		}
	}
}

func TestPlaceholderAt(t *testing.T) {
	d := newDocument("file:///t.hurl", 1, `GET https://example.org?id={{user_id}}`, false)
	offset := len(`GET https://example.org?id={{user_`)
	inside, start, end := placeholderAt(d, offset)
	if !inside {
		t.Fatal("placeholderAt: not inside")
	}
	if got := d.text[start:end]; got != "user_id" {
		t.Errorf("name = %q, want user_id", got)
	}

	// Outside any placeholder.
	if inside, _, _ := placeholderAt(d, 3); inside {
		t.Error("placeholderAt: should not be inside on the method")
	}

	// Already closed before the cursor.
	closed := newDocument("file:///t.hurl", 1, `GET https://example.org?id={{x}}&y=1`, false)
	if inside, _, _ := placeholderAt(closed, len(closed.text)-1); inside {
		t.Error("placeholderAt: should not be inside after the closing }}")
	}
}

func TestLineTextStripsBOM(t *testing.T) {
	d := newDocument("file:///t.hurl", 1, "\xEF\xBB\xBFGET http://a/\nAccept: */*\n", false)
	if got := lineText(d, 0); got != "GET http://a/" {
		t.Errorf("lineText(0) = %q, want the method line without its BOM", got)
	}
	if m := precedingMarker(d, 1); m.kind != markerMethod {
		t.Errorf("precedingMarker after a BOM'd request line = %+v, want markerMethod", m)
	}
}

func TestPrecedingMarker(t *testing.T) {
	text := "GET https://example.org\nAccept: */*\n[Asserts]\nstatus == 200\n"
	d := newDocument("file:///t.hurl", 1, text, false)
	lastLine := int(d.lines.position(len(text)).Line)
	tests := []struct {
		line int
		kind markerKind
		sec  string
	}{
		{0, markerNone, ""},
		{1, markerMethod, ""},
		{2, markerMethod, ""},
		{3, markerSection, "Asserts"},
		{lastLine, markerSection, "Asserts"},
	}
	for _, tt := range tests {
		m := precedingMarker(d, tt.line)
		if m.kind != tt.kind || m.section != tt.sec {
			t.Errorf("precedingMarker(line %d) = %+v, want {%v %q}", tt.line, m, tt.kind, tt.sec)
		}
	}
}
