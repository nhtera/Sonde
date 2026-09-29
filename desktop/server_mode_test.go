// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build server && !e2eharness

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/appdirs"
	"github.com/nhtera/sonde/desktop/internal/serverauth"
)

// TestServerMode runs the server-mode app and checks the guard covers
// every route but Wails' own: health, custom.js and the event and stream
// sockets.
func TestServerMode(t *testing.T) {
	// Wails runs one app per process: with -count>1 only the first run can.
	if serverModeRan {
		t.Skip("Wails runs one app per process")
	}
	serverModeRan = true
	t.Setenv("WAILS_SERVER_HOST", "0.0.0.0") // must be ignored
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := appdirs.Open(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer dirs.Close()
	guard, err := serverauth.New(port)
	if err != nil {
		t.Fatal(err)
	}
	link := &launchLink{dir: dirs.Cache(), guard: guard, port: port}
	launch, err := link.mint()
	if err != nil {
		t.Fatal(err)
	}
	app := newServerApp(&Host{Mode: ModeServer, Root: t.TempDir(), Dirs: dirs}, port, guard)
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	defer func() {
		app.Quit()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("server did not stop")
		}
	}()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	awaitStart(t, port, done)

	t.Run("bound to loopback only", func(t *testing.T) {
		for _, ip := range localIPs(t) {
			c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), time.Second)
			if err == nil {
				_ = c.Close()
				t.Errorf("reachable on %s", ip)
			}
		}
	})

	t.Run("guarded routes", func(t *testing.T) {
		for _, tc := range []struct {
			method, path string
			host         string
			want         int
		}{
			{"GET", "/", "", 401},
			{"GET", "/index.html", "", 401},
			{"GET", "/wails/runtime.js", "", 401},
			{"GET", "/wails/transport.js", "", 401},
			{"GET", "/_sonde/session", "", 401},
			{"GET", serverauth.BodyPrefix + "0123", "", 401},
			{"POST", "/wails/runtime", "", 403}, // no Origin
			{"GET", "/", "evil.example:" + strconv.Itoa(port), 403},
			{"GET", "/", "localhost:1", 403},
		} {
			req, _ := http.NewRequest(tc.method, base+tc.path, nil)
			if tc.host != "" {
				req.Host = tc.host
			}
			res := do(t, http.DefaultClient, req)
			if res.StatusCode != tc.want {
				t.Errorf("%s %s (Host %q): %d, want %d", tc.method, tc.path, tc.host, res.StatusCode, tc.want)
			}
			if res.Header.Get("X-Frame-Options") != "DENY" {
				t.Errorf("%s %s: framing not denied", tc.method, tc.path)
			}
		}
		req, _ := http.NewRequest("POST", base+"/wails/runtime", strings.NewReader("{}"))
		req.Header.Set("Origin", base)
		if res := do(t, http.DefaultClient, req); res.StatusCode != 401 {
			t.Errorf("runtime call without token: %d, want 401", res.StatusCode)
		}
	})

	t.Run("unguarded Wails routes carry no app data", func(t *testing.T) {
		for path, want := range map[string]int{"/health": 200, "/wails/custom.js": 200} {
			if res := do(t, http.DefaultClient, get(base+path)); res.StatusCode != want {
				t.Errorf("%s: %d, want %d", path, res.StatusCode, want)
			}
		}
		// The stream socket has no handlers registered.
		if res := do(t, http.DefaultClient, get(base+"/wails/stream/ws?name=x")); res.StatusCode != 404 {
			t.Errorf("stream socket: %d, want 404", res.StatusCode)
		}
	})

	// Wails' event socket is outside the guard and accepts a DNS-rebinding
	// page (its Origin matches its Host), so no app data may travel on it.
	t.Run("event socket carries nothing", func(t *testing.T) {
		c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		evil := "evil.example:" + strconv.Itoa(port)
		_, err = io.WriteString(c, "GET /wails/events HTTP/1.1\r\nHost: "+evil+"\r\nOrigin: http://"+evil+
			"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		r := bufio.NewReader(c)
		status, err := r.ReadString('\n')
		if err != nil || !strings.Contains(status, " 101 ") {
			t.Skipf("event socket refused the rebinding client (%q): nothing to check", status)
		}
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if line == "\r\n" {
				break
			}
		}
		// Startup is over; any frame now is an app event leaking.
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if b, err := r.ReadByte(); err == nil {
			t.Errorf("event socket sent a frame (first byte %#x): server mode must send events through its guarded endpoints", b)
		}
	})

	t.Run("launch link, session, runtime call", func(t *testing.T) {
		page, err := os.ReadFile(launch)
		if err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(launch); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("launch file mode: %v %v", fi.Mode(), err)
		}
		m := regexp.MustCompile(`url=([^"]+)"`).FindSubmatch(page)
		if m == nil {
			t.Fatalf("no link in %s", page)
		}
		link := strings.ReplaceAll(string(m[1]), "&amp;", "&")
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar}
		if res := do(t, client, get(link)); res.StatusCode != 200 {
			t.Fatalf("exchange: %d", res.StatusCode)
		}
		if _, err := os.Stat(launch); !os.IsNotExist(err) {
			t.Error("launch file kept after use")
		}
		if res := do(t, client, get(link)); res.StatusCode != 403 {
			t.Errorf("reused link: %d, want 403", res.StatusCode)
		}
		if res := do(t, client, get(base+"/")); res.StatusCode != 200 {
			t.Errorf("page with cookie: %d, want 200", res.StatusCode)
		}
		res, err := client.Do(get(base + serverauth.SessionPath))
		if err != nil {
			t.Fatal(err)
		}
		var session struct{ Token string }
		err = json.NewDecoder(res.Body).Decode(&session)
		_ = res.Body.Close()
		if err != nil || session.Token == "" {
			t.Fatalf("session: %d %v", res.StatusCode, err)
		}
		call := func(token string) int {
			req, _ := http.NewRequest("POST", base+"/wails/runtime", strings.NewReader(`{"object":0,"method":0,"args":{}}`))
			req.Header.Set("Origin", base)
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set(serverauth.TokenHeader, token)
			}
			return do(t, client, req).StatusCode
		}
		if got := call(""); got != 401 {
			t.Errorf("cookie without token: %d, want 401", got)
		}
		if got := call(session.Token); got == 401 || got == 403 {
			t.Errorf("cookie and token: %d, want the runtime's answer", got)
		}
	})
}

