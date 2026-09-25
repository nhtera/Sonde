// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"strconv"
	"strings"
)

// Cookie is a cookie set by a response (a Set-Cookie header), with its
// attributes as written.
type Cookie struct {
	Name       string
	Value      string
	Attributes []CookieAttribute
}

// CookieAttribute is a Set-Cookie attribute; flags such as Secure have no
// value.
type CookieAttribute struct {
	Name     string
	Value    string
	HasValue bool
}

// parseSetCookie parses a Set-Cookie header value. The name is the text
// before the first '=', the value runs to the first ';'; each attribute is
// `name` or `name=value` (a value stops at the next '='). Names and values
// are kept as written except that attribute names are trimmed.
func parseSetCookie(s string) (Cookie, bool) {
	name, rest, ok := strings.Cut(s, "=")
	if !ok {
		return Cookie{}, false
	}
	parts := strings.Split(rest, ";")
	c := Cookie{Name: name, Value: parts[0]}
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		fields := strings.Split(p, "=")
		a := CookieAttribute{Name: strings.TrimSpace(fields[0])}
		if len(fields) > 1 {
			a.Value, a.HasValue = fields[1], true
		}
		c.Attributes = append(c.Attributes, a)
	}
	return c, true
}

// Cookies returns the cookies of every Set-Cookie header, in order.
func (r *Response) Cookies() []Cookie {
	var cs []Cookie
	for _, v := range r.Headers.Values("Set-Cookie") {
		if c, ok := parseSetCookie(v); ok {
			cs = append(cs, c)
		}
	}
	return cs
}

// Cookie returns the first cookie named name.
func (r *Response) Cookie(name string) (Cookie, bool) {
	for _, c := range r.Cookies() {
		if c.Name == name {
			return c, true
		}
	}
	return Cookie{}, false
}

// Attr returns the value of the first attribute named name
// (case-insensitive); ok is false when there is none or it has no value.
func (c Cookie) Attr(name string) (value string, ok bool) {
	for _, a := range c.Attributes {
		if strings.EqualFold(a.Name, name) {
			return a.Value, a.HasValue
		}
	}
	return "", false
}

// Flag reports whether a value-less attribute named name is present.
func (c Cookie) Flag(name string) bool {
	for _, a := range c.Attributes {
		if strings.EqualFold(a.Name, name) && !a.HasValue {
			return true
		}
	}
	return false
}

// MaxAge returns the first Max-Age attribute that is a valid integer.
func (c Cookie) MaxAge() (int64, bool) {
	for _, a := range c.Attributes {
		if strings.EqualFold(a.Name, "Max-Age") && a.HasValue {
			if n, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}
