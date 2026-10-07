// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// entryOptions are the effective options of one entry.
type entryOptions struct {
	http          httpx.Options
	delay         time.Duration
	retry         int // 0: none, -1: unlimited
	retryInterval time.Duration
	repeat        *int // nil: once, -1: forever
	skip          bool
	output        *outputTarget
	stream        streamOptions
	// failWithBody writes the body of a failed attempt that is not
	// retried; noJSONPathCoercion keeps jsonpath results lists.
	failWithBody       bool
	noJSONPathCoercion bool
}

// outputTarget is the `output` option: a file or "-" (standard output).
type outputTarget struct {
	name string
	span syntax.Span
}

// baseHTTPOptions converts the run options to transport options.
func (r *Runner) baseHTTPOptions() httpx.Options {
	h := r.opt.HTTP
	o := httpx.Options{
		AWSSigV4: h.AWSSigV4, CACert: h.CACert, ClientCert: h.ClientCert, ClientKey: h.ClientKey,
		Compressed: h.Compressed, ConnectTimeout: h.ConnectTimeout, ConnectTo: h.ConnectTo,
		Digest: h.Digest, FollowLocation: h.FollowLocation || h.LocationTrusted, LocationTrusted: h.LocationTrusted,
		HTTPVersion: httpx.HTTPVersion(h.HTTPVersion), Insecure: h.Insecure, IPResolve: httpx.IPResolve(h.IPResolve),
		MaxFilesize: h.MaxFilesize, MaxRecvSpeed: h.LimitRate, MaxSendSpeed: h.LimitRate,
		MaxRedirects: h.MaxRedirects, Negotiate: h.Negotiate, Netrc: h.Netrc, NetrcFile: h.NetrcFile,
		NetrcOptional: h.NetrcOptional, NetrcAllowReroute: h.NetrcAllowReroute,
		NoHeaders: append([]string(nil), h.NoHeaders...), NoProxy: h.NoProxy, NTLM: h.NTLM,
		PathAsIs: h.PathAsIs, PinnedPublicKey: h.PinnedPublicKey, Proxy: h.Proxy, Resolve: h.Resolve,
		Timeout: h.Timeout, UnixSocket: h.UnixSocket, User: h.User, UserAgent: h.UserAgent,
	}
	if o.MaxRedirects == 0 {
		o.MaxRedirects = 50
	}
	// Command line paths are trusted: they are read as given, not confined
	// to the file root.
	o.LocalFiles = map[string]bool{}
	for _, name := range []string{h.CACert, h.ClientCert, h.ClientKey, h.UnixSocket, h.NetrcFile} {
		if name != "" {
			o.LocalFiles[name] = true
		}
	}
	for part := range strings.SplitSeq(h.PinnedPublicKey, ";") {
		if part != "" && !strings.HasPrefix(part, "sha256//") {
			o.LocalFiles[part] = true
		}
	}
	for _, line := range h.Headers {
		if hd, ok := parseHeader(line); ok {
			o.Headers = append(o.Headers, hd)
		}
	}
	for _, line := range h.ProxyHeaders {
		if hd, ok := parseHeader(line); ok {
			o.ProxyHeaders = append(o.ProxyHeaders, hd)
		}
	}
	return o
}

// parseHeader parses "Name: value".
func parseHeader(line string) (exchange.Header, bool) {
	name, v, ok := strings.Cut(line, ":")
	if !ok {
		return exchange.Header{}, false
	}
	return exchange.Header{Name: strings.TrimSpace(name), Value: strings.TrimSpace(v)}, true
}

// options returns the [Options] of an entry.
func options(e *syntax.Entry) []*syntax.Option {
	var opts []*syntax.Option
	for _, s := range e.Request.Sections {
		if s.Kind == syntax.SectionOptions {
			opts = append(opts, s.Options...)
		}
	}
	return opts
}

