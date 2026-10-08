// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"golang.org/x/net/proxy"

	"github.com/nhtera/sonde/internal/netpolicy"
)

// noProxyMatch reports whether host is covered by a --noproxy list: a
// comma-separated list of domain suffixes (a leading "." is optional) or
// "*" for every host, matching curl's NO_PROXY semantics.
func noProxyMatch(host, noProxy string) bool {
	host = strings.ToLower(host)
	for entry := range strings.SplitSeq(noProxy, ",") {
		entry = strings.TrimSpace(strings.ToLower(entry))
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		entry = strings.TrimPrefix(entry, ".")
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}

// proxyFunc builds an http.Transport.Proxy function from --proxy/--noproxy,
// used only for http(s) proxies; socks5 is wired through DialContext
// instead since net/http has no native socks support.
func proxyFunc(proxyURL *url.URL, noProxy string) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		if noProxy != "" && noProxyMatch(req.URL.Hostname(), noProxy) {
			return nil, nil
		}
		return proxyURL, nil
	}
}

// httpProxyFor returns the HTTP proxy that receives a request to u itself
// (not a tunnel), nil for none: --proxy or the environment's proxy for u,
// when its scheme is http or https.
func httpProxyFor(opts *Options, u *url.URL) *url.URL {
	var p *url.URL
	if opts.Proxy != "" {
		parsed, err := parseProxyURL(opts.Proxy)
		if err != nil || (opts.NoProxy != "" && noProxyMatch(u.Hostname(), opts.NoProxy)) {
			return nil
		}
		p = parsed
	} else {
		p = environmentProxy(u, opts.NoProxy)
	}
	if p == nil || (p.Scheme != "http" && p.Scheme != "https") {
		return nil
	}
	return p
}

// environmentProxy returns the proxy the environment gives for u, nil
// for none or a value that cannot be used (environmentProxyErr reports
// why).
func environmentProxy(u *url.URL, noProxy string) *url.URL {
	p, _ := environmentProxyErr(u, noProxy)
	return p
}

// environmentProxyErr returns the proxy the environment gives for u, nil
// for none: http_proxy or https_proxy (or their uppercase forms), else
// all_proxy or ALL_PROXY, as libcurl reads them; no_proxy (NO_PROXY)
// applies to both, and hosts matching noProxy are never proxied. A value
// that does not parse, or a scheme sonde cannot speak (socks4), is an
// error rather than a direct or plain-HTTP connection.
func environmentProxyErr(u *url.URL, noProxy string) (*url.URL, error) {
	if noProxy != "" && noProxyMatch(u.Hostname(), noProxy) {
		return nil, nil
	}
	cfg := httpproxy.FromEnvironment()
	p, err := cfg.ProxyFunc()(u)
	if err == nil && p == nil {
		all := os.Getenv("all_proxy")
		if all == "" {
			all = os.Getenv("ALL_PROXY")
		}
		if all != "" {
			cfg.HTTPProxy, cfg.HTTPSProxy = all, all
			p, err = cfg.ProxyFunc()(u)
		}
	}
	if err != nil {
		return nil, invalidURLError("proxy from the environment", err.Error())
	}
	if p != nil {
		switch p.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, invalidURLError(p.Redacted(), "unsupported proxy scheme "+p.Scheme)
		}
	}
	return p, nil
}

// socks5DialContext wraps a SOCKS5 proxy as a DialContext for an
// http.Transport, so every connection (not only CONNECT tunnels) goes
// through the proxy. The proxy itself must be allowed by hosts.
func socks5DialContext(proxyURL *url.URL, noProxy string, next func(ctx context.Context, network, addr string) (net.Conn, error), hosts *netpolicy.Policy) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	var auth *proxy.Auth
	if u := proxyURL.User; u != nil {
		pass, _ := u.Password()
		auth = &proxy.Auth{User: u.Username(), Password: pass}
	}
	host := proxyURL.Host
	if proxyURL.Port() == "" {
		host = net.JoinHostPort(proxyURL.Hostname(), "1080")
	}
	forward := proxy.Dialer(proxy.Direct)
	if hosts != nil {
		forward = allowedDialer{hosts: hosts}
	}
	d, err := proxy.SOCKS5("tcp", host, auth, forward)
	if err != nil {
		return nil, otherError("could not configure SOCKS5 proxy", err)
	}
	ctxDialer, ok := d.(proxy.ContextDialer)
	if !ok {
		return nil, otherError("SOCKS5 proxy does not support context dialing", nil)
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if noProxy != "" {
			if h, _, err := net.SplitHostPort(addr); err == nil && noProxyMatch(h, noProxy) {
				return next(ctx, network, addr)
			}
		}
		return ctxDialer.DialContext(ctx, network, addr)
	}, nil
}

// parseProxyURL parses --proxy, adding a scheme (http://) when the value
// is a bare host[:port], the way curl accepts it.
func parseProxyURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, invalidURLError(raw, err.Error())
	}
	if u.Port() == "" {
		port := "1080"
		if u.Scheme == "http" || u.Scheme == "https" {
			port = "80"
		}
		u.Host = net.JoinHostPort(u.Hostname(), port)
	}
	return u, nil
}

// allowedDialer dials directly, like proxy.Direct, after checking the
// address against a host policy.
type allowedDialer struct{ hosts *netpolicy.Policy }

func (a allowedDialer) Dial(network, addr string) (net.Conn, error) {
	return a.DialContext(context.Background(), network, addr)
}

func (a allowedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if err := a.hosts.Allow(host, port); err != nil {
		return nil, hostDeniedError(err)
	}
	return proxy.Direct.DialContext(ctx, network, addr)
}