var serverModeRan bool

func get(u string) *http.Request {
	req, _ := http.NewRequest("GET", u, nil)
	return req
}

func do(t *testing.T, c *http.Client, req *http.Request) *http.Response {
	t.Helper()
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res
}

func awaitStart(t *testing.T, port int, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("server stopped: %v", err)
		default:
		}
		c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start")
}

// localIPs are this machine's non-loopback addresses.
func localIPs(t *testing.T) []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var ips []string
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			ips = append(ips, n.IP.String())
		}
	}
	return ips
}

// TestLaunchLink: a new link revokes the previous one; using a link
// removes its file; shutdown removes a pending one.
func TestLaunchLink(t *testing.T) {
	dirs, err := appdirs.Open(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer dirs.Close()
	guard, err := serverauth.New(7781)
	if err != nil {
		t.Fatal(err)
	}
	link := &launchLink{dir: dirs.Cache(), guard: guard, port: 7781}
	nonceIn := func(path string) string {
		page, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(`nonce=([A-Za-z0-9_-]+)`).FindSubmatch(page)
		if m == nil {
			t.Fatal("no nonce in the launch page")
		}
		return string(m[1])
	}
	exchange := func(nonce string) int {
		r := httptest.NewRequest("GET", "http://127.0.0.1:7781/?nonce="+nonce, nil)
		w := httptest.NewRecorder()
		guard.Middleware(http.NotFoundHandler()).ServeHTTP(w, r)
		return w.Code
	}
	path, err := link.mint()
	if err != nil {
		t.Fatal(err)
	}
	first := nonceIn(path)
	if _, err := link.mint(); err != nil {
		t.Fatal(err)
	}
	second := nonceIn(path)
	if got := exchange(first); got != 403 {
		t.Errorf("replaced link: %d, want 403", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the replaced link's revocation removed the new file: %v", err)
	}
	if got := exchange(second); got != 200 {
		t.Errorf("current link: %d, want 200", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("used link's file kept")
	}
	if _, err := link.mint(); err != nil {
		t.Fatal(err)
	}
	link.remove()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("pending link's file kept after shutdown")
	}
}
