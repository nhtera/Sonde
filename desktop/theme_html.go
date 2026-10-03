// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/settings"
)

// themedIndex serves the page with the theme settings on its <html> tag
// (data-theme-pref, -day and -night): public/theme-boot.js picks the
// theme from them before the first paint, before the settings load. The
// page comes from next (the built files, or the dev server's page under
// task dev); anything but an HTML 200 passes through as it is.
func themedIndex(next http.Handler, get func() settings.Settings) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "", "/", "/index.html":
		default:
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		b := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
		next.ServeHTTP(b, r)
		body := b.body.Bytes()
		if b.status == http.StatusOK && strings.HasPrefix(b.header.Get("Content-Type"), "text/html") && b.header.Get("Content-Encoding") == "" {
			if themed, ok := withTheme(body, get().Appearance); ok {
				body = themed
				b.header.Set("Content-Length", strconv.Itoa(len(body)))
				// The attributes change with the settings.
				b.header.Set("Cache-Control", "no-store")
				b.header.Del("ETag")
				b.header.Del("Last-Modified")
			}
		}
		for k, v := range b.header {
			w.Header()[k] = v
		}
		w.WriteHeader(b.status)
		_, _ = w.Write(body)
	})
}

// withTheme adds the theme attributes to the page's first <html> tag;
// false when it has none.
func withTheme(page []byte, a settings.Appearance) ([]byte, bool) {
	i := htmlTag(page)
	if i < 0 {
		return nil, false
	}
	attrs := ` data-theme-pref="` + html.EscapeString(a.Theme) +
		`" data-theme-day="` + html.EscapeString(a.DayTheme) +
		`" data-theme-night="` + html.EscapeString(a.NightTheme) + `"`
	at := i + len("<html")
	out := make([]byte, 0, len(page)+len(attrs))
	out = append(out, page[:at]...)
	out = append(out, attrs...)
	return append(out, page[at:]...), true
}

// htmlTag is the index of the first "<html" that opens the tag (followed
// by a space or ">"), -1 when there is none.
func htmlTag(page []byte) int {
	lower := bytes.ToLower(page)
	for off := 0; ; {
		i := bytes.Index(lower[off:], []byte("<html"))
		if i < 0 {
			return -1
		}
		i += off
		if end := i + len("<html"); end < len(lower) && strings.IndexByte(" \t\r\n>", lower[end]) >= 0 {
			return i
		}
		off = i + 1
	}
}

// bufferedResponse keeps a response to change it before it is sent.
type bufferedResponse struct {
	header http.Header
	status int
	wrote  bool
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if !b.wrote {
		b.status, b.wrote = status, true
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	b.wrote = true
	return b.body.Write(p)
}
