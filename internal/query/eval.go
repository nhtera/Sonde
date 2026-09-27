// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package query evaluates queries (`status`, `header`, `jsonpath`, …) on the
// responses of an entry.
package query

import (
	"crypto/md5" //nolint:gosec // G501: the md5 query is a checksum, not a security measure
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/datefmt"
	"github.com/nhtera/sonde/internal/filter"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/template"
	"github.com/nhtera/sonde/internal/value"
	"github.com/nhtera/sonde/internal/xpath"
)

// Context evaluates the queries of one entry. It caches the parsed body, so
// it must not be shared between entries or goroutines.
type Context struct {
	// Responses is the redirect chain; the last one is the final response.
	Responses []*exchange.Response
	Env       *template.Env

	// Forms of the final response body, each computed at most once.
	decoded   cached[[]byte]
	decText   cached[string]
	jsonCache cached[value.Value]
	xmlCache  cached[*xpath.Document]
}

// cached is a lazily computed value: err is a body decoding error, invalid
// reports a body that does not parse as JSON or XML.
type cached[T any] struct {
	done    bool
	v       T
	err     error
	invalid bool
}

// NewContext returns a context for the responses of an entry (at least one).
func NewContext(responses []*exchange.Response, env *template.Env) *Context {
	return &Context{Responses: responses, Env: env}
}

func (c *Context) last() *exchange.Response { return c.Responses[len(c.Responses)-1] }

// Eval evaluates q. A nil value means the query returned nothing.
func (c *Context) Eval(q *syntax.Query) (value.Value, error) {
	r := c.last()
	switch q.Kind {
	case syntax.QueryStatus:
		return value.Int(r.Status), nil
	case syntax.QueryVersion:
		return value.String(strings.TrimPrefix(r.Version, "HTTP/")), nil
	case syntax.QueryURL:
		return value.String(r.URL), nil
	case syntax.QueryHeader:
		return c.header(q.Arg.(*syntax.Template))
	case syntax.QueryCookie:
		return c.cookie(q.Arg.(*syntax.CookiePath))
	case syntax.QueryBody:
		s, err := c.text(q)
		if err != nil {
			return nil, err
		}
		return value.String(s), nil
	case syntax.QueryXPath:
		doc, err := c.xmlDoc(q)
		if err != nil {
			return nil, err
		}
		return filter.EvalXPath(doc, q.Arg.(*syntax.Template), c.Env)
	case syntax.QueryJSONPath:
		doc, err := c.jsonDoc(q)
		if err != nil {
			return nil, err
		}
		return filter.EvalJSONPath(doc, q.Arg.(*syntax.Template), c.Env)
	case syntax.QueryRegex:
		s, err := c.text(q)
		if err != nil {
			return nil, err
		}
		re, err := c.Env.Regex(q.Arg, q.Span)
		if err != nil {
			return nil, err
		}
		if g, ok := filter.FirstGroup(re.Re, s); ok {
			return value.String(g), nil
		}
		return nil, nil
	case syntax.QueryVariable:
		name, err := c.Env.Render(q.Arg.(*syntax.Template))
		if err != nil {
			return nil, err
		}
		v, _ := c.Env.Vars.Get(name)
		return v, nil
	case syntax.QueryDuration:
		return value.Int(r.Duration.Milliseconds()), nil
	case syntax.QueryBytes:
		b, err := c.body(q)
		if err != nil {
			return nil, err
		}
		return value.Bytes(b), nil
	case syntax.QueryRawBytes:
		return value.Bytes(r.Body), nil
	case syntax.QuerySHA256:
		b, err := c.body(q)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		return value.Bytes(sum[:]), nil
	case syntax.QueryMD5:
		b, err := c.body(q)
		if err != nil {
			return nil, err
		}
		sum := md5.Sum(b) //nolint:gosec // G401: see import
		return value.Bytes(sum[:]), nil
	case syntax.QueryCertificate:
		return certificate(r.Certificate, q.Arg.(*syntax.CertificateAttribute).Name), nil
	case syntax.QueryIP:
		return value.String(r.IP), nil
	case syntax.QueryRedirects:
		return c.redirects(), nil
	case syntax.QuerySondeStream:
		return stream(q, r.Stream)
	}
	return nil, nil
}

// bodyError converts a body decoding error into a runtime error at the
// query.
func bodyError(q *syntax.Query, err error) error {
	e := runerr.New(q.Span, runerr.HTTP, false)
	var be *exchange.BodyError
	if errors.As(err, &be) {
		e.Value, e.Reason = be.Description(), be.Message()
	} else {
		e.Value, e.Reason = "HTTP connection", err.Error()
	}
	return e
}

func (c *Context) body(q *syntax.Query) ([]byte, error) {
	if !c.decoded.done {
		c.decoded.v, c.decoded.err = c.last().DecodedBody()
		c.decoded.done = true
	}
	if c.decoded.err != nil {
		return nil, bodyError(q, c.decoded.err)
	}
	return c.decoded.v, nil
}

