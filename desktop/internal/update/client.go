// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// repoOwner and repoName are the GitHub repository that publishes desktop/v* releases.
const (
	repoOwner = "nhtera"
	repoName  = "Sonde"
)

// endpoints are where releases are listed and downloaded: GitHub, or a
// test release server (--update-api) that serves both.
type endpoints struct {
	api      *url.URL // https://api.github.com
	download *url.URL // https://github.com
	test     bool
}

func githubEndpoints() endpoints {
	return endpoints{
		api:      &url.URL{Scheme: "https", Host: "api.github.com"},
		download: &url.URL{Scheme: "https", Host: "github.com"},
	}
}

// testEndpoints is a test release server's base URL: https, or http on
// the loopback interface only.
func testEndpoints(raw string) (endpoints, error) {
	u, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return endpoints{}, fmt.Errorf("--update-api %q: a base URL such as https://host or http://127.0.0.1:PORT", raw)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback(u.Hostname())) {
		return endpoints{}, fmt.Errorf("--update-api %q: https, or http on 127.0.0.1 or localhost only", raw)
	}
	return endpoints{api: u, download: u, test: true}, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// at is a URL under base: base's path, then the escaped segments.
func at(base *url.URL, segments ...string) string {
	u := *base
	for _, s := range segments {
		u = *u.JoinPath(s)
	}
	return u.String()
}

// allowed reports whether a request (a redirect included) may go to u:
// GitHub's API, its site and its release asset hosts over https, or the
// test release server.
func (e endpoints) allowed(u *url.URL) bool {
	if e.test {
		return u.Scheme == e.api.Scheme && u.Host == e.api.Host
	}
	if u.Scheme != "https" {
		return false
	}
	h := u.Hostname()
	return h == "api.github.com" || h == "github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

// proxyURL reads the proxy setting as runs do: a bare host[:port] is an
// http:// proxy, the way curl takes it. Its error never repeats the value,
// which may hold a password.
func proxyURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errors.New("the proxy setting is not a proxy URL")
	}
	return u, nil
}

// newClient is the update checks' HTTP client: bounded connection, TLS and
// header waits (the caller's context bounds the whole request), the app's
// proxy setting or else the environment's, and requests and redirects to
// allowed hosts only.
func newClient(e endpoints, proxy func() string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: func(r *http.Request) (*url.URL, error) {
				if p := proxy(); p != "" {
					return proxyURL(p)
				}
				return http.ProxyFromEnvironment(r)
			},
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ForceAttemptHTTP2:     true,
		},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if !e.allowed(r.URL) {
				return fmt.Errorf("redirect to %s://%s refused", r.URL.Scheme, r.URL.Host)
			}
			return nil
		},
	}
}
