// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/settings"
)

const page = `<!doctype html>
<html lang="en">
  <head><script src="/theme-boot.js"></script></head>
</html>`

func themed(t *testing.T, next http.HandlerFunc, path string, a settings.Appearance) *httptest.ResponseRecorder {
	t.Helper()
	get := func() settings.Settings { s := settings.Defaults(); s.Appearance = a; return s }
	w := httptest.NewRecorder()
	themedIndex(next, get).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func serve(status int, contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("X-Kept", "yes")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestThemedIndex(t *testing.T) {
	dracula := settings.Appearance{Theme: "dracula", DayTheme: "light", NightTheme: "dark"}
	for _, path := range []string{"/", "/index.html"} {
		w := themed(t, serve(200, "text/html; charset=utf-8", page), path, dracula)
		want := `<html data-theme-pref="dracula" data-theme-day="light" data-theme-night="dark" lang="en">`
		if body := w.Body.String(); w.Code != 200 || !strings.Contains(body, want) {
			t.Errorf("%s: %d %s", path, w.Code, body)
		}
		if h := w.Header(); h.Get("Content-Length") != strconv.Itoa(w.Body.Len()) || h.Get("Cache-Control") != "no-store" || h.Get("X-Kept") != "yes" {
			t.Errorf("%s: headers %v", path, h)
		}
	}

	// Values are escaped (Set allows only theme ids; this is the guard).
	w := themed(t, serve(200, "text/html", page), "/", settings.Appearance{Theme: `x"><script>`, DayTheme: "light", NightTheme: "dark"})
	if strings.Contains(w.Body.String(), `x"><script>`) || !strings.Contains(w.Body.String(), `data-theme-pref="x&#34;&gt;&lt;script&gt;"`) {
		t.Errorf("not escaped: %s", w.Body)
	}

	// Everything else as it is.
	for name, c := range map[string]struct {
		next   http.HandlerFunc
		path   string
		status int
		body   string
	}{
		"asset":        {serve(200, "text/javascript", "<html>"), "/assets/x.js", 200, "<html>"},
		"no html tag":  {serve(200, "text/html", "<!doctype html><title>Sonde</title>"), "/", 200, "<!doctype html><title>Sonde</title>"},
		"not found":    {serve(404, "text/plain", "nope"), "/", 404, "nope"},
		"other prefix": {serve(200, "text/html", "<htmlx>"), "/", 200, "<htmlx>"},
	} {
		w := themed(t, c.next, c.path, dracula)
		if w.Code != c.status || w.Body.String() != c.body || w.Header().Get("Cache-Control") != "" || w.Header().Get("X-Kept") != "yes" {
			t.Errorf("%s: %d %q %v", name, w.Code, w.Body, w.Header())
		}
	}
}
