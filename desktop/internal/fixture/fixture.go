// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package fixture is shop-api, the test server the desktop's tests and
// browser tests run requests against: a login that sets a token and a
// session cookie, users, carts and a checkout whose order stays pending,
// a validation error, Server-Sent Events with heartbeats, a WebSocket
// echo, slow and binary and gzip responses. Secrets it echoes let tests
// check redaction.
package fixture

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Credentials of the fixture's user.
const (
	User     = "ada"
	Password = "fixture-password-3141"                                        //nolint:gosec // G101: a test credential
	Token    = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI3In0.Zml4dHVyZS1zaWduYXR1cmU" //nolint:gosec // G101: a test token
	Session  = "fixture-session-2718"                                         //nolint:gosec // G101: a test cookie
)

// Server is shop-api's handler.
type Server struct {
	mu    sync.Mutex
	carts map[string][]string
	seen  []string
}

// New returns a fresh shop-api.
func New() *Server { return &Server{carts: map[string][]string{}} }

// Requests returns "METHOD path" of every request so far.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) authorized(r *http.Request) bool {
	c, err := r.Cookie("sid")
	return r.Header.Get("Authorization") == "Bearer "+Token && err == nil && c.Value == Session
}

// ServeHTTP routes shop-api.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.seen = append(s.seen, r.Method+" "+r.URL.Path)
	s.mu.Unlock()
	p := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(p, "/")
	switch {
	case r.Method == "POST" && p == "login":
		var body struct{ User, Password string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.User != User || body.Password != Password {
			writeJSON(w, 401, map[string]string{"error": "bad credentials"})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: Session, Path: "/", HttpOnly: true}) //nolint:gosec // test cookie
		writeJSON(w, 200, map[string]any{"token": Token, "user": map[string]any{"id": 7, "name": User}})
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "users":
		if !s.authorized(r) {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		writeJSON(w, 200, map[string]any{"id": 7, "name": User, "big": json.Number("12345678901234567890")})
	case r.Method == "POST" && p == "users":
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(422)
		_, _ = io.WriteString(w, `{"title":"invalid user","errors":[{"field":"email","message":"is required"}]}`)
	case r.Method == "POST" && p == "carts":
		if !s.authorized(r) {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		s.mu.Lock()
		id := fmt.Sprintf("c%d", len(s.carts)+1)
		s.carts[id] = nil
		s.mu.Unlock()
		writeJSON(w, 201, map[string]string{"id": id})
	case r.Method == "POST" && len(parts) == 3 && parts[0] == "carts" && parts[2] == "items":
		var item struct{ SKU string }
		_ = json.NewDecoder(r.Body).Decode(&item)
		s.mu.Lock()
		s.carts[parts[1]] = append(s.carts[parts[1]], item.SKU)
		n := len(s.carts[parts[1]])
		s.mu.Unlock()
		writeJSON(w, 200, map[string]any{"cart": parts[1], "items": n})
	case r.Method == "POST" && len(parts) == 3 && parts[0] == "carts" && parts[2] == "checkout":
		writeJSON(w, 200, map[string]string{"order": "o-" + parts[1], "status": "pending"})
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "orders":
		writeJSON(w, 200, map[string]string{"id": parts[1], "status": "pending"})
	case p == "events":
		s.events(w, r)
	case p == "ws":
		s.ws(w, r)
	case p == "slow":
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
			writeJSON(w, 200, map[string]bool{"slow": true})
		}
	case p == "binary":
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(append([]byte{0, 1, 2, 0xfe, 0xff}, []byte("token="+r.Header.Get("X-Token"))...)) //nolint:gosec // G705: an echo, for redaction tests
	case p == "gzip":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_ = json.NewEncoder(zw).Encode(map[string]string{"echo": r.Header.Get("X-Token")})
		_ = zw.Close()
	default:
		writeJSON(w, 404, map[string]string{"error": "not found"})
	}
}

// events streams three events with heartbeat comments between them.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	for i := 1; i <= 3; i++ {
		_, _ = fmt.Fprintf(w, ": heartbeat\n\nid: %d\nevent: order\ndata: {\"n\":%d}\n\n", i, i)
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// ws echoes every message until the client closes.
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow() //nolint:errcheck // closing
	for {
		typ, data, err := c.Read(r.Context())
		if err != nil {
			return
		}
		if err := c.Write(r.Context(), typ, data); err != nil {
			return
		}
	}
}
