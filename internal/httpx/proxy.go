// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"golang.org/x/net/proxy"
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

// environmentProxy returns the proxy the http_proxy, https_proxy and
// no_proxy environment variables (or their uppercase forms) give for u,
// nil for none; hosts matching noProxy are never proxied.
func environmentProxy(u *url.URL, noProxy string) *url.URL {
	if noProxy != "" && noProxyMatch(u.Hostname(), noProxy) {
		return nil
	}
	p, err := httpproxy.FromEnvironment().ProxyFunc()(u)
	if err != nil {
		return nil
	}
	return p
}

// socks5DialContext wraps a SOCKS5 proxy as a DialContext for an
// http.Transport, so every connection (not only CONNECT tunnels) goes
// through the proxy.
func socks5DialContext(proxyURL *url.URL, noProxy string, next func(ctx context.Context, network, addr string) (net.Conn, error)) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	var auth *proxy.Auth
	if u := proxyURL.User; u != nil {
		pass, _ := u.Password()
		auth = &proxy.Auth{User: u.Username(), Password: pass}
	}
	host := proxyURL.Host
	if proxyURL.Port() == "" {
		host = net.JoinHostPort(proxyURL.Hostname(), "1080")
	}
	d, err := proxy.SOCKS5("tcp", host, auth, proxy.Direct)
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
