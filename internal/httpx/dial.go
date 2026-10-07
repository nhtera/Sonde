// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/netpolicy"
	"github.com/nhtera/sonde/internal/sandbox"
)

// connectToRule is one --connect-to HOST1:PORT1:HOST2:PORT2 rule: dial
// HOST2:PORT2 instead of HOST1:PORT1, keeping the original Host header
// and TLS server name. An empty HOST1 or PORT1 matches any value.
type connectToRule struct {
	fromHost, fromPort, toHost, toPort string
}

func parseConnectTo(rules []string) []connectToRule {
	var out []connectToRule
	for _, r := range rules {
		parts := strings.SplitN(r, ":", 4)
		if len(parts) != 4 {
			continue
		}
		out = append(out, connectToRule{parts[0], parts[1], parts[2], parts[3]})
	}
	return out
}

func matchConnectTo(rules []connectToRule, host, port string) (string, string, bool) {
	for _, r := range rules {
		if (r.fromHost == "" || strings.EqualFold(r.fromHost, host)) &&
			(r.fromPort == "" || r.fromPort == port) {
			h, p := r.toHost, r.toPort
			if h == "" {
				h = host
			}
			if p == "" {
				p = port
			}
			return h, p, true
		}
	}
	return "", "", false
}

// resolveRule is one --resolve HOST:PORT:ADDR[,ADDR...] rule.
type resolveRule struct {
	host, port string
	addrs      []string
}

func parseResolve(rules []string) []resolveRule {
	var out []resolveRule
	for _, r := range rules {
		parts := strings.SplitN(r, ":", 3)
		if len(parts) != 3 {
			continue
		}
		addrs := strings.Split(parts[2], ",")
		out = append(out, resolveRule{parts[0], parts[1], addrs})
	}
	return out
}

func matchResolve(rules []resolveRule, host, port string) ([]string, bool) {
	for _, r := range rules {
		if strings.EqualFold(r.host, host) && r.port == port {
			return r.addrs, true
		}
	}
	return nil, false
}

// dialOptions is the subset of Options that shapes how a connection is
// made; it is also the cache key for a Client's per-options transport.
type dialOptions struct {
	network        string // "tcp", "tcp4" or "tcp6"
	connectTo      []connectToRule
	resolve        []resolveRule
	unixSocket     string
	connectTimeout time.Duration
	hosts          *netpolicy.Policy
}

func newDialOptions(opts *Options, box *sandbox.Root, hosts *netpolicy.Policy) (dialOptions, error) {
	d := dialOptions{
		hosts:          hosts,
		network:        "tcp",
		connectTo:      parseConnectTo(opts.ConnectTo),
		resolve:        parseResolve(opts.Resolve),
		connectTimeout: opts.ConnectTimeout,
	}
	switch opts.IPResolve {
	case IPv4:
		d.network = "tcp4"
	case IPv6:
		d.network = "tcp6"
	}
	if d.connectTimeout <= 0 {
		d.connectTimeout = 300 * time.Second
	}
	if opts.UnixSocket != "" {
		p, err := box.Path(opts.UnixSocket)
		if opts.LocalFiles[opts.UnixSocket] {
			p, err = filepath.Abs(opts.UnixSocket)
		}
		if err != nil {
			return dialOptions{}, err
		}
		d.unixSocket = p
	}
	return d, nil
}

// dialContext builds the DialContext used by an http.Transport, applying
// unix-socket, connect-to, resolve and IP family restrictions. With a
// host policy, the host actually dialed (after connect-to, and each
// address of a resolve rule) must be allowed, and unix sockets are
// refused: this is where a proxy or a rewritten address is caught.
func (d dialOptions) dialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	nd := &net.Dialer{Timeout: d.connectTimeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if deadline, ok := ctx.Value(connectDeadlineKey{}).(time.Time); ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline)
			defer cancel()
		}
		if d.unixSocket != "" {
			if d.hosts != nil {
				return nil, hostDeniedError(fmt.Errorf("unix socket %s: %w", d.unixSocket, netpolicy.ErrDenied))
			}
			conn, err := nd.DialContext(ctx, "unix", d.unixSocket)
			if err != nil {
				return nil, classifyDialErr(d.unixSocket, "", err)
			}
			return conn, nil
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host, port = addr, ""
		}
		if h, p, ok := matchConnectTo(d.connectTo, host, port); ok {
			host, port = h, p
		}
		dialNetwork := d.network
		if network == "udp" || network == "udp4" || network == "udp6" {
			dialNetwork = network
		}
		if addrs, ok := matchResolve(d.resolve, host, port); ok {
			var lastErr error
			for _, a := range addrs {
				if err := d.hosts.Allow(a, port); err != nil {
					return nil, hostDeniedError(err)
				}
				conn, err := nd.DialContext(ctx, dialNetwork, net.JoinHostPort(a, port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, classifyDialErr(host, port, lastErr)
		}
		if err := d.hosts.Allow(host, port); err != nil {
			return nil, hostDeniedError(err)
		}
		conn, err := nd.DialContext(ctx, dialNetwork, net.JoinHostPort(host, port))
		if err != nil {
			return nil, classifyDialErr(host, port, err)
		}
		return conn, nil
	}
}

type connectDeadlineKey struct{}

// withConnectDeadline bounds the connection of a request by deadline as
// well as by the connect timeout: what is left after a QUIC attempt.
func withConnectDeadline(ctx context.Context, deadline time.Time) context.Context {
	return context.WithValue(ctx, connectDeadlineKey{}, deadline)
}

// classifyDialErr turns a dial failure into a resolveError (DNS lookup
// failed) or a connectError (TCP connect failed), matching curl's codes
// (6) and (7).
func classifyDialErr(host, port string, err error) error {
	if err == nil {
		return nil
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return resolveError(host, err)
	}
	portNum, _ := strconv.Atoi(port)
	return connectError(host, portNum, err)
}
