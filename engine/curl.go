// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/httpx"
)

// curlCommand renders the curl command line equivalent to a request,
// before it is sent: cookies are those of the store at that time.
func (u *unit) curlCommand(spec *httpx.RequestSpec, opts *httpx.Options, output *outputTarget) string {
	args := []string{"curl"}
	hasBody := len(spec.Multipart) > 0 || len(spec.Form) > 0 || len(spec.Body.Data) > 0
	args = append(args, methodArgs(spec.Method, hasBody, opts.FollowLocation)...)
	headers := append(append([]httpxHeader{}, headerList(spec)...), headerListFrom(opts)...)
	headers = slices.DeleteFunc(headers, func(h httpxHeader) bool {
		return slices.ContainsFunc(opts.NoHeaders, func(name string) bool { return strings.EqualFold(h.name, name) })
	})
	args = append(args, u.headerArgs(headers, spec)...)
	for _, name := range opts.NoHeaders {
		args = append(args, "--header", shellString(name+":"))
	}
	body, stdin := u.bodyArgs(spec)
	args = append(args, body...)
	args = append(args, u.cookieArgs(spec)...)
	args = append(args, u.optionArgs(opts, output)...)
	args = append(args, urlArgs(spec)...)
	if stdin != "" {
		return stdin + " | " + strings.Join(args, " ")
	}
	return strings.Join(args, " ")
}

type httpxHeader struct{ name, value string }

func headerList(spec *httpx.RequestSpec) []httpxHeader {
	hs := make([]httpxHeader, len(spec.Headers))
	for i, h := range spec.Headers {
		hs[i] = httpxHeader{h.Name, h.Value}
	}
	return hs
}

func headerListFrom(opts *httpx.Options) []httpxHeader {
	hs := make([]httpxHeader, len(opts.Headers))
	for i, h := range opts.Headers {
		hs[i] = httpxHeader{h.Name, h.Value}
	}
	return hs
}

func methodArgs(method string, hasBody, follow bool) []string {
	switch method {
	case "GET":
		if hasBody {
			return []string{"--request", "GET"}
		}
		return nil
	case "HEAD":
		return []string{"--head"}
	case "POST":
		switch {
		case hasBody:
			return nil
		case follow:
			return []string{"--data", "''"}
		}
		return []string{"--request", "POST"}
	}
	return []string{"--request", method}
}

func (u *unit) headerArgs(headers []httpxHeader, spec *httpx.RequestSpec) []string {
	var args []string
	explicit := false
	for _, h := range headers {
		if strings.EqualFold(h.name, "Content-Type") {
			explicit = true
		}
		if h.value == "" {
			args = append(args, "--header", shellString(h.name+";"))
		} else {
			args = append(args, "--header", shellString(h.name+": "+h.value))
		}
	}
	if explicit {
		return args
	}
	switch ct := spec.ImplicitContentType; {
	case ct != "":
		if ct != "application/x-www-form-urlencoded" && ct != "multipart/form-data" {
			args = append(args, "--header", shellString("Content-Type: "+ct))
		}
	case len(spec.Body.Data) > 0 && spec.Body.Kind == httpx.BodyBinary:
		args = append(args, "--header", "'Content-Type: application/octet-stream'")
	case len(spec.Body.Data) > 0:
		args = append(args, "--header", "'Content-Type:'")
	}
	return args
}

