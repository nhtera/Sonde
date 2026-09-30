// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package serverauth guards the HTTP server of server mode. Anything that
// reaches the loopback port could otherwise drive a request runner with
// file access, so every request passes these checks:
//
//   - Host is 127.0.0.1:<port> or localhost:<port> (no DNS rebinding);
//   - Origin, when sent, is this server, and every request other than GET
//     and HEAD must send it; a browser's Sec-Fetch-Site of same-site or
//     cross-site is refused (cookies are shared across 127.0.0.1 ports);
//   - the page opens with a one-time nonce, exchanged for an HttpOnly,
//     SameSite=Strict session cookie; the page then reads the per-launch
//     token from SessionPath and sends it as the TokenHeader of every call;
//   - runtime calls and app endpoints need the token; the page's assets
//     and body URLs need the cookie;
//   - no response can be framed, and none carries CORS headers.
//
// Tokens, cookies and nonces are compared in constant time and are never
// logged.
package serverauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TokenHeader carries the per-launch token on runtime calls.
const TokenHeader = "X-Sonde-Token" //nolint:gosec // a header name, not a credential

// Paths the guard serves or classifies.
const (
	// SessionPath returns {"token": ...} to a page holding the cookie.
	SessionPath = "/_sonde/session"
	// BodyPrefix serves response bodies; the cookie suffices (ids are
	// 128-bit random), so a fetch from a worker needs no token.
	BodyPrefix = "/_sonde/body/"
	// appPrefix is every other app endpoint: the token is required.
	appPrefix = "/_sonde/"
)

// NonceTTL is how long a launch nonce stays valid.
const NonceTTL = 60 * time.Second

// Guard checks the requests of one server.
type Guard struct {
	port   int
	token  string
	cookie string // session cookie value; "" in fixed mode
	fixed  bool

	mu     sync.Mutex
	nonces map[string]*nonce
	now    func() time.Time
}

type nonce struct {
	expires time.Time
	done    func()
	timer   *time.Timer
}

// New returns a guard for a server listening on 127.0.0.1:port, with a
// fresh random token and session cookie.
func New(port int) (*Guard, error) {
	token, err := random()
	if err != nil {
		return nil, err
	}
	cookie, err := random()
	if err != nil {
		return nil, err
	}
	return &Guard{port: port, token: token, cookie: cookie, nonces: map[string]*nonce{}, now: time.Now}, nil
}

// NewFixed returns a guard for the test-only harness: token is fixed and
// known to the tests, and assets need no cookie. Host, Origin and framing
// are checked as in New.
func NewFixed(port int, token string) *Guard {
	return &Guard{port: port, token: token, fixed: true, nonces: map[string]*nonce{}, now: time.Now}
}

// CookieName is the session cookie's name, per port: a server on another
// 127.0.0.1 port shares the cookie jar but not the name.
func (g *Guard) CookieName() string { return "sonde_session_" + strconv.Itoa(g.port) }

// Nonce mints a launch nonce, valid once within NonceTTL. done, when not
// nil, runs once when the nonce is used or expires (the launch link file
// is removed then).
func (g *Guard) Nonce(done func()) (string, error) {
	n, err := random()
	if err != nil {
		return "", err
	}
	entry := &nonce{expires: g.now().Add(NonceTTL), done: done}
	g.mu.Lock()
	defer g.mu.Unlock()
	entry.timer = time.AfterFunc(NonceTTL, func() { g.take(n, false) })
	g.nonces[n] = entry
	return n, nil
}

// Revoke invalidates every pending nonce, running their done callbacks.
func (g *Guard) Revoke() {
	g.mu.Lock()
	pending := g.nonces
	g.nonces = map[string]*nonce{}
	g.mu.Unlock()
	for _, e := range pending {
		e.timer.Stop()
		if e.done != nil {
			e.done()
		}
	}
}

// take removes nonce n and reports whether it was pending and unexpired.
// Keys are compared in constant time: a map lookup would leak timing.
func (g *Guard) take(n string, use bool) bool {
	g.mu.Lock()
	var (
		key   string
		entry *nonce
	)
	for k, e := range g.nonces {
		if equal(k, n) {
			key, entry = k, e
		}
	}
	if entry != nil {
		delete(g.nonces, key)
	}
	g.mu.Unlock()
	if entry == nil {
		return false
	}
	entry.timer.Stop()
	if entry.done != nil {
		entry.done()
	}
	return !use || g.now().Before(entry.expires)
}

