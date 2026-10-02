// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package fixture is shop-api, the test server the desktop's tests and
// browser tests run requests against: a login that sets a token and a
// session cookie, users, carts and a checkout whose order stays pending,
// a validation error, Server-Sent Events with heartbeats, a WebSocket
// echo, slow and binary and gzip responses, an HTML page with a script, a
// PNG and a JSON body of any size. Secrets it echoes let tests check
// redaction.
package fixture

import (
	"bufio"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	// users created by POST /users, by id.
	users map[string]map[string]string
	seen  []string
	wire  []string
}

// New returns a fresh shop-api.
func New() *Server {
	return &Server{carts: map[string][]string{}, users: map[string]map[string]string{}}
}

// Wire returns "METHOD path|Authorization|Cookie" of every request so far,
// for comparing requests on the wire.
func (s *Server) Wire() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.wire...)
}

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
	s.wire = append(s.wire, r.Method+" "+r.URL.RequestURI()+"|"+r.Header.Get("Authorization")+"|"+r.Header.Get("Cookie"))
	s.mu.Unlock()
	p := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(p, "/")
	switch {
	case p == "health": // readiness, for the browser tests
		w.WriteHeader(http.StatusNoContent)
	case r.Method == "POST" && p == "login":
		var body struct{ User, Password string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.User != User || body.Password != Password {
			writeJSON(w, 401, map[string]string{"error": "bad credentials"})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: Session, Path: "/", HttpOnly: true}) //nolint:gosec // test cookie
		writeJSON(w, 200, map[string]any{"token": Token, "user": map[string]any{"id": 7, "name": User}})
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "users" && s.user(parts[1]) != nil:
		writeJSON(w, 200, s.user(parts[1]))
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "users":
		if !s.authorized(r) {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		writeJSON(w, 200, map[string]any{"id": 7, "name": User, "big": json.Number("12345678901234567890")})
	case r.Method == "POST" && p == "users":
		// A user with a name and an email is created; an email that is not
		// one is refused, and so is a user without one.
		var u struct{ Name, Email string }
		_ = json.NewDecoder(r.Body).Decode(&u)
		switch {
		case u.Email == "":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(422)
			_, _ = io.WriteString(w, `{"title":"invalid user","errors":[{"field":"email","message":"is required"}]}`)
		case !validEmail(u.Email):
			writeJSON(w, 422, map[string]string{"error": "invalid_email", "field": "email"})
		default:
			writeJSON(w, 201, s.addUser(u.Name, u.Email))
		}
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
	case p == "orders/export":
		s.export(w, r)
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
	case p == "page":
		// A script and a remote load the preview must block.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	case p == "pixel.png":
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pixel)
	case p == "big":
		s.big(w, r)
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

// page is an HTML receipt whose script, if it ran, would change the title
// and call the server.
const page = `<!doctype html><html><head><title>Receipt</title><style>
body{margin:0;padding:22px 24px;font:14px/1.5 -apple-system,system-ui,sans-serif;color:#1d2433;background:#fff}
.shop{font-size:11px;letter-spacing:.12em;color:#6b7385}h1{margin:4px 0 0;font-size:22px;font-weight:600}
.sub{color:#6b7385;margin:0 0 14px}table{width:100%;border-collapse:collapse;border-top:1px solid #e3e6ec}
td{padding:6px 0}td+td{text-align:right}.muted td{color:#8a91a1}.total td{font-weight:600;border-top:1px solid #e3e6ec;padding-top:8px}
.badge{display:inline-block;margin-top:10px;padding:2px 8px;border-radius:5px;background:#fdf1d6;color:#8a5a00;font-size:12px;font-weight:600}
</style></head><body>
<div class="shop">SHOP</div><h1 id="title">Receipt</h1><p class="sub">Order o-c1 · 29 Sep 2026</p>
<table><tr><td>Earl Grey 250 g × 2</td><td>25.80 EUR</td></tr><tr class="muted"><td>Shipping</td><td>0.00 EUR</td></tr>
<tr class="total"><td>Total</td><td>25.80 EUR</td></tr></table><span class="badge">Payment pending</span>
<img src="/health?from=preview-img" alt="">
<script>document.getElementById("title").textContent = "SCRIPT RAN"; fetch("/health?from=preview-script");</script>
</body></html>`

// pixel is a 1×1 PNG.
var pixel, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")

// big streams a JSON array of orders of about ?mb= MiB (default 1); the
// first order's id is a 64-bit integer.
func (s *Server) big(w http.ResponseWriter, r *http.Request) {
	mb, err := strconv.Atoi(r.URL.Query().Get("mb"))
	if err != nil || mb < 1 || mb > 200 {
		mb = 1
	}
	w.Header().Set("Content-Type", "application/json")
	bw := bufio.NewWriterSize(w, 1<<16)
	_, _ = io.WriteString(bw, `[{"id":12345678901234567890,"status":"paid","total":1}`)
	size := 0
	for i := 1; size < mb<<20; i++ {
		n, _ := fmt.Fprintf(bw, `,{"id":%d,"sku":"TEA-%05d","status":"paid","total":%d.5,"currency":"EUR"}`, i, i%100000, i%1000)
		size += n
	}
	_, _ = io.WriteString(bw, "]")
	_ = bw.Flush()
}

// export streams ?mb= MiB (default 1) of orders as NDJSON, one a line.
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	mb, err := strconv.Atoi(r.URL.Query().Get("mb"))
	if err != nil || mb < 1 || mb > 200 {
		mb = 1
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	bw := bufio.NewWriterSize(w, 1<<16)
	statuses := []string{"paid", "paid", "refunded", "paid", "pending", "paid", "paid", "cancelled", "paid"}
	size := 0
	for i := 1; size < mb<<20; i++ {
		n, _ := fmt.Fprintf(bw, `{"id":"ord_%d","status":%q,"total":%d.%d,"currency":"EUR"}`+"\n", 1000+i, statuses[i%len(statuses)], 9+(i*7)%90, i%10)
		size += n
	}
	_ = bw.Flush()
}

// events streams ?n= events (default 3) with heartbeat comments between
// them.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	n, err := strconv.Atoi(r.URL.Query().Get("n"))
	if err != nil || n < 1 || n > 1000 {
		n = 3
	}
	for i := 1; i <= n; i++ {
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

// user returns the user created with id, or nil.
func (s *Server) user(id string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.users[id]
}

// addUser creates a user and returns it.
func (s *Server) addUser(name, email string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.Itoa(100 + len(s.users))
	s.users[id] = map[string]string{"id": id, "name": name, "email": email}
	return s.users[id]
}

// validEmail reports whether e looks like an email: a name, @, a domain
// with a dot.
func validEmail(e string) bool {
	at := strings.LastIndex(e, "@")
	return at > 0 && strings.Contains(e[at+1:], ".") && !strings.HasSuffix(e, ".")
}
