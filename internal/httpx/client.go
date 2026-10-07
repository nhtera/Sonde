// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"fmt"
	"os"
	"strings"
)

// warnOnce forwards msg to cfg.Warn the first time it is sent for this
// client, matching curl's "warn once" behavior for --insecure.
func (c *Client) warnOnce(msg string) {
	if c.warned[msg] {
		return
	}
	if c.warned == nil {
		c.warned = map[string]bool{}
	}
	c.warned[msg] = true
	if c.cfg.Warn != nil {
		c.cfg.Warn(msg)
	}
}

// transportFor returns the cached transport for opts and the host the
// server certificate must match, building and caching one if this is the
// first time these are seen.
func (c *Client) transportFor(opts *Options, tlsHost string) (*builtTransport, error) {
	key := transportCacheKey(opts) + ";host=" + tlsHost
	if t, ok := c.transports[key]; ok {
		return t, nil
	}
	warnCfg := c.cfg
	warnCfg.Warn = c.warnOnce
	t, err := buildTransport(opts, warnCfg, tlsHost)
	if err != nil {
		return nil, err
	}
	if c.transports == nil {
		c.transports = map[string]*builtTransport{}
	}
	c.transports[key] = t
	return t, nil
}

// netrcFor loads (and caches, per file) the netrc file requested by opts:
// a netrc-file option like any option file, the user's default netrc
// directly. It returns nil, nil when no netrc use was requested or the
// file is missing and optional.
func (c *Client) netrcFor(opts *Options) (*netrcFile, error) {
	if !opts.Netrc && !opts.NetrcOptional && opts.NetrcFile == "" {
		return nil, nil
	}
	path := opts.NetrcFile
	read := func(name string) ([]byte, error) { return readOptionFile(c.cfg.Sandbox, opts, name) }
	if path == "" {
		path = defaultNetrcPath()
		read = os.ReadFile
	}
	if path == "" {
		return nil, nil
	}
	if f, ok := c.netrc[path]; ok {
		return f, nil
	}
	f, err := loadNetrc(path, read)
	if err != nil {
		if opts.NetrcOptional {
			return nil, nil
		}
		return nil, fileAccessError(path, err)
	}
	if f == nil {
		f = &netrcFile{}
	}
	if c.netrc == nil {
		c.netrc = map[string]*netrcFile{}
	}
	c.netrc[path] = f
	return f, nil
}

// NewClient returns a client; it fails when CookieFile cannot be read.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Sandbox == nil {
		return nil, fmt.Errorf("httpx: ClientConfig.Sandbox is required")
	}
	jar, err := newCookieJar(cfg.CookieFile, cfg.NoCookieStore)
	if err != nil {
		return nil, err
	}
	if cfg.Version == "" {
		cfg.Version = "0.0.0"
	}
	return &Client{cfg: cfg, jar: jar}, nil
}

// Cookies returns the cookie store content, in insertion order.
func (c *Client) Cookies() []Cookie {
	cookies := c.jar.all()
	// A cookie valid for subdomains is written with a leading dot, as in
	// curl's cookie list.
	for i, ck := range cookies {
		if ck.IncludeSubdomain && !strings.HasPrefix(ck.Domain, ".") {
			cookies[i].Domain = "." + ck.Domain
		}
	}
	return cookies
}

// AddCookie adds a cookie written in Netscape format (the
// `@cookie_storage_set:` comment command).
func (c *Client) AddCookie(netscape string) error {
	cookie, ok := parseNetscapeCookie(netscape)
	if !ok {
		return fmt.Errorf("httpx: invalid Netscape cookie: %q", netscape)
	}
	c.jar.store(cookie)
	return nil
}

// ClearCookies empties the cookie store (`@cookie_storage_clear`).
func (c *Client) ClearCookies() {
	c.jar.clear()
}

// Close releases idle connections.
func (c *Client) Close() error {
	for _, t := range c.transports {
		t.closeIdle()
	}
	return nil
}