// bodyArgs returns the body options, and the command that pipes a body
// curl reads from its standard input ("" when none): a body with a NUL
// byte, which no shell string can hold.
func (u *unit) bodyArgs(spec *httpx.RequestSpec) (args []string, stdin string) {
	for _, p := range spec.Form {
		args = append(args, "--data", shellString(p.Name+"="+escapeURL(p.Value)))
	}
	for _, p := range spec.Multipart {
		if p.Param != nil {
			args = append(args, "--form", shellString(p.Param.Name+"="+p.Param.Value))
			continue
		}
		path := u.resolvedPath(p.File.Filename)
		args = append(args, "--form", shellString(fmt.Sprintf("%s=@%s;type=%s", p.File.Name, path, p.File.ContentType)))
	}
	if len(spec.Body.Data) == 0 {
		return args, ""
	}
	if spec.Body.Kind != httpx.BodyFile && slices.Contains(spec.Body.Data, 0) {
		return append(args, "--data-binary", "@-"), "printf '" + hexEscapes(spec.Body.Data) + "'"
	}
	switch spec.Body.Kind {
	case httpx.BodyFile:
		return append(args, "--data-binary", shellString("@"+u.resolvedPath(spec.Body.Filename))), ""
	case httpx.BodyBinary:
		return append(args, "--data", "$'"+hexEscapes(spec.Body.Data)+"'"), ""
	}
	return append(args, "--data", shellString(string(spec.Body.Data))), ""
}

// hexEscapes writes every byte of data as \xHH.
func hexEscapes(data []byte) string {
	var b strings.Builder
	for _, c := range data {
		fmt.Fprintf(&b, "\\x%02x", c)
	}
	return b.String()
}