// entryVerbosity is the verbosity of an entry: the run verbosity unless
// its [Options] set `verbose`, `very-verbose` or `verbosity`.
func (u *unit) entryVerbosity(e *syntax.Entry) Verbosity {
	v := u.runner.opt.Verbosity
	for _, o := range options(e) {
		switch o.Name {
		case "verbosity":
			if id, ok := o.Value.(*syntax.Identifier); ok {
				switch id.Value {
				case "brief":
					v = Brief
				case "verbose":
					v = Verbose
				case "debug":
					v = VeryVerbose
				}
			}
		case "verbose", "very-verbose":
			b, err := u.boolOption(o)
			if err != nil {
				return u.runner.opt.Verbosity
			}
			switch {
			case !b:
				v = Quiet
			case o.Name == "verbose":
				v = Verbose
			default:
				v = VeryVerbose
			}
		}
	}
	return v
}

// entryOptions evaluates the [Options] of an entry on top of the run
// options. Variables defined there stay defined for the next entries.
func (u *unit) entryOptions(e *syntax.Entry) (*entryOptions, error) {
	opt := u.runner.opt
	eo := &entryOptions{
		http:               u.runner.baseHTTPOptions(),
		delay:              opt.Delay,
		retry:              opt.Retry,
		retryInterval:      opt.RetryInterval,
		failWithBody:       opt.FailWithBody,
		noJSONPathCoercion: opt.NoJSONPathCoercion,
	}
	if eo.retryInterval == 0 {
		eo.retryInterval = time.Second
	}
	opts := options(e)
	if len(opts) == 0 {
		return eo, nil
	}
	u.debug("")
	u.debugImportant("Entry options:")
	h := &eo.http
	for _, o := range opts {
		if u.runner.hosts != nil && refusedOptions[o.Name] {
			return nil, refusedOption(e, o)
		}
		var err error
		switch o.Name {
		case "aws-sigv4":
			h.AWSSigV4, err = u.stringOption(o)
		case "cacert":
			h.CACert, err = u.stringOption(o)
		case "cert":
			h.ClientCert, err = u.stringOption(o)
		case "key":
			h.ClientKey, err = u.stringOption(o)
		case "compressed":
			h.Compressed, err = u.boolOption(o)
		case "connect-to":
			var s string
			s, err = u.stringOption(o)
			h.ConnectTo = append(h.ConnectTo, s)
		case "connect-timeout":
			h.ConnectTimeout, err = u.durationOption(o)
		case "delay":
			eo.delay, err = u.durationOption(o)
		case "digest":
			h.Digest, err = u.boolOption(o)
		case "fail-with-body":
			eo.failWithBody, err = u.boolOption(o)
		case "header":
			err = u.headerOption(o, h)
		case "http1.0", "http1.1", "http2", "http2-prior-knowledge", "http3":
			err = u.versionOption(o, h)
		case "location":
			var b bool
			b, err = u.boolOption(o)
			h.FollowLocation, h.LocationTrusted = b, false
		case "location-trusted":
			var b bool
			if b, err = u.boolOption(o); b {
				h.FollowLocation, h.LocationTrusted = true, true
			}
		case "insecure":
			h.Insecure, err = u.boolOption(o)
		case "ipv4", "ipv6":
			var b bool
			b, err = u.boolOption(o)
			if (o.Name == "ipv4") == b {
				h.IPResolve = httpx.IPv4
			} else {
				h.IPResolve = httpx.IPv6
			}
		case "limit-rate":
			var n int64
			n, err = u.naturalOption(o)
			h.MaxSendSpeed, h.MaxRecvSpeed = n, n
		case "max-redirs":
			var n int
			n, err = u.countOption(o)
			h.MaxRedirects = n
		case "max-time":
			h.Timeout, err = u.durationOption(o)
		case "negotiate":
			h.Negotiate, err = u.boolOption(o)
		case "netrc":
			h.Netrc, err = u.boolOption(o)
		case "netrc-file":
			h.NetrcFile, err = u.stringOption(o)
		case "netrc-optional":
			h.NetrcOptional, err = u.boolOption(o)
		case "no-header":
			var s string
			s, err = u.stringOption(o)
			h.NoHeaders = append(h.NoHeaders, s)
		case "no-jsonpath-coercion":
			eo.noJSONPathCoercion, err = u.boolOption(o)
		case "ntlm":
			h.NTLM, err = u.boolOption(o)
		case "output":
			var s string
			s, err = u.stringOption(o)
			eo.output = &outputTarget{name: s, span: o.Value.(*syntax.Template).Span}
		case "path-as-is":
			h.PathAsIs, err = u.boolOption(o)
		case "pinnedpubkey":
			h.PinnedPublicKey, err = u.stringOption(o)
		case "proxy":
			h.Proxy, err = u.stringOption(o)
		case "repeat":
			var n int
			n, err = u.countOption(o)
			eo.repeat = &n
		case "resolve":
			var s string
			s, err = u.stringOption(o)
			h.Resolve = append(h.Resolve, s)
		case "retry":
			eo.retry, err = u.countOption(o)
		case "retry-interval":
			eo.retryInterval, err = u.durationOption(o)
		case "skip":
			eo.skip, err = u.boolOption(o)
		case "unix-socket":
			h.UnixSocket, err = u.stringOption(o)
		case "user":
			h.User, err = u.stringOption(o)
		case "variable":
			err = u.variableOption(o)
		case "verbose", "very-verbose":
			_, err = u.boolOption(o)
		case "sonde-stream-count", "sonde-stream-max-bytes", "sonde-stream-timeout":
			err = u.streamOption(o, &eo.stream)
		}
		if err != nil {
			return nil, err
		}
		u.debug(optionText(o))
	}
	return eo, nil
}

