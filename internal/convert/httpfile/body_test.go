// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

func renderBody(t *testing.T, spec *syntax.BodySpec) string {
	t.Helper()
	f, err := syntax.BuildFile([]syntax.EntrySpec{{Method: "POST", URL: syntax.PlainText("https://x"), Body: spec}}, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	out := string(syntax.Format(f))
	_, body, _ := strings.Cut(out, "\n")
	return strings.TrimRight(body, "\n")
}

// assertLivePlaceholder fails unless got holds name's placeholder live
// (never escaped, e.g. into "\u{7B}...").
func assertLivePlaceholder(t *testing.T, got, name string) {
	t.Helper()
	if !strings.Contains(got, "{{"+name+"}}") {
		t.Errorf("expected a live {{%s}} placeholder in %q", name, got)
	}
	if strings.Contains(got, `\u{`) {
		t.Errorf("placeholder was escaped instead of kept live: %q", got)
	}
}

func TestBuildBodyJSON(t *testing.T) {
	body, form, mp, warns := buildBody(&rawBody{kind: bodyText, text: `{"name": "{{n}}", "count": 3}`}, "application/json")
	if form != nil || mp != nil {
		t.Fatalf("form/mp should be nil: %v %v", form, mp)
	}
	if len(warns) != 0 {
		t.Errorf("warns = %v", warns)
	}
	if got := renderBody(t, body); got != `{"name": "{{n}}", "count": 3}` {
		t.Errorf("body = %q", got)
	}
}

// TestBuildBodyJSONDuplicateKeysKept checks M1: convert.JSONBody keeps
// duplicate keys (a client sending duplicate JSON keys is unusual but
// legal), rather than the old map[string]any-based rebuild that silently
// collapsed them.
func TestBuildBodyJSONDuplicateKeysKept(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyText, text: `{"a": 1, "a": 2}`}, "application/json")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if got := renderBody(t, body); got != `{"a": 1, "a": 2}` {
		t.Errorf("body = %q", got)
	}
}

// TestBuildBodyJSONPlaceholderInKeyFallsBack checks M1: a placeholder in a
// JSON object key (which the old map[string]any-based rebuild sent as
// literal text) falls back to templatedBody, keeping it live instead.
func TestBuildBodyJSONPlaceholderInKeyFallsBack(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyText, text: `{"{{field}}": "v"}`}, "application/json")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	assertLivePlaceholder(t, renderBody(t, body), "field")
}

// TestBuildBodyJSONInvalidKeepsPlaceholderLive checks that a placeholder
// used outside a JSON string (making the text invalid JSON on its own)
// still comes out live, via syntax.TextBody, not escaped away.
func TestBuildBodyJSONInvalidKeepsPlaceholderLive(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyText, text: `{"count": {{n}}}`}, "application/json")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	assertLivePlaceholder(t, renderBody(t, body), "n")
}

func TestBuildBodyForm(t *testing.T) {
	_, form, _, warns := buildBody(&rawBody{kind: bodyText, text: "a=1&b=hello%20world&c={{v}}"}, "application/x-www-form-urlencoded")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if len(form) != 3 {
		t.Fatalf("form = %+v", form)
	}
}

func TestBuildBodyXMLNoPlaceholder(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyText, text: "<a>hi</a>"}, "application/xml")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if got := renderBody(t, body); !strings.HasPrefix(got, "```xml") {
		t.Errorf("body = %q", got)
	}
}

// TestBuildBodyXMLPlaceholderStaysLive checks that syntax.TextBody (unlike
// the old RawTextBody-based fallback) keeps a placeholder inside an XML body
// live rather than escaping it into permanently literal text.
func TestBuildBodyXMLPlaceholderStaysLive(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyText, text: "<a>{{x}}</a>"}, "application/xml")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	assertLivePlaceholder(t, renderBody(t, body), "x")
}

func TestBuildBodyRawKeepsPlaceholderLive(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyText, text: "plain {{x}} text"}, "text/plain")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	assertLivePlaceholder(t, renderBody(t, body), "x")
}

// TestBuildBodyFileRefLiteralNoWarning checks "< path": REST Client and
// JetBrains send the file as is, matching Sonde's file body exactly, so no
// warning is expected.
func TestBuildBodyFileRefLiteralNoWarning(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyFileRef, path: "./f.json"}, "")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if got := renderBody(t, body); got != "file,./f.json;" {
		t.Errorf("body = %q", got)
	}
}