// cookieArgs lists the entry's cookies then the stored cookies matching
// the URL.
func (u *unit) cookieArgs(spec *httpx.RequestSpec) []string {
	var pairs []string
	for _, c := range spec.Cookies {
		pairs = append(pairs, c.Name+"="+c.Value)
	}
	if target, err := url.Parse(spec.URL); err == nil {
		for _, c := range u.client.Cookies() {
			if c.Expires == 1 || !cookieMatches(c, target) {
				continue
			}
			pairs = append(pairs, c.Name+"="+c.Value)
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	return []string{"--cookie", shellString(strings.Join(pairs, "; "))}
}

// cookieMatches reports whether a stored cookie applies to a URL: domain
// (exact, or suffix when subdomains are included) and path prefix.
func cookieMatches(c httpx.Cookie, target *url.URL) bool {
	domain := strings.TrimPrefix(c.Domain, ".")
	host := target.Hostname()
	if c.IncludeSubdomain {
		if !strings.HasSuffix(host, domain) {
			return false
		}
	} else if host != domain {
		return false
	}
	path := target.Path
	if path == "" {
		path = "/"
	}
	return strings.HasPrefix(path, c.Path)
}

func (u *unit) optionArgs(o *httpx.Options, output *outputTarget) []string {
	var a []string
	add := func(v ...string) { a = append(a, v...) }
	if o.AWSSigV4 != "" {
		add("--aws-sigv4", shellArg(o.AWSSigV4))
	}
	if o.CACert != "" {
		add("--cacert", shellArg(o.CACert))
	}
	if o.ClientCert != "" {
		add("--cert", shellArg(o.ClientCert))
	}
	if o.ClientKey != "" {
		add("--key", shellArg(o.ClientKey))
	}
	if o.Compressed {
		add("--compressed")
	}
	if o.ConnectTimeout != 0 && o.ConnectTimeout != 300*time.Second {
		add("--connect-timeout", strconv.Itoa(int(o.ConnectTimeout/time.Second)))
	}
	for _, c := range o.ConnectTo {
		add("--connect-to", shellArg(c))
	}
	if u.runner.opt.CookieFile != "" {
		add("--cookie", shellArg(u.runner.opt.CookieFile))
	}
	if o.Digest {
		add("--digest")
	}
	switch o.HTTPVersion {
	case httpx.HTTP10:
		add("--http1.0")
	case httpx.HTTP11:
		add("--http1.1")
	case httpx.HTTP2:
		add("--http2")
	case httpx.HTTP2PriorKnowledge:
		add("--http2-prior-knowledge")
	case httpx.HTTP3:
		add("--http3")
	}
	if o.Insecure {
		add("--insecure")
	}
	switch o.IPResolve {
	case httpx.IPv4:
		add("--ipv4")
	case httpx.IPv6:
		add("--ipv6")
	}
	switch {
	case o.FollowLocation && o.LocationTrusted:
		add("--location-trusted")
	case o.FollowLocation:
		add("--location")
	}
	if o.MaxFilesize > 0 {
		add("--max-filesize", strconv.FormatInt(o.MaxFilesize, 10))
	}
	if o.MaxRecvSpeed > 0 {
		add("--limit-rate", strconv.FormatInt(o.MaxRecvSpeed, 10))
	}
	if o.MaxRedirects != 50 {
		add("--max-redirs", strconv.Itoa(o.MaxRedirects))
	}
	if o.Timeout != 0 && o.Timeout != 300*time.Second {
		add("--max-time", strconv.Itoa(int(o.Timeout/time.Second)))
	}
	if o.Negotiate {
		add("--negotiate")
	}
	if o.NetrcFile != "" {
		add("--netrc-file", shellString(o.NetrcFile))
	}
	if o.NetrcOptional {
		add("--netrc-optional")
	}
	if o.Netrc {
		add("--netrc")
	}
	if o.NTLM {
		add("--ntlm")
	}
	if o.PathAsIs {
		add("--path-as-is")
	}
	if o.PinnedPublicKey != "" {
		add("--pinnedpubkey", shellArg(o.PinnedPublicKey))
	}
	if o.Proxy != "" {
		add("--proxy", shellString(o.Proxy))
	}
	for _, h := range o.ProxyHeaders {
		if h.Value == "" {
			add("--proxy-header", shellString(h.Name+";"))
		} else {
			add("--proxy-header", shellString(h.Name+": "+h.Value))
		}
	}
	for _, r := range o.Resolve {
		add("--resolve", shellArg(r))
	}
	if o.UnixSocket != "" {
		add("--unix-socket", shellString(o.UnixSocket))
	}
	if o.User != "" {
		add("--user", shellString(o.User))
	}
	if o.UserAgent != "" {
		add("--user-agent", shellString(o.UserAgent))
	}
	switch {
	case output == nil:
	case output.name == "-":
		add("--output", "-")
	default:
		add("--output", shellArg(u.resolvedPath(output.name)))
	}
	return a
}

func urlArgs(spec *httpx.RequestSpec) []string {
	target := spec.URL
	if len(spec.Query) > 0 {
		params := make([]string, len(spec.Query))
		for i, p := range spec.Query {
			params[i] = p.Name + "=" + escapeURL(p.Value)
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + strings.Join(params, "&")
	}
	quoted := shellString(target)
	if strings.ContainsAny(quoted, "{}[]") {
		return []string{"--globoff", quoted}
	}
	return []string{quoted}
}

// escapeURL percent-encodes every byte except ASCII letters and digits.
func escapeURL(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// shellString quotes s for a POSIX shell, using $'…' when it contains a
// newline, a tab or a quote.
func shellString(s string) string {
	if !strings.ContainsAny(s, "\n\t'") {
		return "'" + s + "'"
	}
	r := strings.NewReplacer("\n", `\n`, "\t", `\t`, "'", `\'`, `\`, `\\`)
	return "$'" + r.Replace(s) + "'"
}

// shellArg returns s as is when the shell reads it literally, else quoted
// by shellString.
func shellArg(s string) string {
	for _, c := range []byte(s) {
		plain := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("_@%+=:,./-", c) >= 0
		if !plain {
			return shellString(s)
		}
	}
	if s == "" {
		return "''"
	}
	return s
}

// resolvedPath joins a request-file path to the file root as given,
// without normalizing it.
func (u *unit) resolvedPath(name string) string {
	if filepath.IsAbs(name) || u.rootDir == "" || u.rootDir == "." {
		return name
	}
	return strings.TrimSuffix(u.rootDir, string(filepath.Separator)) + string(filepath.Separator) + name
}
