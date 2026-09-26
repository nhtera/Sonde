// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/nhtera/sonde/exchange"
)

// cookieJar is a per-client cookie store, matching the domain, path,
// secure and expiry semantics of a browser or curl's cookie engine.
// Cookies are kept in insertion order; updating an existing cookie (same
// domain, path and name) replaces it in place rather than moving it.
type cookieJar struct {
	cookies []Cookie
	disable bool
}

// newCookieJar builds a jar, optionally seeded from a Netscape cookie file.
// cookieFile is a command line path and is read directly, not through a
// sandbox.
func newCookieJar(cookieFile string, noStore bool) (*cookieJar, error) {
	j := &cookieJar{disable: noStore}
	if cookieFile == "" {
		return j, nil
	}
	data, err := os.ReadFile(cookieFile)
	if err != nil {
		return nil, fileAccessError(cookieFile, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || (strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#HttpOnly_")) {
			continue
		}
		c, ok := parseNetscapeCookie(line)
		if !ok {
			continue
		}
		j.store(c)
	}
	return j, nil
}

// store inserts c, replacing an existing cookie with the same domain, path
// and name in place.
func (j *cookieJar) store(c Cookie) {
	for i, e := range j.cookies {
		if strings.EqualFold(e.Domain, c.Domain) && e.Path == c.Path && e.Name == c.Name {
			j.cookies[i] = c
			return
		}
	}
	j.cookies = append(j.cookies, c)
}

// remove deletes the cookie with the given domain, path and name, if any.
func (j *cookieJar) remove(domain, path, name string) {
	for i, e := range j.cookies {
		if strings.EqualFold(e.Domain, domain) && e.Path == path && e.Name == name {
			j.cookies = append(j.cookies[:i], j.cookies[i+1:]...)
			return
		}
	}
}

// all returns the stored cookies in insertion order.
func (j *cookieJar) all() []Cookie {
	out := make([]Cookie, len(j.cookies))
	copy(out, j.cookies)
	return out
}

// clear empties the jar.
func (j *cookieJar) clear() { j.cookies = nil }

// forRequest returns the cookies that apply to u, ordered by path length
// (longest first, as curl sends them) and then insertion order.
func (j *cookieJar) forRequest(u *url.URL, now time.Time) []Cookie {
	if j.disable {
		return nil
	}
	host := cookieHost(u)
	secure := u.Scheme == "https"
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	var matched []Cookie
	for _, c := range j.cookies {
		if isExpired(c, now) {
			continue
		}
		if !domainMatch(host, c.Domain, c.IncludeSubdomain) {
			continue
		}
		if !pathMatch(path, c.Path) {
			continue
		}
		if c.HTTPS && !secure {
			continue
		}
		matched = append(matched, c)
	}
	// curl's order: longer path, longer domain, longer name, then the most
	// recently stored cookie first.
	for i, k := 0, len(matched)-1; i < k; i, k = i+1, k-1 {
		matched[i], matched[k] = matched[k], matched[i]
	}
	sort.SliceStable(matched, func(i, k int) bool {
		a, b := matched[i], matched[k]
		switch {
		case len(a.Path) != len(b.Path):
			return len(a.Path) > len(b.Path)
		case len(a.Domain) != len(b.Domain):
			return len(a.Domain) > len(b.Domain)
		}
		return len(a.Name) > len(b.Name)
	})
	return matched
}

// cookieHeader formats cookies as a Cookie header value.
func cookieHeader(cookies []Cookie) string {
	parts := make([]string, len(cookies))
	for i, c := range cookies {
		parts[i] = c.Name + "=" + c.Value
	}
	return strings.Join(parts, "; ")
}

// updateFromResponse stores every Set-Cookie header of resp, resolving
// missing Domain/Path attributes against reqURL and rejecting cookies whose
// Domain attribute does not domain-match the request host or names a
// public suffix (RFC 6265 §5.3).
func (j *cookieJar) updateFromResponse(reqURL *url.URL, resp *exchange.Response, now time.Time) {
	if j.disable {
		return
	}
	host := cookieHost(reqURL)
	for _, sc := range resp.Cookies() {
		c := Cookie{Name: sc.Name, Value: sc.Value}

		domain := host
		if v, ok := sc.Attr("domain"); ok && v != "" {
			v = strings.ToLower(strings.TrimPrefix(v, "."))
			if !domainMatch(host, v, true) {
				continue // cookie tries to set a domain outside the request host
			}
			if ps, icann := publicsuffix.PublicSuffix(v); icann && ps == v {
				continue // rejects cookies scoped to an entire public suffix
			}
			domain = v
			c.IncludeSubdomain = true
		}
		c.Domain = domain

		if v, ok := sc.Attr("path"); ok && strings.HasPrefix(v, "/") {
			c.Path = v
		} else {
			c.Path = defaultCookiePath(reqURL.EscapedPath())
		}

		c.HTTPS = sc.Flag("secure")
		c.HTTPOnly = sc.Flag("httponly")

		if maxAge, ok := sc.MaxAge(); ok {
			if maxAge <= 0 {
				j.remove(c.Domain, c.Path, c.Name)
				continue
			}
			c.Expires = now.Add(time.Duration(maxAge) * time.Second).Unix()
		} else if v, ok := sc.Attr("expires"); ok {
			if t, ok := parseCookieDate(v); ok {
				if !t.After(now) {
					j.remove(c.Domain, c.Path, c.Name)
					continue
				}
				c.Expires = t.Unix()
			}
		}
		j.store(c)
	}
}

// domainMatch reports whether host is covered by cookieDomain: an exact
// match when includeSubdomain is false, or host equal to or a subdomain of
// cookieDomain (with a "." boundary) when true.
func domainMatch(host, cookieDomain string, includeSubdomain bool) bool {
	host = strings.ToLower(host)
	cookieDomain = strings.ToLower(strings.TrimPrefix(cookieDomain, "."))
	if host == cookieDomain {
		return true
	}
	if !includeSubdomain {
		return false
	}
	return strings.HasSuffix(host, "."+cookieDomain)
}

// pathMatch implements the RFC 6265 §5.1.4 path-match algorithm.
func pathMatch(requestPath, cookiePath string) bool {
	if cookiePath == "" || cookiePath == "/" {
		return true
	}
	if requestPath == cookiePath {
		return true
	}
	if strings.HasPrefix(requestPath, cookiePath) {
		if strings.HasSuffix(cookiePath, "/") {
			return true
		}
		return strings.HasPrefix(requestPath[len(cookiePath):], "/")
	}
	return false
}

// defaultCookiePath implements the RFC 6265 §5.1.4 default-path algorithm.
func defaultCookiePath(requestPath string) string {
	if requestPath == "" || requestPath[0] != '/' {
		return "/"
	}
	i := strings.LastIndex(requestPath, "/")
	if i <= 0 {
		return "/"
	}
	return requestPath[:i]
}

// cookieHost returns the lowercase host of u, without the port.
func cookieHost(u *url.URL) string {
	return strings.ToLower(u.Hostname())
}

// isExpired reports whether c has expired at now. Expires == 0 is a
// session cookie (never expires while the client is alive); Expires == 1
// is the libcurl convention for an already-expired cookie.
func isExpired(c Cookie, now time.Time) bool {
	if c.Expires == 0 {
		return false
	}
	if c.Expires == 1 {
		return true
	}
	return c.Expires <= now.Unix()
}

// cookieDateLayouts covers the formats RFC 6265 asks servers to avoid but
// browsers still accept, plus the RFC 1123 format servers should send.
var cookieDateLayouts = []string{
	time.RFC1123,
	time.RFC1123Z,
	"Mon, 02-Jan-2006 15:04:05 MST",
	"Monday, 02-Jan-06 15:04:05 MST",
	time.ANSIC,
	time.RFC850,
}

func parseCookieDate(s string) (time.Time, bool) {
	for _, layout := range cookieDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseNetscapeCookie parses one line of a Netscape cookie file: fields
// separated by tabs (or, for inline use, spaces), in the order Domain,
// IncludeSubdomain, Path, HTTPS, Expires, Name, Value; an optional
// "#HttpOnly_" prefix on Domain marks an HttpOnly cookie.
func parseNetscapeCookie(line string) (Cookie, bool) {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return Cookie{}, false
	}
	var c Cookie
	domain := fields[0]
	if v, ok := strings.CutPrefix(domain, "#HttpOnly_"); ok {
		c.HTTPOnly = true
		domain = v
	}
	c.Domain = domain
	c.IncludeSubdomain = fields[1] == "TRUE"
	c.Path = fields[2]
	c.HTTPS = fields[3] == "TRUE"
	expires, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return Cookie{}, false
	}
	c.Expires = expires
	c.Name = fields[5]
	if len(fields) > 6 {
		c.Value = strings.Join(fields[6:], " ")
	}
	return c, true
}

// formatNetscapeCookie writes c in Netscape cookie file format.
func formatNetscapeCookie(c Cookie) string {
	prefix := ""
	if c.HTTPOnly {
		prefix = "#HttpOnly_"
	}
	subdomain := "FALSE"
	if c.IncludeSubdomain {
		subdomain = "TRUE"
	}
	https := "FALSE"
	if c.HTTPS {
		https = "TRUE"
	}
	return fmt.Sprintf("%s%s\t%s\t%s\t%s\t%d\t%s\t%s",
		prefix, c.Domain, subdomain, c.Path, https, c.Expires, c.Name, c.Value)
}
