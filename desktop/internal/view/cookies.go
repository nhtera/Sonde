// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"regexp"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/credential"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/redact"
	"github.com/nhtera/sonde/internal/report"
)

// Cookie values never reach the page, secret or not (as in history): a
// session cookie is a credential. A converter learns each value it sees
// (the Cookie header a request sends, the Set-Cookie of a response, the
// cookie store the verbose log prints) and masks it wherever it appears,
// like a secret; and the cookie headers and lists are masked by their
// structure, so a value too short to mask as text is masked there too.

// cookieValues is the cookie values seen so far in a unit.
type cookieValues struct {
	r *redact.Registry
	// inSection: the log is in a request's [Cookies] section.
	inSection bool
}

func newCookieValues() *cookieValues { return &cookieValues{r: redact.New()} }

// add learns value when it is long enough to mask as text; structural
// masking covers a shorter one in cookie headers, lists and log lines.
func (v *cookieValues) add(value string) {
	if credential.Maskable(value) {
		v.r.Add("cookie", value)
	}
}

// learnHeader learns the values of a Cookie or Set-Cookie header value.
func (v *cookieValues) learnHeader(name, value string) {
	switch strings.ToLower(name) {
	case "cookie":
		for _, part := range strings.Split(value, ";") {
			if _, val, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
				v.add(val)
			}
		}
	case "set-cookie":
		first, _, _ := strings.Cut(value, ";")
		if _, val, ok := strings.Cut(strings.TrimSpace(first), "="); ok {
			v.add(val)
		}
	}
}

// learnLine learns the values of a log line: a Cookie or Set-Cookie
// header line, a line of the cookie store (seven tab-separated fields,
// the value last), a name=value line of a request's [Cookies] section or
// the --cookie argument of the curl command. It reports whether the line
// is in a [Cookies] section.
func (v *cookieValues) learnLine(text string) bool {
	switch {
	case text == "[Cookies]":
		v.inSection = true
		return false
	case v.inSection && (text == "" || strings.HasPrefix(text, "[")):
		v.inSection = false
	case v.inSection:
		if _, val, ok := strings.Cut(text, "="); ok {
			v.add(val)
		}
		return true
	}
	if name, value, ok := strings.Cut(text, ":"); ok && !strings.ContainsAny(name, " \t") {
		v.learnHeader(name, strings.TrimSpace(value))
	}
	if f := strings.Split(text, "\t"); len(f) == 7 {
		v.add(f[6])
	}
	curlCookies(text, func(pair string) string {
		if _, val, ok := strings.Cut(pair, "="); ok {
			v.add(val)
		}
		return pair
	})
	return false
}

// curlCookie is a curl --cookie argument, quoted ('…' or $'…').
var curlCookie = regexp.MustCompile(`(--cookie|-b) ('[^']*'|\$'(?:[^'\\]|\\.)*')`)

// curlCookies calls f on each name=value pair of the --cookie arguments
// of a curl command and returns the command with each pair f returns;
// a cookie file argument (no pairs) is left as it is.
func curlCookies(text string, f func(pair string) string) string {
	return curlCookie.ReplaceAllStringFunc(text, func(m string) string {
		flag, arg, _ := strings.Cut(m, " ")
		open := "'"
		if strings.HasPrefix(arg, "$") {
			open = "$'"
		}
		pairs := strings.Split(arg[len(open):len(arg)-1], "; ")
		for i, p := range pairs {
			if strings.Contains(p, "=") {
				pairs[i] = f(p)
			}
		}
		return flag + " " + open + strings.Join(pairs, "; ") + "'"
	})
}

// learnRequest learns the Cookie header of a request.
func (v *cookieValues) learnRequest(req exchange.Request) {
	for _, h := range req.Headers {
		v.learnHeader(h.Name, h.Value)
	}
}

// learnEntry learns the cookies an attempt sent and received.
func (v *cookieValues) learnEntry(e *engine.EntryResult) {
	for _, call := range e.Calls {
		v.learnRequest(call.Request)
		if call.Response != nil {
			for _, c := range call.Response.Cookies() {
				v.add(c.Value)
			}
		}
	}
}

// wrap masks the values learned, after redact.
func (v *cookieValues) wrap(redact Redactor) Redactor {
	return func(s string) string { return v.r.Redact(redact(s)) }
}

// maskCookieHeader masks the value of each cookie of a Cookie header
// ("a=1; b=2") or of a Set-Cookie header's cookie (its attributes stay).
func maskCookieHeader(name, value string) string {
	switch strings.ToLower(name) {
	case "cookie":
		parts := strings.Split(value, ";")
		for i, part := range parts {
			if k, _, ok := strings.Cut(part, "="); ok {
				parts[i] = k + "=" + redact.Mask
			}
		}
		return strings.Join(parts, ";")
	case "set-cookie":
		first, rest, found := strings.Cut(value, ";")
		if k, _, ok := strings.Cut(first, "="); ok {
			first = k + "=" + redact.Mask
		}
		if found {
			return first + ";" + rest
		}
		return first
	}
	return value
}

// maskCookieLine masks a log line: a Cookie or Set-Cookie header line, a
// name=value line of a [Cookies] section (inSection) or the --cookie
// argument of the curl command.
func maskCookieLine(text string, inSection bool) string {
	if inSection {
		if k, _, ok := strings.Cut(text, "="); ok {
			return k + "=" + redact.Mask
		}
		return text
	}
	text = curlCookies(text, func(pair string) string {
		k, _, _ := strings.Cut(pair, "=")
		return k + "=" + redact.Mask
	})
	name, value, ok := strings.Cut(text, ":")
	if !ok || strings.ContainsAny(name, " \t") {
		return text
	}
	switch strings.ToLower(name) {
	case "cookie", "set-cookie":
		lead := value[:len(value)-len(strings.TrimLeft(value, " "))]
		return name + ":" + lead + maskCookieHeader(name, strings.TrimLeft(value, " "))
	}
	return text
}

// maskRequestCookies masks the cookie headers and cookies of a request.
func maskRequestCookies(r *report.Request) {
	for i := range r.Headers {
		r.Headers[i].Value = maskCookieHeader(r.Headers[i].Name, r.Headers[i].Value)
	}
	for i := range r.Cookies {
		r.Cookies[i].Value = redact.Mask
	}
}

// maskEntryCookies masks every cookie of an entry's calls.
func maskEntryCookies(e *report.Entry) {
	for i := range e.Calls {
		c := &e.Calls[i]
		maskRequestCookies(&c.Request)
		for j := range c.Response.Headers {
			c.Response.Headers[j].Value = maskCookieHeader(c.Response.Headers[j].Name, c.Response.Headers[j].Value)
		}
		for j := range c.Response.Cookies {
			c.Response.Cookies[j].Value = redact.Mask
		}
	}
}