// refusedOptions are the entry options a run with a host allowlist
// (`sonde mcp`) refuses: they send a connection somewhere else than the
// host the URL names, send stored netrc credentials, or write a file.
var refusedOptions = map[string]bool{
	"connect-to":     true,
	"netrc":          true,
	"netrc-file":     true,
	"netrc-optional": true,
	"output":         true,
	"proxy":          true,
	"resolve":        true,
	"unix-socket":    true,
}

// refusedOption reports a refused option at the [Options] section of e.
func refusedOption(e *syntax.Entry, o *syntax.Option) *runerr.Error {
	span := e.Request.Span
	for _, s := range e.Request.Sections {
		if s.Kind == syntax.SectionOptions {
			span = s.Span
		}
	}
	re := runerr.New(span, runerr.HTTP, false)
	re.Value = "Option not allowed"
	re.Reason = fmt.Sprintf("option %q is not available with a host allowlist (sonde mcp)", o.Name)
	return re
}

// optionText is the option as written, for verbose output.
func optionText(o *syntax.Option) string {
	return o.Name + ": " + nodeText(o.Value)
}

func nodeText(n syntax.Node) string {
	switch n := n.(type) {
	case *syntax.Template:
		return templateText(n)
	case *syntax.Boolean:
		if n.Value {
			return "true"
		}
		return "false"
	case *syntax.Number:
		return n.Source
	case *syntax.Duration:
		return n.Value.Source + n.Unit
	case *syntax.Identifier:
		return n.Value
	case *syntax.Placeholder:
		return "{{" + n.Expr.Name + "}}"
	case *syntax.VariableDefinition:
		return n.Name + "=" + nodeText(n.Value)
	case *syntax.Null:
		return "null"
	}
	return ""
}

func templateText(t *syntax.Template) string {
	var b strings.Builder
	for _, el := range t.Elements {
		switch el := el.(type) {
		case *syntax.TemplateString:
			b.WriteString(el.Value)
		case *syntax.Placeholder:
			b.WriteString("{{" + el.Expr.Name + "}}")
		}
	}
	return b.String()
}

func (u *unit) stringOption(o *syntax.Option) (string, error) {
	return u.env.Render(o.Value.(*syntax.Template))
}

func (u *unit) boolOption(o *syntax.Option) (bool, error) {
	switch v := o.Value.(type) {
	case *syntax.Boolean:
		return v.Value, nil
	case *syntax.Placeholder:
		x, err := u.env.Eval(v.Expr)
		if err != nil {
			return false, err
		}
		if b, ok := x.(value.Bool); ok {
			return bool(b), nil
		}
		return false, invalidType(v.Expr.Span, value.Repr(x), "boolean")
	}
	return false, nil
}

func (u *unit) naturalOption(o *syntax.Option) (int64, error) {
	switch v := o.Value.(type) {
	case *syntax.Number:
		return v.Int, nil
	case *syntax.Placeholder:
		x, err := u.env.Eval(v.Expr)
		if err != nil {
			return 0, err
		}
		i, ok := x.(value.Int)
		if !ok {
			return 0, invalidType(v.Expr.Span, value.Repr(x), "integer")
		}
		if i <= 0 {
			return 0, invalidType(v.Expr.Span, value.Repr(x), "integer > 0")
		}
		return int64(i), nil
	}
	return 0, nil
}

