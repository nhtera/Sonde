// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

func respWith(contentType string, body []byte) *exchange.Response {
	r := &exchange.Response{Body: body}
	if contentType != "" {
		r.Headers = exchange.Headers{{Name: "Content-Type", Value: contentType}}
	}
	return r
}

func TestHTMLBodyPreview_JSONPrettyPrinted(t *testing.T) {
	got := htmlBodyPreview(respWith("application/json", []byte(`{"a":1,"b":[2,3]}`)), redactTestSecret)
	want := "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestHTMLBodyPreview_JSONContentTypeButNotJSON(t *testing.T) {
	// Content-Type lies; the body is shown as text instead of erroring out.
	got := htmlBodyPreview(respWith("application/json", []byte("not json")), redactTestSecret)
	if got != "not json" {
		t.Errorf("got %q, want %q", got, "not json")
	}
}

func TestHTMLBodyPreview_Text(t *testing.T) {
	got := htmlBodyPreview(respWith("text/plain", []byte("hello, "+testSecret)), redactTestSecret)
	if got != "hello, ***" {
		t.Errorf("got %q, want %q", got, "hello, ***")
	}
}

func TestHTMLBodyPreview_Binary(t *testing.T) {
	body := []byte{0xff, 0xfe, 0x00, 0x01, 0x02}
	got := htmlBodyPreview(respWith("application/octet-stream", body), redactTestSecret)
	if got != "binary, 5 bytes" {
		t.Errorf("got %q, want %q", got, "binary, 5 bytes")
	}
}

func TestHTMLBodyPreview_Empty(t *testing.T) {
	if got := htmlBodyPreview(respWith("text/plain", nil), redactTestSecret); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := htmlBodyPreview(nil, redactTestSecret); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestHTMLBodyPreview_Truncated(t *testing.T) {
	body := strings.Repeat("x", htmlBodyPreviewCap+100)
	got := htmlBodyPreview(respWith("text/plain", []byte(body)), redactTestSecret)
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected a truncation note, got a %d-byte preview", len(got))
	}
	if strings.Count(got, "x") > htmlBodyPreviewCap {
		t.Errorf("preview kept more than the cap: %d 'x' characters", strings.Count(got, "x"))
	}
}

// TestHTMLBodyPreview_RedactsAcrossTruncationBoundary is the case
// truncate-then-redact would get wrong: a secret straddling the cap must
// come out fully masked, never partially visible.
func TestHTMLBodyPreview_RedactsAcrossTruncationBoundary(t *testing.T) {
	body := strings.Repeat("x", htmlBodyPreviewCap-4) + testSecret + strings.Repeat("y", 200)
	got := htmlBodyPreview(respWith("text/plain", []byte(body)), redactTestSecret)
	if strings.Contains(got, testSecret) {
		t.Errorf("secret survived truncation unredacted:\n%s", got)
	}
}

func TestHTMLBodyPreview_TooLargeToProcess(t *testing.T) {
	body := make([]byte, htmlBodyMaxProcess+1)
	got := htmlBodyPreview(respWith("text/plain", body), redactTestSecret)
	if !strings.Contains(got, "too large") {
		t.Errorf("got %q, want a size summary", got)
	}
}
