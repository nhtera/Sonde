// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// serverSpec describes one background Python server the harness starts
// before running scripts that need it. Ports and arguments mirror
// bin/test/test_prerequisites.sh in the reference repository, which is how
// upstream CI starts the same servers.
type serverSpec struct {
	name string
	host string
	port int // 0 for the Unix-socket server, which has no TCP port.
	args []string
	// capability is the name a script's classification uses to say it
	// needs this server; see Script.NeedsProxy and the ssl/unixsocket/ipv6
	// lane reasons in lanes.go.
	capability string
}

const unixSocketPath = "build/unix_socket.sock"

// blockingServers are required for LaneBlocking; extendedServers are
// additionally required for LaneExtended.
var blockingServers = []serverSpec{
	{name: "server.py", host: "127.0.0.1", port: 8000, args: []string{"server.py", "--host", "127.0.0.1", "--port", "8000"}},
}

var extendedServers = []serverSpec{
	{name: "server.py (IPv6)", host: "::1", port: 8004, args: []string{"server.py", "--host", "::1", "--port", "8004"}, capability: "ipv6"},
	{name: "ssl_server.py (self-signed)", host: "127.0.0.1", port: 8001, args: []string{"tests_ssl/ssl_server.py", "8001", "tests_ssl/certs/server/cert.selfsigned.pem", "false"}, capability: "ssl"},
	{name: "ssl_server.py (CA-signed)", host: "127.0.0.1", port: 8002, args: []string{"tests_ssl/ssl_server.py", "8002", "tests_ssl/certs/server/cert.pem", "false"}, capability: "ssl"},
	{name: "ssl_server.py (client cert auth)", host: "127.0.0.1", port: 8003, args: []string{"tests_ssl/ssl_server.py", "8003", "tests_ssl/certs/server/cert.selfsigned.pem", "true"}, capability: "ssl"},
	{name: "unix_socket_server.py", args: []string{"tests_unix_socket/unix_socket_server.py"}, capability: "unixsocket"},
}

// serverManager owns every child process the harness starts and can tear
// them all down in one call.
type serverManager struct {
	python       string
	hurlRoot     string
	procs        []*os.Process
	capabilities map[string]bool
}

func newServerManager(python, hurlRoot string) *serverManager {
	return &serverManager{python: python, hurlRoot: hurlRoot, capabilities: map[string]bool{}}
}

// StartBlocking starts the plain HTTP server every lane needs.
func (m *serverManager) StartBlocking() error {
	for _, s := range blockingServers {
		if err := m.start(s); err != nil {
			return err
		}
	}
	return nil
}

// StartExtended starts the TLS, IPv6 and Unix-socket servers, and the local
// squid proxy if one is installed. Squid is optional: its absence is
// logged, and scripts whose Script.NeedsProxy is true are skipped at run
// time rather than failing the harness.
func (m *serverManager) StartExtended(log func(string)) error {
	for _, s := range extendedServers {
		if err := m.start(s); err != nil {
			return err
		}
		if s.capability != "" {
			m.capabilities[s.capability] = true
		}
	}
	if err := m.startSquid(log); err != nil {
		return err
	}
	return nil
}

// HasCapability reports whether a server providing capability was started
// successfully.
func (m *serverManager) HasCapability(capability string) bool {
	return m.capabilities[capability]
}

