// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestLintMatchesReferenceFixtures checks Lint against the formatter
// suite's expected output for every export fixture.
func TestLintMatchesReferenceFixtures(t *testing.T) {
	inputs, err := filepath.Glob("../../testdata/conformance/hurlfmt/tests_export/*.hurl")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, in := range inputs {
		if strings.HasSuffix(in, ".lint.hurl") {
			continue
		}
		want, err := os.ReadFile(strings.TrimSuffix(in, ".hurl") + ".lint.hurl")
		if err != nil {
			continue
		}
		n++
		src, err := os.ReadFile(in)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Parse(in, src, DialectHurl)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got := string(Lint(f)); got != string(want) {
			t.Errorf("%s: Lint differs from %s.lint.hurl:\n got %q\nwant %q", filepath.Base(in), strings.TrimSuffix(filepath.Base(in), ".hurl"), got, want)
		}
	}
	if n < 19 {
		t.Fatalf("checked %d fixtures, want 19", n)
	}
}

func TestLintRules(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"reorder and comments move with their section",
			"GET http://x\n[Cookies]\na: b\n# about options\n[Options]\ndelay: 2\nHTTP 200\n[Asserts]\nstatus == 200\n[Captures]\nc: status\n",
			"GET http://x\n# about options\n[Options]\ndelay: 2ms\n[Cookies]\na: b\nHTTP 200\n[Captures]\nc: status\n[Asserts]\nstatus == 200\n"},
		{"indented comment line, trailing comment kept aligned",
			"   # top   \nGET http://x   # trailing\n",
			"# top\nGET http://x   # trailing\n"},
		{"crlf kept, final newline added",
			"GET http://x\r\nHTTP 200",
			"GET http://x\r\nHTTP 200\n"},
		{"units kept, other durations untouched",
			"GET http://x\n[Options]\nretry-interval: 1s\nmax-time: 30\nconnect-timeout: 5\nretry: 3\n",
			"GET http://x\n[Options]\nretry-interval: 1s\nmax-time: 30ms\nconnect-timeout: 5ms\nretry: 3\n"},
		{"empty value", "GET http://x\nA:    \n", "GET http://x\nA:\n"},
		{"empty cookie and option keep the space",
			"GET http://x\n[Options]\nuser:\n[Cookies]\nc:\n", "GET http://x\n[Options]\nuser: \n[Cookies]\nc: \n"},
		{"bom dropped", "\uFEFFGET http://x\n", "GET http://x\n"},
	} {
		f, err := Parse("t.hurl", []byte(tc.src), DialectHurl)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := string(Lint(f)); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}

func TestLintSondeSectionsLast(t *testing.T) {
	src := "POST http://x/svc/M\n[SondeGrpc]\nproto: a.proto\n[Options]\ndelay: 1\n"
	f, err := Parse("t.sonde", []byte(src), DialectSonde)
	if err != nil {
		t.Fatal(err)
	}
	want := "POST http://x/svc/M\n[Options]\ndelay: 1ms\n[SondeGrpc]\nproto: a.proto\n"
	if got := string(Lint(f)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestLintCorpus: over every valid conformance file, Lint leaves the AST
// unchanged (Lint's own reordering and units aside), its output parses,
// linting it again changes nothing, and the input AST is not modified.
func TestLintCorpus(t *testing.T) {
	files := append(conformanceFiles(t), localSyntaxFiles(t)...)
	for _, path := range files {
		if expectedFailures[conformanceName(path)] != 0 {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Parse(path, src, DialectFor(path))
		if err != nil {
			continue // covered by the round-trip tests
		}
		before := string(Print(f))
		out := Lint(f)
		if string(Print(f)) != before {
			t.Errorf("%s: Lint modified its input", path)
		}
		f2, err := Parse(path, out, DialectFor(path))
		if err != nil {
			t.Errorf("%s: Lint output does not parse: %v", path, err)
			continue
		}
		if again := Lint(f2); string(again) != string(out) {
			t.Errorf("%s: Lint is not idempotent:\nfirst  %q\nsecond %q", path, out, again)
		}
		if got, want := sectionsBySide(f2, true), sectionsBySide(withDefaultUnits(f), false); got != want {
			t.Errorf("%s: Lint changed the sections:\n got %q\nwant %q", path, got, want)
		}
		if got, want := semanticRequestLines(f2), semanticRequestLines(f); got != want {
			t.Errorf("%s: Lint changed a request or response line:\n got %q\nwant %q", path, got, want)
		}
	}
}

// sectionsBySide renders each entry's request and response sections
// canonically, without trivia, as a sorted multiset per side, so the
// comparison ignores order but not content. An empty [BasicAuth] is left
// out (Lint drops it). With ordered, it also checks the sections are in
// canonical order, marking the side otherwise.
func sectionsBySide(f *File, ordered bool) string {
	stripTrivia(reflect.ValueOf(f))
	var b strings.Builder
	side := func(sections []*Section, order map[SectionKind]int) {
		var items []string
		for i, s := range sections {
			if ordered && i > 0 && order[sections[i-1].Kind] > order[s.Kind] {
				b.WriteString("OUT OF ORDER;")
			}
			if s.Kind == SectionBasicAuth && len(s.KeyValues) == 0 {
				continue
			}
			p := printer{canonical: true}
			p.sections([]*Section{s})
			items = append(items, strings.ReplaceAll(p.String(), "\r\n", "\n"))
		}
		sort.Strings(items)
		b.WriteString(strings.Join(items, "|") + ";")
	}
	for _, e := range f.Entries {
		side(e.Request.Sections, requestSectionOrder)
		if e.Response != nil {
			side(e.Response.Sections, responseSectionOrder)
		}
	}
	return b.String()
}

// semanticRequestLines renders every request and response without its
// sections or trivia: method, URL, headers, body, version and status.
func semanticRequestLines(f *File) string {
	stripTrivia(reflect.ValueOf(f))
	var b strings.Builder
	for _, e := range f.Entries {
		r := *e.Request
		r.Sections = nil
		p := printer{canonical: true}
		p.request(&r)
		if e.Response != nil {
			resp := *e.Response
			resp.Sections = nil
			p.response(&resp)
		}
		b.WriteString(strings.ReplaceAll(p.String(), "\r\n", "\n"))
	}
	return b.String()
}

// withDefaultUnits adds `ms` to unitless connect-timeout, delay, max-time
// and retry-interval options of f, independently of Lint's own code.
func withDefaultUnits(f *File) *File {
	for _, e := range f.Entries {
		for _, s := range e.Request.Sections {
			for _, o := range s.Options {
				switch o.Name {
				case "connect-timeout", "delay", "max-time", "retry-interval":
					if d, ok := o.Value.(*Duration); ok && d.Unit == "" {
						d.Unit = "ms"
					}
				}
			}
		}
	}
	return f
}

// stripTrivia drops every comment and blank line reachable from v.
func stripTrivia(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			if lt, ok := v.Interface().(*LineTerminator); ok {
				lt.Comment, lt.Space0 = nil, Whitespace{}
				return
			}
			stripTrivia(v.Elem())
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if fv := v.Field(i); v.Type().Field(i).Name == "LineTerminators" {
				fv.Set(reflect.Zero(fv.Type()))
			} else if fv.CanSet() {
				stripTrivia(fv)
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			stripTrivia(v.Index(i))
		}
	}
}
