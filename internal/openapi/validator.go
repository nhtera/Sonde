// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// Options configure a validator.
type Options struct {
	// Server replaces the spec's servers for matching request URLs.
	Server string
	// Strict makes a request no operation matches a violation rather than
	// a warning.
	Strict bool
	// ExcludeOperations are never validated: "METHOD /path/template".
	ExcludeOperations []string
}

// Validator checks responses against a spec. It is safe for concurrent
// use.
type Validator struct {
	spec    *Spec
	router  *Router
	strict  bool
	exclude map[string]bool
}

var _ engine.ResponseValidator = (*Validator)(nil)

// Validator returns a validator of the spec; validators share the loaded
// spec.
func (s *Spec) Validator(opt Options) *Validator {
	v := &Validator{spec: s, router: s.router, strict: opt.Strict, exclude: map[string]bool{}}
	if opt.Server != "" {
		v.router = NewRouter(s.templates, []string{opt.Server})
	}
	for _, op := range opt.ExcludeOperations {
		v.exclude[op] = true
	}
	return v
}

// unchecked are the statuses never checked: a redirect or a cached
// response need not be documented.
var unchecked = map[int]bool{
	http.StatusMovedPermanently: true, http.StatusNotModified: true,
	http.StatusTemporaryRedirect: true, http.StatusPermanentRedirect: true,
}

// ValidateResponse implements engine.ResponseValidator: it checks the
// status, the documented headers, the content type and, for JSON, the
// body. A HEAD request without its own operation is checked against GET,
// without its body.
func (v *Validator) ValidateResponse(ctx context.Context, req *exchange.Request, resp *exchange.Response) (vs []engine.Violation) {
	defer func() {
		if p := recover(); p != nil {
			vs = []engine.Violation{{Kind: engine.ViolationError, Message: fmt.Sprintf("the response could not be checked against the spec: %v", p)}}
		}
	}()
	method := strings.ToUpper(req.Method)
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil
	}
	operation := func(t string) *openapi3.Operation {
		item := v.spec.doc.Paths.Value(t)
		op := item.GetOperation(method)
		if op == nil && method == http.MethodHead {
			op = item.GetOperation(http.MethodGet)
		}
		return op
	}
	rt, ok := v.router.Match(req.URL, func(t string) bool { return operation(t) != nil })
	if !ok {
		if other, ok := v.router.Match(req.URL, nil); ok {
			if v.exclude[method+" "+other.Template] {
				return nil
			}
			return v.unmatched(fmt.Sprintf("the OpenAPI spec has no %s %s operation", method, other.Template))
		}
		return v.unmatched(fmt.Sprintf("no operation of the OpenAPI spec matches %s %s", method, u.EscapedPath()))
	}
	opName := method + " " + rt.Template
	if v.exclude[opName] || unchecked[resp.Status] {
		return nil
	}
	op := operation(rt.Template)
	if op.Responses == nil || op.Responses.Len() == 0 {
		return nil
	}
	c := respCheck{op: opName, ptr: "#/paths/" + escapePointer(rt.Template) + "/" + strings.ToLower(method) + "/responses"}
	key, ref := documentedResponse(op.Responses, resp.Status)
	if ref == nil || ref.Value == nil {
		return c.add(engine.ViolationStatus, "", fmt.Sprintf("status %d is not documented", resp.Status))
	}
	c.ptr += "/" + escapePointer(key)
	header := http.Header{}
	for _, h := range resp.Headers {
		header.Add(h.Name, h.Value)
	}
	route := &routers.Route{Spec: v.spec.checkDoc, Path: rt.Template, PathItem: v.spec.doc.Paths.Value(rt.Template), Method: method, Operation: op}
	c.headers(ctx, route, rt.Params, u, resp.Status, header)
	if method == http.MethodHead || len(ref.Value.Content) == 0 {
		return c.vs
	}
	ct := header.Get("Content-Type")
	mediaKey, media := documentedMedia(ref.Value.Content, ct)
	if media == nil {
		c.ptr += "/content"
		return c.add(engine.ViolationContentType, "", fmt.Sprintf("content type %q is not documented", ct))
	}
	if media.Schema == nil || media.Schema.Value == nil || !isJSON(mediaType(ct)) {
		return c.vs
	}
	c.ptr += "/content/" + escapePointer(mediaKey) + "/schema"
	body, err := resp.DecodedBody()
	if err != nil {
		return c.add(engine.ViolationError, "", "the response body can not be decoded: "+err.Error())
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return c.add(engine.ViolationBody, "", "the response body is not valid JSON: "+err.Error())
	}
	c.body(media.Schema.Value.VisitJSON(value, openapi3.MultiErrors(), openapi3.VisitAsResponse()))
	return c.vs
}

