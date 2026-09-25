// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

func compress(t *testing.T, coding string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w interface {
		Write([]byte) (int, error)
		Close() error
	}
	switch coding {
	case "gzip":
		w = gzip.NewWriter(&buf)
	case "deflate":
		w = zlib.NewWriter(&buf)
	case "br":
		w = brotli.NewWriter(&buf)
	case "zstd":
		zw, err := zstd.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
		w = zw
	default:
		t.Fatalf("unknown coding %s", coding)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestHeaders(t *testing.T) {
	h := Headers{{"Set-Cookie", "a=1"}, {"Content-Type", "text/html"}, {"set-cookie", "b=2"}}
	if v, ok := h.Get("SET-COOKIE"); !ok || v != "a=1" {
		t.Errorf("Get = %q, %v", v, ok)
	}
	if _, ok := h.Get("X"); ok {
		t.Error("Get(X) found")
	}
	if got := h.Values("set-cookie"); !reflect.DeepEqual(got, []string{"a=1", "b=2"}) {
		t.Errorf("Values = %q", got)
	}
}

func TestDecodedBody(t *testing.T) {
	plain := []byte("Hello, world! Hello, world!")
	for _, c := range []string{"gzip", "deflate", "br", "zstd"} {
		r := &Response{Headers: Headers{{"Content-Encoding", c}}, Body: compress(t, c, plain)}
		got, err := r.DecodedBody()
		if err != nil || !bytes.Equal(got, plain) {
			t.Errorf("%s: %q, %v", c, got, err)
		}
	}
	// Codings are undone in reverse order of application.
	both := compress(t, "br", compress(t, "gzip", plain))
	r := &Response{Headers: Headers{{"Content-Encoding", "gzip, identity, br"}}, Body: both}
	if got, err := r.DecodedBody(); err != nil || !bytes.Equal(got, plain) {
		t.Errorf("gzip, br: %q, %v", got, err)
	}
	r = &Response{Body: plain}
	if got, err := r.DecodedBody(); err != nil || !bytes.Equal(got, plain) {
		t.Errorf("identity: %q, %v", got, err)
	}
}

func TestDecodedBodyErrors(t *testing.T) {
	tests := []struct{ coding, msg string }{
		{"compress", "Decompression error: compression compress is not supported"},
		{"gzip", "Decompression error: could not uncompress response with gzip"},
		{"deflate", "Decompression error: could not uncompress response with zlib"},
		{"br", "Decompression error: could not uncompress response with brotli"},
		{"zstd", "Decompression error: could not uncompress response with zstd"},
	}
	for _, tt := range tests {
		r := &Response{Headers: Headers{{"Content-Encoding", tt.coding}}, Body: []byte("not compressed")}
		_, err := r.DecodedBody()
		var be *BodyError
		if !errors.As(err, &be) || err.Error() != tt.msg {
			t.Errorf("%s: %v", tt.coding, err)
		}
		if _, err := r.Text(); err == nil {
			t.Errorf("%s: Text succeeded", tt.coding)
		}
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		contentType string
		body        []byte
		want        string
		err         string
	}{
		{"", []byte("café"), "café", ""},
		{"text/plain", []byte("\xef\xbb\xbfcafé"), "\uFEFFcafé", ""},
		{"text/plain; charset=ISO-8859-1", []byte("caf\xe9"), "café", ""},
		{"text/plain;charset=\"x\"; x=y", nil, "", "Invalid charset: the charset '\"x\"' is not valid"},
		{"text/plain; charset=l9", []byte("\xa4"), "€", ""},
		{"text/plain; charset=utf-16le", []byte{'h', 0, 'i', 0, 0x3d, 0xd8, 0x00, 0xde}, "hi😀", ""},
		{"text/plain; charset=utf-16be", []byte{0, 'h', 0xff, 0xfd}, "h�", ""},
		{"text/plain; charset=utf-16be", []byte{0, 'h', 0}, "", "Invalid decoding: could not decode response body with charset 'UTF-16BE'"},
		{"text/plain; charset=utf-16le", []byte{0x00, 0xdc}, "", "Invalid decoding: could not decode response body with charset 'UTF-16LE'"},
		{"text/plain; charset=utf-16le", []byte{0x3d, 0xd8, 'a', 0}, "", "Invalid decoding: could not decode response body with charset 'UTF-16LE'"},
		{"text/plain; charset=utf-8", []byte("caf\xe9"), "", "Invalid decoding: could not decode response body with charset 'UTF-8'"},
		{"text/plain; charset=shift_jis", []byte("\x82\xa0"), "あ", ""},
		{"text/plain; charset=shift_jis", []byte("\x82"), "", "Invalid decoding: could not decode response body with charset 'Shift_JIS'"},
		{"text/plain; charset=windows-1253", []byte("\xaa"), "", "Invalid decoding: could not decode response body with charset 'windows-1253'"},
		{"text/plain; charset=replacement", nil, "", ""},
		{"text/plain; charset=replacement", []byte("a"), "", "Invalid decoding: could not decode response body with charset 'replacement'"},
	}
	for _, tt := range tests {
		r := &Response{Body: tt.body}
		if tt.contentType != "" {
			r.Headers = Headers{{"Content-Type", tt.contentType}}
		}
		got, err := r.Text()
		if tt.err != "" {
			if err == nil || err.Error() != tt.err {
				t.Errorf("%s: error %v, want %s", tt.contentType, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: %q, %v; want %q", tt.contentType, got, err, tt.want)
		}
	}
}

func TestIsHTML(t *testing.T) {
	for ct, want := range map[string]bool{
		"text/html": true, " Text/HTML; charset=utf-8": true, "application/xhtml+xml": false, "": false,
	} {
		r := &Response{}
		if ct != "" {
			r.Headers = Headers{{"Content-Type", ct}}
		}
		if r.IsHTML() != want {
			t.Errorf("IsHTML(%q) = %v", ct, !want)
		}
	}
}

func TestParseSetCookie(t *testing.T) {
	tests := []struct {
		in   string
		want Cookie
		ok   bool
	}{
		{"LSID=DQAAAKEaem_vYg; Path=/accounts; Expires=Wed, 13 Jan 2021 22:23:01 GMT; Secure; HttpOnly", Cookie{
			Name: "LSID", Value: "DQAAAKEaem_vYg",
			Attributes: []CookieAttribute{
				{"Path", "/accounts", true}, {"Expires", "Wed, 13 Jan 2021 22:23:01 GMT", true},
				{"Secure", "", false}, {"HttpOnly", "", false},
			},
		}, true},
		{"a=b=c; X=1=2;", Cookie{Name: "a", Value: "b=c", Attributes: []CookieAttribute{{"X", "1", true}}}, true},
		{" n = v ;;  Flag ", Cookie{Name: " n ", Value: " v ", Attributes: []CookieAttribute{{"Flag", "", false}}}, true},
		{"novalue", Cookie{}, false},
	}
	for _, tt := range tests {
		got, ok := parseSetCookie(tt.in)
		if ok != tt.ok || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseSetCookie(%q) = %#v, %v", tt.in, got, ok)
		}
	}
}

func TestCookies(t *testing.T) {
	r := &Response{Headers: Headers{
		{"Set-Cookie", "a=1; Max-Age=x; max-age=60; Secure=yes; secure; SameSite=Lax; Domain"},
		{"Set-Cookie", "bad"},
		{"Set-Cookie", "a=2"},
		{"Set-Cookie", "b=3"},
	}}
	if got := r.Cookies(); len(got) != 3 {
		t.Fatalf("Cookies = %#v", got)
	}
	c, ok := r.Cookie("a")
	if !ok || c.Value != "1" {
		t.Fatalf("Cookie(a) = %#v", c)
	}
	if n, ok := c.MaxAge(); !ok || n != 60 {
		t.Errorf("MaxAge = %d, %v", n, ok)
	}
	if !c.Flag("SECURE") || c.Flag("HttpOnly") {
		t.Error("Flag")
	}
	if v, ok := c.Attr("samesite"); !ok || v != "Lax" {
		t.Errorf("SameSite = %q, %v", v, ok)
	}
	if _, ok := c.Attr("Domain"); ok {
		t.Error("value-less Domain reported a value")
	}
	if _, ok := c.Attr("Path"); ok {
		t.Error("Path found")
	}
	if _, ok := r.Cookie("z"); ok {
		t.Error("Cookie(z) found")
	}
	b, _ := r.Cookie("b")
	if _, ok := b.MaxAge(); ok {
		t.Error("MaxAge on cookie without attributes")
	}
}

func TestMaxDecodedBody(t *testing.T) {
	for _, c := range []string{"gzip", "br", "zstd", "deflate"} {
		body := compress(t, c, []byte(strings.Repeat("a", 1001)))
		r := &Response{Headers: Headers{{"Content-Encoding", c}}, Body: body, MaxDecodedBody: 1000}
		var be *BodyError
		if _, err := r.DecodedBody(); !errors.As(err, &be) || be.Kind != BodyTooLarge ||
			err.Error() != "Decompression error: decoded body is larger than 1000 bytes" {
			t.Errorf("%s: err = %v", c, err)
		}
		r.MaxDecodedBody = 1001
		if b, err := r.DecodedBody(); err != nil || len(b) != 1001 {
			t.Errorf("%s at limit: %d, %v", c, len(b), err)
		}
	}
}

// A tiny zstd frame declaring a huge window must not allocate it.
func TestZstdWindowLimit(t *testing.T) {
	// Frame header: magic, descriptor (single segment off, no checksum),
	// window descriptor for a 512 MiB window, then an empty last raw block.
	frame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0x00, 0x98, 0x01, 0x00, 0x00}
	r := &Response{Headers: Headers{{"Content-Encoding", "zstd"}}, Body: frame}
	var be *BodyError
	if _, err := r.DecodedBody(); !errors.As(err, &be) || be.Kind != DecompressFailed {
		t.Errorf("err = %v", err)
	}
}