// Middleware wraps the server's handler with the checks.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Referrer-Policy", "no-referrer")

		if !g.hostOK(r.Host) {
			deny(w, http.StatusForbidden, "host not allowed")
			return
		}
		path := r.URL.Path
		// The launch link may be opened from a file (--open) or another
		// app, so its navigation is cross-site: the nonce is the check.
		if path == "/" && r.URL.Query().Has("nonce") && !g.fixed {
			g.exchange(w, r)
			return
		}
		if !g.originOK(r) {
			deny(w, http.StatusForbidden, "origin not allowed")
			return
		}
		switch {
		case path == SessionPath:
			if !g.cookieOK(r) {
				deny(w, http.StatusUnauthorized, "open the launch link again")
				return
			}
			h.Set("Cache-Control", "no-store")
			h.Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": g.token})
		case strings.HasPrefix(path, BodyPrefix):
			if !g.cookieOK(r) {
				deny(w, http.StatusUnauthorized, "no session")
				return
			}
			h.Set("Cache-Control", "no-store")
			// The Results preview frames a body (sandboxed, by the body's
			// own CSP): the app's page may frame it, no other page.
			h.Set("Content-Security-Policy", "frame-ancestors 'self'")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			next.ServeHTTP(w, r)
		case needsToken(path):
			if !g.cookieOK(r) || !g.tokenOK(r) {
				deny(w, http.StatusUnauthorized, "no session token")
				return
			}
			h.Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		default:
			if !g.cookieOK(r) {
				deny(w, http.StatusUnauthorized, "open the launch link printed by sonde-desktop")
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

// needsToken reports whether path is a runtime call or app endpoint: the
// runtime's call, stream and event payload routes, and our own endpoints.
func needsToken(path string) bool {
	return path == "/wails/runtime" ||
		strings.HasPrefix(path, "/wails/stream/") ||
		strings.HasPrefix(path, "/wails/eventpayload/") ||
		strings.HasPrefix(path, appPrefix)
}

// exchange trades a launch nonce for the session cookie, then reloads the
// page without the nonce. The reload is a page of our own rather than a
// redirect: a redirect would continue the cross-site navigation, whose
// requests carry no SameSite=Strict cookie.
func (g *Guard) exchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !g.take(r.URL.Query().Get("nonce"), true) {
		deny(w, http.StatusForbidden, "this launch link was used or has expired")
		return
	}
	// Not Secure: the server is plain HTTP on loopback, where browsers
	// would then drop the cookie.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // see above
		Name:     g.CookieName(),
		Value:    g.cookie,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	_, _ = io.WriteString(w, reloadPage)
}

// reloadPage opens the app at / once the cookie is set.
const reloadPage = `<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=/"><title>Sonde</title><a href="/">Open Sonde</a>`

func (g *Guard) hostOK(host string) bool {
	port := ":" + strconv.Itoa(g.port)
	return host == "127.0.0.1"+port || host == "localhost"+port
}

// originOK checks Origin and Sec-Fetch-Site. A browser sends Origin on
// every request but a plain GET or HEAD; requiring it elsewhere refuses
// clients that hide it.
func (g *Guard) originOK(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) > 1 {
		return false
	}
	if len(origins) == 0 {
		return r.Method == http.MethodGet || r.Method == http.MethodHead
	}
	return origins[0] == "http://"+r.Host
}

func (g *Guard) cookieOK(r *http.Request) bool {
	if g.fixed {
		return true
	}
	c, err := r.Cookie(g.CookieName())
	return err == nil && equal(c.Value, g.cookie)
}

func (g *Guard) tokenOK(r *http.Request) bool {
	values := r.Header.Values(TokenHeader)
	return len(values) == 1 && equal(values[0], g.token)
}

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func deny(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, msg, code)
}

// random returns 256 random bits, URL-safe.
func random() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.New("serverauth: no randomness")
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