// respCheck collects the violations of one response.
type respCheck struct {
	op, ptr string
	vs      []engine.Violation
}

func (c *respCheck) add(kind engine.ViolationKind, instance, msg string) []engine.Violation {
	c.vs = append(c.vs, engine.Violation{Kind: kind, Operation: c.op, SpecPointer: c.ptr, InstancePath: instance, Message: msg})
	return c.vs
}

var headerNameRe = regexp.MustCompile(`header "([^"]+)"`)

// headers checks the documented headers; the library stops at the first
// failing one.
func (c *respCheck) headers(ctx context.Context, route *routers.Route, params map[string]string, u *url.URL, status int, header http.Header) {
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    &http.Request{Method: http.MethodGet, URL: u, Header: http.Header{}},
			PathParams: params,
			Route:      route,
		},
		Status:  status,
		Header:  header,
		Options: &openapi3filter.Options{ExcludeResponseBody: true, MultiError: true},
	}
	err := openapi3filter.ValidateResponse(ctx, in)
	if err == nil {
		return
	}
	var re *openapi3filter.ResponseError
	if !errors.As(err, &re) {
		c.add(engine.ViolationHeader, "", err.Error())
		return
	}
	msg := re.Reason
	if re.Err != nil {
		var reasons []string
		for _, e := range flatten(re.Err) {
			reasons = append(reasons, schemaReason(e))
		}
		msg += ": " + strings.Join(reasons, "; ")
	}
	m := headerNameRe.FindStringSubmatch(re.Reason)
	if m == nil {
		c.add(engine.ViolationHeader, "", msg)
		return
	}
	ptr := c.ptr
	c.ptr += "/headers/" + escapePointer(m[1])
	if strings.Contains(re.Reason, "doesn't match schema") {
		c.ptr += "/schema"
	}
	c.add(engine.ViolationHeader, "header "+m[1], msg)
	c.ptr = ptr
}

// body converts the schema errors of the body.
func (c *respCheck) body(err error) {
	if err == nil {
		return
	}
	for _, e := range flatten(err) {
		instance := ""
		var se *openapi3.SchemaError
		if errors.As(e, &se) {
			if p := se.JSONPointer(); len(p) > 0 {
				instance = "/" + strings.Join(escapeAll(p), "/")
			}
		}
		c.add(engine.ViolationBody, instance, schemaReason(e))
	}
}

// schemaReason is the message of a schema error, with its keyword.
func schemaReason(err error) string {
	var se *openapi3.SchemaError
	if !errors.As(err, &se) {
		return err.Error()
	}
	msg := strings.TrimSpace(se.Reason)
	if msg == "" {
		msg = se.Error()
	}
	if se.SchemaField != "" {
		msg += " (" + se.SchemaField + ")"
	}
	return msg
}

func (v *Validator) unmatched(msg string) []engine.Violation {
	return []engine.Violation{{Kind: engine.ViolationUnmatched, Message: msg, Warning: !v.strict}}
}

// flatten returns the errors of a multi-error, recursively.
func flatten(err error) []error {
	var me openapi3.MultiError
	if errors.As(err, &me) {
		var out []error
		for _, e := range me {
			out = append(out, flatten(e)...)
		}
		return out
	}
	return []error{err}
}

// documentedResponse returns the response documenting status, with its
// key: the exact code, its range ("2XX") or "default".
func documentedResponse(r *openapi3.Responses, status int) (string, *openapi3.ResponseRef) {
	code := strconv.Itoa(status)
	for _, k := range []string{code, code[:1] + "XX", code[:1] + "xx", "default"} {
		if ref := r.Value(k); ref != nil {
			return k, ref
		}
	}
	return "", nil
}

// documentedMedia returns the media type of content matching ct, with
// its key (exact, "type/*" or "*/*").
func documentedMedia(content openapi3.Content, ct string) (string, *openapi3.MediaType) {
	m := content.Get(ct)
	if m == nil {
		return "", nil
	}
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if content[k] == m {
			return k, m
		}
	}
	return "", m
}

// isJSON reports whether a media type is JSON: application/json,
// */*+json or */json.
func isJSON(mt string) bool {
	return mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "/json")
}

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func mediaType(ct string) string {
	mt, _, _ := strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(mt))
}

func escapeAll(segs []string) []string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = escapePointer(s)
	}
	return out
}

// Operations returns the operations of the spec, "METHOD /path", sorted.
func (s *Spec) Operations() []string {
	var ops []string
	for _, t := range s.templates {
		for m := range s.doc.Paths.Value(t).Operations() {
			ops = append(ops, m+" "+t)
		}
	}
	sort.Strings(ops)
	return ops
}