func (m *serverManager) start(s serverSpec) error {
	if s.port != 0 {
		if pid, busy := portOwner(s.host, s.port); busy {
			return fmt.Errorf("port %d (%s) is already in use by pid %s; stop it (or the other conformance run: make conformance and make conformance-next share these ports) before running the conformance suite", s.port, s.name, pid)
		}
	}

	cmd := exec.Command(m.python, s.args...) //nolint:gosec // G204: fixed, vendored args; python path is developer/CI-controlled.
	cmd.Dir = m.hurlRoot
	logPath := filepath.Join(m.hurlRoot, "build", strings.ReplaceAll(s.name, " ", "_")+".log")
	if err := os.MkdirAll(filepath.Join(m.hurlRoot, "build"), 0o750); err != nil {
		return err
	}
	logFile, err := os.Create(logPath) //nolint:gosec // G304: fixed path under the vendored tree.
	if err != nil {
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = groupAttr()

	if err := cmd.Start(); err != nil {
		logFile.Close() //nolint:errcheck,gosec // best-effort; Start already failed.
		return fmt.Errorf("starting %s: %w", s.name, err)
	}
	m.procs = append(m.procs, cmd.Process)

	ready := waitReady(m.hurlRoot, s)
	if !ready {
		b, _ := os.ReadFile(logPath) //nolint:gosec,errcheck // G304: fixed path; best-effort diagnostic read.
		return fmt.Errorf("%s did not become ready within the timeout; log:\n%s", s.name, string(b))
	}
	return nil
}

// startSquid starts the local forward proxy used by the proxy tests. It is
// entirely optional: CI images that install squid get proxy coverage,
// developer machines without it do not, and either way the harness keeps
// going.
func (m *serverManager) startSquid(log func(string)) error {
	squid, err := exec.LookPath("squid")
	if err != nil {
		log("squid not found on PATH; proxy-dependent conformance scripts will be skipped")
		return nil
	}
	if _, busy := portOwner("127.0.0.1", 3128); busy {
		log("port 3128 is already in use; skipping squid startup, proxy-dependent scripts will be skipped")
		return nil
	}

	// Upstream CI runs squid under sudo; the harness runs it as the current
	// user, so it must not try to write the system pid file.
	conf := "pid_filename none\ncache deny all\ncache_log /dev/null\naccess_log /dev/null\nhttp_access allow all\n" +
		"http_port 127.0.0.1:3128\nrequest_header_add From-Proxy Hello\nreply_header_add From-Proxy Hello\n"

	cmd := exec.Command(squid, "-d", "2", "-N", "-f", "/dev/stdin") //nolint:gosec // G204: fixed args, resolved from PATH.
	cmd.Stdin = strings.NewReader(conf)
	logPath := filepath.Join(m.hurlRoot, "build", "squid.log")
	logFile, err := os.Create(logPath) //nolint:gosec // G304: fixed path under the vendored tree.
	if err != nil {
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = groupAttr()

	if err := cmd.Start(); err != nil {
		log(fmt.Sprintf("squid found but failed to start (%v); proxy-dependent scripts will be skipped", err))
		return nil
	}
	m.procs = append(m.procs, cmd.Process)

	if !pollUntilReady(func() bool { _, busy := portOwner("127.0.0.1", 3128); return busy }) {
		b, _ := os.ReadFile(logPath) //nolint:gosec,errcheck // G304: fixed path; best-effort diagnostic read.
		log(fmt.Sprintf("squid did not become ready within the timeout; proxy-dependent scripts will be skipped; log:\n%s", string(b)))
		return nil
	}
	m.capabilities["proxy"] = true
	return nil
}

// Stop terminates every process this manager started, process-group first
// (SIGTERM, then SIGKILL if it is still alive after a grace period).
func (m *serverManager) Stop() {
	for _, p := range m.procs {
		killGroup(p.Pid, syscall.SIGTERM)
	}
	time.Sleep(500 * time.Millisecond)
	for _, p := range m.procs {
		killGroup(p.Pid, syscall.SIGKILL)
	}
	for _, p := range m.procs {
		_, _ = p.Wait() //nolint:errcheck // reaping zombies; the exit status is not interesting here.
	}
	m.procs = nil
}

// waitReady polls the server's TCP port (or, for the Unix-socket server,
// the socket file) until it accepts connections or the timeout elapses.
// 30 one-second tries mirrors bin/test/test_prerequisites.sh's
// check_listen_port in the reference repository.
func waitReady(hurlRoot string, s serverSpec) bool {
	if s.port == 0 {
		return pollUntilReady(func() bool {
			c, err := net.DialTimeout("unix", filepath.Join(hurlRoot, unixSocketPath), time.Second)
			if err != nil {
				return false
			}
			_ = c.Close() //nolint:errcheck // probe-only connection.
			return true
		})
	}
	return pollUntilReady(func() bool {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(s.host, strconv.Itoa(s.port)), time.Second)
		if err != nil {
			return false
		}
		_ = c.Close() //nolint:errcheck // probe-only connection.
		return true
	})
}

func pollUntilReady(probe func() bool) bool {
	for range 30 {
		if probe() {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

// portOwner reports whether host:port is already bound, and by which pid
// if `lsof` can say so (best-effort; a missing lsof or an unparsable
// answer just yields an empty pid with busy=true).
func portOwner(host string, port int) (pid string, busy bool) {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 200*time.Millisecond)
	if err != nil {
		return "", false
	}
	_ = c.Close() //nolint:errcheck // probe-only connection.

	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output() //nolint:gosec // G204: fixed args, port is ours.
	if err != nil {
		return "", true
	}
	return strings.TrimSpace(string(out)), true
}
