// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package netpolicy decides which hosts a run may contact. `sonde mcp`
// builds a Policy from its --allow-host patterns; the HTTP client checks
// both the host of every request URL and the address of every connection
// it opens against the same Policy, with the same normalization, so what a
// request names and where its bytes go cannot disagree.
package netpolicy

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// ErrDenied reports a host outside the allowlist.
var ErrDenied = errors.New("not in the host allowlist")

// Policy is a parsed allowlist. A nil *Policy allows everything.
type Policy struct {
	all   bool
	rules []rule
}

// rule is one pattern. Exactly one of name, suffix, addr and prefix is
// set; port is empty for any port.
type rule struct {
	name   string       // exact host name
	suffix string       // "*.example.com" as ".example.com"
	addr   netip.Addr   // IP literal
	prefix netip.Prefix // CIDR
	port   int          // 0: any port
}

// Parse builds a Policy from patterns:
//
//	api.example.com        that name, any port
//	api.example.com:8443   that name and port
//	*.example.com          any subdomain of example.com (not example.com)
//	10.1.2.3, [::1]:8080   that address (port optional; IPv6 in brackets with a port)
//	10.0.0.0/8, fd00::/8   an address in that range
//	*                      every host
//
// Names are case-insensitive and a trailing dot is ignored. Names are
// never resolved: an address or range pattern matches only hosts written
// as addresses.
func Parse(patterns []string) (*Policy, error) {
	p := &Policy{}
	for _, pat := range patterns {
		if pat == "*" {
			p.all = true
			continue
		}
		r, err := parseRule(pat)
		if err != nil {
			return nil, fmt.Errorf("invalid host pattern %q: %w", pat, err)
		}
		p.rules = append(p.rules, r)
	}
	return p, nil
}

func parseRule(pat string) (rule, error) {
	if pat == "" {
		return rule{}, errors.New("empty")
	}
	if strings.ContainsAny(pat, "/") {
		prefix, err := netip.ParsePrefix(pat)
		if err != nil {
			return rule{}, errors.New("not an address range")
		}
		return rule{prefix: prefix.Masked()}, nil
	}
	if strings.Contains(pat, "://") {
		return rule{}, errors.New("a pattern has no scheme")
	}
	host, port := pat, 0
	if a, err := netip.ParseAddr(strings.Trim(pat, "[]")); err == nil && strings.Count(pat, ":") > 1 && !strings.HasPrefix(pat, "[") {
		// A bare IPv6 address has no port.
		return rule{addr: a.Unmap().WithZone("")}, nil
	}
	if h, pt, err := net.SplitHostPort(pat); err == nil {
		n, err := strconv.Atoi(pt)
		if err != nil || n < 1 || n > 65535 {
			return rule{}, errors.New("invalid port")
		}
		host, port = h, n
	} else if strings.HasPrefix(pat, "[") {
		if !strings.HasSuffix(pat, "]") {
			return rule{}, errors.New("unbalanced brackets")
		}
		host = pat[1 : len(pat)-1]
	}
	if strings.HasPrefix(host, "*.") {
		name, err := normalizeName(host[2:])
		if err != nil {
			return rule{}, err
		}
		return rule{suffix: "." + name, port: port}, nil
	}
	h, err := Normalize(host)
	if err != nil {
		return rule{}, err
	}
	if a, err := netip.ParseAddr(h); err == nil {
		return rule{addr: a, port: port}, nil
	}
	return rule{name: h, port: port}, nil
}

// Normalize returns the canonical form of a host as written in a URL or
// a dial address: lowercase, without a trailing dot, IPv6 brackets or
// zone, and an address in its canonical text. Only plain ASCII names
// (letters, digits, '-' and '_' in dot-separated labels) are accepted: a
// resolver may read anything else differently than this package does
// (a '%' is cut here but not by the system resolver, a Unicode name is
// converted to punycode before it is dialed). A host that looks like a
// short or hexadecimal IPv4 form (127.1, 0x7f000001), which some
// resolvers read as an address, is an error too.
func Normalize(host string) (string, error) {
	h := strings.Trim(host, "[]")
	if a, err := netip.ParseAddr(h); err == nil {
		// A zone is valid on an IPv6 address only.
		return a.WithZone("").Unmap().String(), nil
	}
	return normalizeName(h)
}

func normalizeName(h string) (string, error) {
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	if h == "" || len(h) > 253 {
		return "", fmt.Errorf("invalid host %q", h)
	}
	for label := range strings.SplitSeq(h, ".") {
		if label == "" || len(label) > 63 {
			return "", fmt.Errorf("invalid host %q", h)
		}
		for _, c := range []byte(label) {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return "", fmt.Errorf("invalid host %q", h)
			}
		}
	}
	if numericHost(h) {
		return "", fmt.Errorf("ambiguous address %q", h)
	}
	return h, nil
}

// numericHost reports whether every label of h is a decimal or 0x
// hexadecimal number: a host some resolvers turn into an IPv4 address.
func numericHost(h string) bool {
	for label := range strings.SplitSeq(h, ".") {
		digits := label
		if l := strings.ToLower(label); strings.HasPrefix(l, "0x") {
			digits = l[2:]
			if digits == "" {
				return false
			}
			if strings.Trim(digits, "0123456789abcdef") != "" {
				return false
			}
			continue
		}
		if digits == "" || strings.Trim(digits, "0123456789") != "" {
			return false
		}
	}
	return true
}

// Allow reports whether host (as written in a URL or dial address) and
// port may be contacted. The error wraps ErrDenied.
func (p *Policy) Allow(host, port string) error {
	if p == nil || p.all {
		return nil
	}
	h, err := Normalize(host)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrDenied, err)
	}
	a, perr := netip.ParseAddr(h)
	isAddr := perr == nil
	portNum, _ := strconv.Atoi(port)
	for _, r := range p.rules {
		if r.port != 0 && r.port != portNum {
			continue
		}
		switch {
		case r.name != "":
			if r.name == h {
				return nil
			}
		case r.suffix != "":
			if !isAddr && strings.HasSuffix(h, r.suffix) {
				return nil
			}
		case r.addr.IsValid():
			if isAddr && r.addr == a {
				return nil
			}
		default:
			if isAddr && r.prefix.Contains(a) {
				return nil
			}
		}
	}
	return fmt.Errorf("%s: %w", net.JoinHostPort(h, port), ErrDenied)
}

// AllowsAll reports whether the policy is `*` (or nil).
func (p *Policy) AllowsAll() bool { return p == nil || p.all }