// TestBuildBodyFileRefSubstitutedWarns checks "<@ path": REST Client
// substitutes that file's own {{variables}} before sending, which Sonde's
// file body cannot do, so this warns.
func TestBuildBodyFileRefSubstitutedWarns(t *testing.T) {
	body, _, _, warns := buildBody(&rawBody{kind: bodyFileRefSubstituted, path: "./f.bin"}, "")
	if len(warns) != 1 || warns[0].Kind != convert.WarnUnsupportedBody {
		t.Fatalf("warns = %v", warns)
	}
	if got := renderBody(t, body); got != "file,./f.bin;" {
		t.Errorf("body = %q", got)
	}
}

func TestBuildMultipartSimple(t *testing.T) {
	// buildMultipart always runs on text already normalized to "\n" line
	// endings by parseDocument, so the fixture uses "\n" too.
	text := "--B\nContent-Disposition: form-data; name=\"a\"\n\nhello\n--B\n" +
		"Content-Disposition: form-data; name=\"f\"; filename=\"x.png\"\nContent-Type: image/png\n\n< ./x.png\n--B--"
	fields, warns, ok := buildMultipart(text, "boundary=B")
	if !ok {
		t.Fatal("expected ok")
	}
	if len(warns) != 0 {
		t.Errorf("warns = %v", warns)
	}
	if len(fields) != 2 || fields[1].File == nil {
		t.Fatalf("fields = %+v", fields)
	}
}

// TestBuildMultipartFileUsesBodyPathNotFilenameParam checks that the local
// path a file part reads comes from its own "< path"/"<@ path" content, not
// the Content-Disposition "filename=" parameter, which may legitimately
// differ (it's only what gets reported to the server).
func TestBuildMultipartFileUsesBodyPathNotFilenameParam(t *testing.T) {
	text := "--B\nContent-Disposition: form-data; name=\"f\"; filename=\"reported.png\"\n\n< ./local/actual.png\n--B--"
	fields, _, ok := buildMultipart(text, "boundary=B")
	if !ok || len(fields) != 1 || fields[0].File == nil {
		t.Fatalf("fields = %+v, ok=%v", fields, ok)
	}
	f, err := syntax.BuildFile([]syntax.EntrySpec{{Method: "POST", URL: syntax.PlainText("https://x"), Multipart: fields}}, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	out := string(syntax.Format(f))
	if !strings.Contains(out, "./local/actual.png") {
		t.Errorf("expected the body's own path, not the filename= parameter:\n%s", out)
	}
	if strings.Contains(out, "reported.png") {
		t.Errorf("filename= parameter should not leak into the file path:\n%s", out)
	}
}

func TestBuildMultipartFileSubstitutedWarns(t *testing.T) {
	text := "--B\nContent-Disposition: form-data; name=\"f\"; filename=\"x.png\"\n\n<@ ./x.png\n--B--"
	_, warns, ok := buildMultipart(text, "boundary=B")
	if !ok || len(warns) != 1 || warns[0].Kind != convert.WarnUnsupportedBody {
		t.Fatalf("ok=%v warns=%v", ok, warns)
	}
}

func TestBuildMultipartFileInlineContentFallsBack(t *testing.T) {
	text := "--B\nContent-Disposition: form-data; name=\"f\"; filename=\"x.png\"\nContent-Type: image/png\n\nnot-a-file-reference\n--B--"
	_, _, ok := buildMultipart(text, "boundary=B")
	if ok {
		t.Error("expected ok=false: inline content has no [Multipart] file representation")
	}
}

func TestBuildMultipartFallback(t *testing.T) {
	_, _, ok := buildMultipart("not really multipart", "")
	if ok {
		t.Error("expected ok=false without a boundary")
	}
	_, _, ok = buildMultipart("garbage", "boundary=B")
	if ok {
		t.Error("expected ok=false without a valid part")
	}
}

func TestBuildBodyMultipartFallsBackWhenNotSimple(t *testing.T) {
	body, form, mp, warns := buildBody(&rawBody{kind: bodyText, text: "garbage"}, "multipart/form-data")
	if body == nil || form != nil || mp != nil {
		t.Fatalf("expected raw fallback, got body=%v form=%v mp=%v", body, form, mp)
	}
	_ = warns
}
