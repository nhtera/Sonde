// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http/httptrace"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/nhtera/sonde/internal/netpolicy"
)

// maxH3Handshake bounds the QUIC handshake before --http3 falls back to
// TCP: half the connect timeout, at most this (curl races the two; here
// TCP keeps at least half the connect timeout).
const maxH3Handshake = 2 * time.Second

// h3DialError is a QUIC connection that could not be established: --http3
// falls back to TCP after it.
type h3DialError struct{ err error }

func (e *h3DialError) Error() string { return "HTTP/3: " + e.err.Error() }
func (e *h3DialError) Unwrap() error { return e.err }

// newH3Transport returns the HTTP/3 transport of a set of options: QUIC
// over UDP sockets dialed with the dial options (connect-to, resolve, IP
// family, host policy) and the TLS configuration of the TCP transports.
func newH3Transport(d dialOptions, tlsConfig *tls.Config) *http3.Transport {
	handshake := min(d.connectTimeout/2, maxH3Handshake)
	return &http3.Transport{
		TLSClientConfig:    tlsConfig,
		DisableCompression: true,
		QUICConfig:         &quic.Config{HandshakeIdleTimeout: handshake},
		Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
			udpAddr, err := d.resolveUDP(ctx, addr)
			if err != nil {
				return nil, err // a policy denial or a resolution error: no fallback
			}
			pc, err := net.ListenUDP(d.udpNetwork(), nil)
			if err != nil {
				return nil, &h3DialError{err}
			}
			// The transport does not own a socket it is given: both are
			// closed together.
			tr := &quic.Transport{Conn: pc}
			release := func() { _ = tr.Close(); _ = pc.Close() }
			trace := httptrace.ContextClientTrace(ctx)
			if trace != nil && trace.ConnectStart != nil {
				trace.ConnectStart("udp", udpAddr.String())
			}
			if trace != nil && trace.TLSHandshakeStart != nil {
				trace.TLSHandshakeStart()
			}
			// QUIC connects and secures in one handshake: connect and TLS
			// end together.
			conn, err := tr.Dial(ctx, udpAddr, tlsCfg, cfg)
			var state tls.ConnectionState
			if conn != nil {
				state = conn.ConnectionState().TLS
			}
			if trace != nil && trace.ConnectDone != nil {
				trace.ConnectDone("udp", udpAddr.String(), err)
			}
			if trace != nil && trace.TLSHandshakeDone != nil {
				trace.TLSHandshakeDone(state, err)
			}
			if err != nil {
				release()
				if isCertVerifyError(err) {
					return nil, err // the server answered: a TLS failure is final
				}
				return nil, &h3DialError{err}
			}
			go func() {
				<-conn.Context().Done()
				release()
			}()
			return conn, nil
		},
	}
}

// udpNetwork is the UDP network of the IP family option.
func (d dialOptions) udpNetwork() string {
	switch d.network {
	case "tcp4":
		return "udp4"
	case "tcp6":
		return "udp6"
	}
	return "udp"
}

// resolveUDP returns the address a QUIC connection to addr is made to:
// connect-to and resolve rules apply, then name resolution in the IP
// family. With a host policy, the address must be allowed before any
// packet is sent; a unix socket cannot carry QUIC.
func (d dialOptions) resolveUDP(ctx context.Context, addr string) (*net.UDPAddr, error) {
	if d.unixSocket != "" {
		if d.hosts != nil {
			return nil, hostDeniedError(fmt.Errorf("unix socket %s: %w", d.unixSocket, netpolicy.ErrDenied))
		}
		return nil, &h3DialError{errors.New("a unix socket cannot carry QUIC")}
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, &h3DialError{err}
	}
	if h, p, ok := matchConnectTo(d.connectTo, host, port); ok {
		host, port = h, p
	}
	// The host policy as on the TCP path (dialContext): every address of
	// a resolve rule, else the host as written (a name, or an address).
	candidates, ok := matchResolve(d.resolve, host, port)
	if ok {
		for _, c := range candidates {
			if err := d.hosts.Allow(c, port); err != nil {
				return nil, hostDeniedError(err)
			}
		}
	} else {
		if err := d.hosts.Allow(host, port); err != nil {
			return nil, hostDeniedError(err)
		}
		if ip := net.ParseIP(host); ip != nil {
			candidates = []string{host}
		} else {
			trace := httptrace.ContextClientTrace(ctx)
			if trace != nil && trace.DNSStart != nil {
				trace.DNSStart(httptrace.DNSStartInfo{Host: host})
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, map[string]string{"udp": "ip", "udp4": "ip4", "udp6": "ip6"}[d.udpNetwork()], host)
			if trace != nil && trace.DNSDone != nil {
				trace.DNSDone(httptrace.DNSDoneInfo{Err: err})
			}
			if err != nil {
				return nil, resolveError(host, err)
			}
			for _, ip := range ips {
				candidates = append(candidates, ip.String())
			}
		}
	}
	if len(candidates) == 0 {
		return nil, resolveError(host, errors.New("no address"))
	}
	ua, err := net.ResolveUDPAddr(d.udpNetwork(), net.JoinHostPort(candidates[0], port))
	if err != nil {
		return nil, &h3DialError{err}
	}
	return ua, nil
}