func (c *Context) text(q *syntax.Query) (string, error) {
	if !c.decText.done {
		c.decText.v, c.decText.err = c.last().Text()
		c.decText.done = true
	}
	if c.decText.err != nil {
		return "", bodyError(q, c.decText.err)
	}
	return c.decText.v, nil
}

// jsonDoc parses the body as JSON once per context.
func (c *Context) jsonDoc(q *syntax.Query) (value.Value, error) {
	if !c.jsonCache.done {
		s, err := c.text(q)
		if err != nil {
			return nil, err
		}
		c.jsonCache.v, err = value.DecodeJSON(s)
		c.jsonCache.invalid, c.jsonCache.done = err != nil, true
	}
	if c.jsonCache.invalid {
		return nil, runerr.New(q.Span, runerr.QueryInvalidJSON, false)
	}
	return c.jsonCache.v, nil
}

// xmlDoc parses the body as HTML or XML (by content type) once per context.
func (c *Context) xmlDoc(q *syntax.Query) (*xpath.Document, error) {
	if !c.xmlCache.done {
		s, err := c.text(q)
		if err != nil {
			return nil, err
		}
		format := xpath.XML
		if c.last().IsHTML() {
			format = xpath.HTML
		}
		c.xmlCache.v, err = xpath.Parse(s, format)
		c.xmlCache.invalid, c.xmlCache.done = err != nil, true
	}
	if c.xmlCache.invalid {
		return nil, runerr.New(q.Span, runerr.QueryInvalidXML, false)
	}
	return c.xmlCache.v, nil
}

func (c *Context) header(name *syntax.Template) (value.Value, error) {
	n, err := c.Env.Render(name)
	if err != nil {
		return nil, err
	}
	values := c.last().Headers.Values(n)
	switch len(values) {
	case 0:
		return nil, nil
	case 1:
		return value.String(values[0]), nil
	}
	list := make(value.List, len(values))
	for i, v := range values {
		list[i] = value.String(v)
	}
	return list, nil
}

func (c *Context) cookie(p *syntax.CookiePath) (value.Value, error) {
	name, err := c.Env.Render(p.Name)
	if err != nil {
		return nil, err
	}
	ck, ok := c.last().Cookie(name)
	if !ok {
		return nil, nil
	}
	attr := "value"
	if p.Attribute != nil {
		attr = strings.ToLower(p.Attribute.Name)
	}
	switch attr {
	case "expires":
		s, ok := ck.Attr("Expires")
		if !ok {
			return nil, nil
		}
		if t, ok := parseExpires(s); ok {
			return value.Date(t), nil
		}
		e := runerr.New(p.Name.Span, runerr.HTTP, true)
		e.Value, e.Reason = "HTTP connection", "could not parse Cookie Expires attribute value <"+s+">"
		return nil, e
	case "max-age":
		if n, ok := ck.MaxAge(); ok {
			return value.Int(n), nil
		}
	case "domain", "path", "samesite":
		if s, ok := ck.Attr(attr); ok {
			return value.String(s), nil
		}
	case "secure", "httponly":
		if ck.Flag(attr) {
			return value.Unit{}, nil
		}
	default:
		return value.String(ck.Value), nil
	}
	return nil, nil
}

// parseExpires reads a cookie date: RFC 2822, or the dashed form
// `Wed, 13-Jan-2021 22:23:01 GMT`.
func parseExpires(s string) (time.Time, bool) {
	if t, err := datefmt.ParseRFC2822(s); err == nil {
		return t, true
	}
	if t, err := datefmt.ParseNaiveDateTime(s, "%a, %d-%b-%Y %H:%M:%S%.3f GMT"); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func certificate(cert *exchange.CertInfo, attr string) value.Value {
	if cert == nil {
		return nil
	}
	str := func(s string) value.Value {
		if s == "" {
			return nil
		}
		return value.String(s)
	}
	date := func(t time.Time) value.Value {
		if t.IsZero() {
			return nil
		}
		return value.Date(t.UTC())
	}
	switch attr {
	case "Subject":
		return str(cert.Subject)
	case "Issuer":
		return str(cert.Issuer)
	case "Start-Date":
		return date(cert.StartDate)
	case "Expire-Date":
		return date(cert.ExpireDate)
	case "Serial-Number":
		return str(cert.SerialNumber)
	case "Subject-Alt-Name":
		return str(cert.SubjectAltName)
	}
	return str(cert.Value)
}

// redirects lists the hops of the redirect chain: each response that was
// followed by another, with the URL of that next response as location.
func (c *Context) redirects() value.Value {
	list := value.List{}
	for i := 0; i+1 < len(c.Responses); i++ {
		list = append(list, value.HTTPResponse{
			Location: c.Responses[i+1].URL, HasLocation: true, Status: c.Responses[i].Status,
		})
	}
	return list
}
