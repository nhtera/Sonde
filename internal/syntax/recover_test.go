// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAll(t *testing.T) {
	src := "GET http://a/\nHTTP 200\n\n" +
		"GET http://b/\n[Options]\nnope: 1\n\n" +
		"POST http://c/\nX-Ok: 1\n\n" +
		"GET http://d/\nHTTP 2000x\n\n" +
		"PUT http://e/\nHTTP 200\n[Asserts]\nstatus ==\n\n" +
		"DELETE http://f/\n"
	f, errs := ParseAll("x.hurl", []byte(src), DialectHurl, 50)
	if len(errs) != 3 {
		t.Fatalf("errs = %v, want 3", errs)
	}
	for i, line := range []int{6, 12, 17} {
		if errs[i].Pos.Line != line {
			t.Errorf("errs[%d] at line %d, want %d (%v)", i, errs[i].Pos.Line, line, errs[i])
		}
	}
	var urls []string
	for _, e := range f.Entries {
		urls = append(urls, e.Request.URL.Elements[0].(*TemplateString).Value)
	}
	if got := strings.Join(urls, " "); got != "http://a/ http://c/ http://f/" {
		t.Errorf("entries = %s", got)
	}

	_, first := Parse("x.hurl", []byte(src), DialectHurl)
	var pe *Error
	if !errors.As(first, &pe) || *pe != *errs[0] {
		t.Errorf("first error %v differs from Parse's %v", errs[0], first)
	}
}

func TestParseAllLimitAndClean(t *testing.T) {
	src := strings.Repeat("GET http://x/\nHTTP 2x\n", 80)
	if _, errs := ParseAll("x.hurl", []byte(src), DialectHurl, 50); len(errs) != 50 {
		t.Errorf("len(errs) = %d, want 50", len(errs))
	}
	clean := utf8BOM + "GET http://x/\n# end\n"
	f, errs := ParseAll("x.hurl", []byte(clean), DialectHurl, 50)
	if len(errs) != 0 || !f.BOM || len(f.Entries) != 1 || string(Print(f)) != clean {
		t.Errorf("clean parse: errs=%v entries=%d", errs, len(f.Entries))
	}
	if _, errs := ParseAll("x.hurl", []byte{0xff}, DialectHurl, 50); len(errs) != 1 || errs[0].Kind != ErrInvalidUTF8 {
		t.Errorf("invalid utf-8: %v", errs)
	}
	// A broken entry after which no method line follows ends the scan.
	if _, errs := ParseAll("x.hurl", []byte("GET http://x/\nHTTP 2x\nnot a request\n"), DialectHurl, 50); len(errs) != 1 {
		t.Errorf("tail: %v", errs)
	}
}

func FuzzParseAll(f *testing.F) {
	f.Add([]byte("GET http://a/\nHTTP 2x\nPOST http://b/\n"))
	f.Add([]byte("GET http://a/\n```\nGET x\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		file, errs := ParseAll("x.hurl", src, DialectHurl, 50)
		if len(errs) > 50 {
			t.Fatalf("%d errors", len(errs))
		}
		for i := 1; i < len(errs); i++ {
			if errs[i].Pos.Offset <= errs[i-1].Pos.Offset {
				t.Fatalf("errors out of order: %v", errs)
			}
		}
		if len(errs) == 0 {
			if got := Print(file); string(got) != string(src) {
				t.Fatalf("roundtrip mismatch")
			}
		}
	})
}
