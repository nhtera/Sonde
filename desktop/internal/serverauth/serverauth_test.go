// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package serverauth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const port = 7780

var origin = "http://127.0.0.1:7780"

// ok is the wrapped handler: it answers 200 "app".
var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "app") })

type req struct {
	method, path, host string
	headers            map[string]string
	cookie             bool
	token              bool
}

func (g *Guard) do(t *testing.T, q req) *httptest.ResponseRecorder {
	t.Helper()
	if q.method == "" {
		q.method = http.MethodGet
	}
	r := httptest.NewRequest(q.method, "http://127.0.0.1:7780"+q.path, nil)
	r.Host = "127.0.0.1:7780"
	if q.host != "" {
		r.Host = q.host
	}
	for k, v := range q.headers {
		r.Header.Set(k, v)
	}
	if q.cookie {
		r.AddCookie(&http.Cookie{Name: g.CookieName(), Value: g.cookie}) //nolint:gosec // a request cookie
	}
	if q.token {
		r.Header.Set(TokenHeader, g.token)
	}
	w := httptest.NewRecorder()
	g.Middleware(ok).ServeHTTP(w, r)
	return w
}

func newGuard(t *testing.T) *Guard {
	t.Helper()
	g, err := New(port)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func post(path string, cookie, token bool) req {
	return req{method: http.MethodPost, path: path, headers: map[string]string{"Origin": origin}, cookie: cookie, token: token}
}

func TestRuntimeCallNeedsTokenAndCookie(t *testing.T) {
	g := newGuard(t)
	for name, tc := range map[string]struct {
		q    req
		want int
	}{
		"no credentials":        {post("/wails/runtime", false, false), http.StatusUnauthorized},
		"cookie only":           {post("/wails/runtime", true, false), http.StatusUnauthorized},
		"token only":            {post("/wails/runtime", false, true), http.StatusUnauthorized},
		"both":                  {post("/wails/runtime", true, true), http.StatusOK},
		"app endpoint, cookie":  {post("/_sonde/events", true, false), http.StatusUnauthorized},
		"app endpoint, both":    {post("/_sonde/events", true, true), http.StatusOK},
		"stream poll, cookie":   {post("/wails/stream/poll", true, false), http.StatusUnauthorized},
		"event payload, cookie": {req{path: "/wails/eventpayload/1", cookie: true}, http.StatusUnauthorized},
		"bad token": {req{method: http.MethodPost, path: "/wails/runtime", cookie: true,
			headers: map[string]string{"Origin": origin, TokenHeader: g.token + "x"}}, http.StatusUnauthorized},
	} {
		t.Run(name, func(t *testing.T) {
			if got := g.do(t, tc.q).Code; got != tc.want {
				t.Errorf("status %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDuplicateTokenHeaderRefused(t *testing.T) {
	g := newGuard(t)
	r := httptest.NewRequest(http.MethodPost, "/wails/runtime", nil)
	r.Host = "127.0.0.1:7780"
	r.Header.Set("Origin", origin)
	r.Header.Add(TokenHeader, "wrong")
	r.Header.Add(TokenHeader, g.token)
	r.AddCookie(&http.Cookie{Name: g.CookieName(), Value: g.cookie}) //nolint:gosec // a request cookie
	w := httptest.NewRecorder()
	g.Middleware(ok).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", w.Code)
	}
}

func TestAssetsAndBodiesNeedCookie(t *testing.T) {
	g := newGuard(t)
	for _, path := range []string{"/", "/assets/index.js", "/wails/runtime.js", BodyPrefix + "abc"} {
		if got := g.do(t, req{path: path}).Code; got != http.StatusUnauthorized {
			t.Errorf("%s without cookie: %d, want 401", path, got)
		}
		if got := g.do(t, req{path: path, cookie: true}).Code; got != http.StatusOK {
			t.Errorf("%s with cookie: %d, want 200", path, got)
		}
	}
	if got := g.do(t, req{path: "/", headers: map[string]string{"Cookie": g.CookieName() + "=forged"}}).Code; got != http.StatusUnauthorized {
		t.Errorf("forged cookie: %d, want 401", got)
	}
}

func TestHostChecked(t *testing.T) {
	g := newGuard(t)
	for _, host := range []string{"evil.example:7780", "evil.example", "127.0.0.1:7781", "127.0.0.1", "[::1]:7780", "localhost.evil.example:7780"} {
		if got := g.do(t, req{path: "/", host: host, cookie: true}).Code; got != http.StatusForbidden {
			t.Errorf("Host %q: %d, want 403", host, got)
		}
	}
	w := g.do(t, req{path: "/", host: "localhost:7780", cookie: true,
		headers: map[string]string{"Origin": "http://localhost:7780"}})
	if w.Code != http.StatusOK {
		t.Errorf("localhost: %d, want 200", w.Code)
	}
}

func TestOriginChecked(t *testing.T) {
	g := newGuard(t)
	cases := map[string]struct {
		q    req
		want int
	}{
		"POST without Origin": {req{method: http.MethodPost, path: "/wails/runtime", cookie: true, token: true}, http.StatusForbidden},
		"PUT without Origin":  {req{method: http.MethodPut, path: "/x", cookie: true, token: true}, http.StatusForbidden},
		"page on another port": {req{method: http.MethodPost, path: "/wails/runtime", cookie: true, token: true,
			headers: map[string]string{"Origin": "http://127.0.0.1:3000"}}, http.StatusForbidden},
		"null origin": {req{method: http.MethodPost, path: "/wails/runtime", cookie: true, token: true,
			headers: map[string]string{"Origin": "null"}}, http.StatusForbidden},
		"https origin": {req{method: http.MethodPost, path: "/wails/runtime", cookie: true, token: true,
			headers: map[string]string{"Origin": "https://127.0.0.1:7780"}}, http.StatusForbidden},
		"cross-port GET of a body": {req{path: BodyPrefix + "abc", cookie: true,
			headers: map[string]string{"Sec-Fetch-Site": "same-site"}}, http.StatusForbidden},
		"cross-site GET": {req{path: "/", cookie: true,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"}}, http.StatusForbidden},
		"preflight": {req{method: http.MethodOptions, path: "/wails/runtime",
			headers: map[string]string{"Origin": "http://127.0.0.1:3000", "Access-Control-Request-Method": "POST"}}, http.StatusForbidden},
		"same origin fetch": {req{method: http.MethodPost, path: "/wails/runtime", cookie: true, token: true,
			headers: map[string]string{"Origin": origin, "Sec-Fetch-Site": "same-origin"}}, http.StatusOK},
		"navigation": {req{path: "/", cookie: true, headers: map[string]string{"Sec-Fetch-Site": "none"}}, http.StatusOK},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := g.do(t, tc.q)
			if w.Code != tc.want {
				t.Errorf("status %d, want %d", w.Code, tc.want)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Error("CORS header set")
			}
		})
	}
}

func TestFramingDenied(t *testing.T) {
	g := newGuard(t)
	for _, q := range []req{{path: "/", cookie: true}, {path: "/"}, {path: "/", host: "evil.example"}} {
		w := g.do(t, q)
		if got := w.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
			t.Errorf("CSP %q", got)
		}
		if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("X-Frame-Options %q", got)
		}
		if got := w.Header().Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
			t.Errorf("CORP %q", got)
		}
	}
}

func TestNonceExchange(t *testing.T) {
	g := newGuard(t)
	done := 0
	n, err := g.Nonce(func() { done++ })
	if err != nil {
		t.Fatal(err)
	}
	// Opened from the launch file: a cross-site navigation.
	w := g.do(t, req{path: "/?nonce=" + url.QueryEscape(n), headers: map[string]string{"Sec-Fetch-Site": "cross-site"}})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `content="0;url=/"`) {
		t.Fatalf("exchange: %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("exchange page is cacheable or frameable")
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Name != g.CookieName() || c[0].Value != g.cookie || !c[0].HttpOnly ||
		c[0].SameSite != http.SameSiteStrictMode || c[0].Path != "/" {
		t.Fatalf("cookie %+v", c)
	}
	if done != 1 {
		t.Errorf("done ran %d times, want 1", done)
	}
	if w := g.do(t, req{path: "/?nonce=" + url.QueryEscape(n)}); w.Code != http.StatusForbidden {
		t.Errorf("second use: %d, want 403", w.Code)
	}
	if w := g.do(t, req{path: "/?nonce=" + url.QueryEscape(n), host: "evil.example:7780"}); w.Code != http.StatusForbidden {
		t.Errorf("rebound host: %d, want 403", w.Code)
	}
	if w := g.do(t, req{path: "/?nonce=guess"}); w.Code != http.StatusForbidden {
		t.Errorf("unknown nonce: %d, want 403", w.Code)
	}
	if w := g.do(t, req{method: http.MethodPost, path: "/?nonce=x", headers: map[string]string{"Origin": origin}}); w.Code != http.StatusForbidden {
		t.Errorf("POST exchange: %d, want 403", w.Code)
	}
	if done != 1 {
		t.Errorf("done ran %d times, want 1", done)
	}
}

func TestNonceExpires(t *testing.T) {
	g := newGuard(t)
	now := time.Now()
	g.now = func() time.Time { return now }
	n, err := g.Nonce(nil)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(NonceTTL + time.Second) // the timer has not fired yet
	if w := g.do(t, req{path: "/?nonce=" + url.QueryEscape(n)}); w.Code != http.StatusForbidden {
		t.Errorf("expired nonce: %d, want 403", w.Code)
	}
	g.mu.Lock()
	left := len(g.nonces)
	g.mu.Unlock()
	if left != 0 {
		t.Errorf("%d nonces left", left)
	}
}

func TestNonceTimerRemoves(t *testing.T) {
	g := newGuard(t)
	fired := make(chan struct{})
	n, err := g.Nonce(func() { close(fired) })
	if err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	g.nonces[n].timer.Reset(time.Millisecond)
	g.mu.Unlock()
	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("expiry did not run done")
	}
	if w := g.do(t, req{path: "/?nonce=" + url.QueryEscape(n)}); w.Code != http.StatusForbidden {
		t.Errorf("after expiry: %d, want 403", w.Code)
	}
}

func TestSessionReturnsToken(t *testing.T) {
	g := newGuard(t)
	if w := g.do(t, req{path: SessionPath}); w.Code != http.StatusUnauthorized {
		t.Errorf("without cookie: %d, want 401", w.Code)
	}
	w := g.do(t, req{path: SessionPath, cookie: true, headers: map[string]string{"Origin": origin}})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var body struct{ Token string }
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body.Token != g.token {
		t.Errorf("token mismatch (err %v)", err)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("session response is cacheable")
	}
}

func TestSecretsNeverInRefusals(t *testing.T) {
	g := newGuard(t)
	n, err := g.Nonce(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []req{
		post("/wails/runtime", true, false),
		{path: "/", host: "evil.example"},
		{path: "/?nonce=bad"},
		{path: SessionPath},
		{method: http.MethodPost, path: "/wails/runtime", cookie: true, token: true},
	} {
		body := g.do(t, q).Body.String()
		for _, secret := range []string{g.token, g.cookie, n} {
			if strings.Contains(body, secret) {
				t.Errorf("%s %s: refusal leaks a secret", q.method, q.path)
			}
		}
	}
}

func TestSecretsAreRandom(t *testing.T) {
	a, b := newGuard(t), newGuard(t)
	if a.token == b.token || a.cookie == b.cookie || a.token == a.cookie || len(a.token) < 43 {
		t.Error("token and cookie must be fresh 256-bit values")
	}
}

func TestFixedMode(t *testing.T) {
	g := NewFixed(port, "harness-token")
	if got := g.do(t, req{path: "/"}).Code; got != http.StatusOK {
		t.Errorf("asset without cookie: %d, want 200", got)
	}
	if got := g.do(t, post("/wails/runtime", false, false)).Code; got != http.StatusUnauthorized {
		t.Errorf("call without token: %d, want 401", got)
	}
	if got := g.do(t, post("/wails/runtime", false, true)).Code; got != http.StatusOK {
		t.Errorf("call with token: %d, want 200", got)
	}
	if got := g.do(t, req{path: "/", host: "evil.example:7780"}).Code; got != http.StatusForbidden {
		t.Errorf("bad Host: %d, want 403", got)
	}
	if got := g.do(t, req{method: http.MethodPost, path: "/wails/runtime", token: true}).Code; got != http.StatusForbidden {
		t.Errorf("POST without Origin: %d, want 403", got)
	}
}

func TestRevoke(t *testing.T) {
	g := newGuard(t)
	done := 0
	a, _ := g.Nonce(func() { done++ })
	b, _ := g.Nonce(func() { done++ })
	g.Revoke()
	if done != 2 {
		t.Errorf("done ran %d times, want 2", done)
	}
	for _, n := range []string{a, b} {
		if w := g.do(t, req{path: "/?nonce=" + url.QueryEscape(n)}); w.Code != http.StatusForbidden {
			t.Errorf("revoked nonce: %d, want 403", w.Code)
		}
	}
}