// countOption returns a count: -1 means unlimited.
func (u *unit) countOption(o *syntax.Option) (int, error) {
	switch v := o.Value.(type) {
	case *syntax.Number:
		return int(v.Int), nil
	case *syntax.Placeholder:
		x, err := u.env.Eval(v.Expr)
		if err != nil {
			return 0, err
		}
		i, ok := x.(value.Int)
		if !ok {
			return 0, invalidType(v.Expr.Span, value.Repr(x), "integer")
		}
		if i < -1 {
			return 0, invalidType(v.Expr.Span, value.Repr(x), "integer >= -1")
		}
		return int(i), nil
	}
	return 0, nil
}

func (u *unit) durationOption(o *syntax.Option) (time.Duration, error) {
	switch v := o.Value.(type) {
	case *syntax.Duration:
		n := time.Duration(v.Value.Int)
		switch v.Unit {
		case "s":
			return n * time.Second, nil
		case "m":
			return n * time.Minute, nil
		case "h":
			return n * time.Hour, nil
		}
		return n * time.Millisecond, nil
	case *syntax.Number:
		return time.Duration(v.Int) * time.Millisecond, nil
	case *syntax.Placeholder:
		x, err := u.env.Eval(v.Expr)
		if err != nil {
			return 0, err
		}
		i, ok := x.(value.Int)
		if !ok {
			return 0, invalidType(v.Expr.Span, value.Repr(x), "positive integer")
		}
		if i < 0 {
			return 0, invalidType(v.Expr.Span, value.Repr(x), "positive integer")
		}
		return time.Duration(i) * time.Millisecond, nil
	}
	return 0, nil
}

func (u *unit) headerOption(o *syntax.Option, h *httpx.Options) error {
	t := o.Value.(*syntax.Template)
	s, err := u.env.Render(t)
	if err != nil {
		return err
	}
	hd, ok := parseHeader(s)
	if !ok {
		e := runerr.New(t.Span, runerr.InvalidOptionValue, false)
		e.Name, e.Value, e.Reason = "header", s, "missing `:`"
		return e
	}
	h.Headers = append(h.Headers, hd)
	return nil
}

func (u *unit) versionOption(o *syntax.Option, h *httpx.Options) error {
	b, err := u.boolOption(o)
	if err != nil {
		return err
	}
	on := map[string]httpx.HTTPVersion{"http1.0": httpx.HTTP10, "http1.1": httpx.HTTP11, "http2": httpx.HTTP2,
		"http2-prior-knowledge": httpx.HTTP2PriorKnowledge, "http3": httpx.HTTP3}
	off := map[string]httpx.HTTPVersion{"http1.1": httpx.HTTP10, "http2": httpx.HTTP11,
		"http2-prior-knowledge": httpx.HTTP11, "http3": httpx.HTTP2}
	switch {
	case b:
		h.HTTPVersion = on[o.Name]
	case o.Name != "http1.0":
		h.HTTPVersion = off[o.Name]
	}
	return nil
}

func (u *unit) variableOption(o *syntax.Option) error {
	d := o.Value.(*syntax.VariableDefinition)
	var v value.Value
	switch x := d.Value.(type) {
	case *syntax.Null:
		v = value.Null{}
	case *syntax.Boolean:
		v = value.Bool(x.Value)
	case *syntax.Number:
		v = numberValue(x)
	case *syntax.Template:
		s, err := u.env.Render(x)
		if err != nil {
			return err
		}
		v = value.String(s)
	}
	u.env.Vars.Set(d.Name, v)
	return nil
}

func numberValue(n *syntax.Number) value.Value {
	switch n.Kind {
	case syntax.NumberFloat:
		return value.Float(n.Float)
	case syntax.NumberBigInteger:
		return value.BigInt(n.Source)
	}
	return value.Int(n.Int)
}

func invalidType(span syntax.Span, actual, expected string) error {
	e := runerr.New(span, runerr.ExpressionInvalidType, false)
	e.Actual, e.Expected = actual, expected
	return e
}
