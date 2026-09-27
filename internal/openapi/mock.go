// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

// Mock answers requests with the documented responses of a spec (sonde
// mock). It keeps every kin-openapi type to itself, so the server around
// it only deals with net/http. It is read only and safe for concurrent
// use; its responses depend only on the request, never on earlier ones.
type Mock struct {
	spec   *Spec
	router *Router
}

// Mock returns a mock of the spec. A non-empty server replaces the spec's
// servers: its base path is the one stripped from request paths.
func (s *Spec) Mock(server string) *Mock {
	m := &Mock{spec: s, router: s.router}
	if server != "" {
		m.router = NewRouter(s.templates, []string{server})
	}
	return m
}

// Operation is the operation a request matched.
type Operation struct {
	// Method is the operation's method: GET for a HEAD request answered
	// by the GET operation.
	Method   string
	Template string
	// Params are the decoded path parameters.
	Params map[string]string
	item   *openapi3.PathItem
	op     *openapi3.Operation
}

// Name is "METHOD /template".
func (o *Operation) Name() string { return o.Method + " " + o.Template }

// Problem is a request the mock refuses, answered with an RFC 9457
// problem document.
type Problem struct {
	Status int
	Detail string
	// Violations list what is wrong with the request.
	Violations []string
	// Allow lists the methods of the path, for a 405.
	Allow []string
}

func (p *Problem) Error() string { return p.Detail }

func problemf(status int, format string, args ...any) *Problem {
	return &Problem{Status: status, Detail: fmt.Sprintf(format, args...)}
}

// Match returns the operation of method on path, an escaped URL path: a
// 404 problem when no path template matches it, a 405 when its template
// has no such operation. A HEAD request falls back to the GET operation.
func (m *Mock) Match(method, path string) (*Operation, *Problem) {
	method = strings.ToUpper(method)
	operation := func(t string) (string, *openapi3.Operation) {
		item := m.spec.doc.Paths.Value(t)
		if op := item.GetOperation(method); op != nil {
			return method, op
		}
		if method == http.MethodHead {
			if op := item.GetOperation(http.MethodGet); op != nil {
				return http.MethodGet, op
			}
		}
		return "", nil
	}
	rawURL := "http://mock" + path
	rt, ok := m.router.Match(rawURL, func(t string) bool { _, op := operation(t); return op != nil })
	if !ok {
		if other, ok := m.router.Match(rawURL, nil); ok {
			p := problemf(http.StatusMethodNotAllowed, "the OpenAPI spec has no %s %s operation", method, other.Template)
			item := m.spec.doc.Paths.Value(other.Template)
			for name := range item.Operations() {
				p.Allow = append(p.Allow, name)
				if name == http.MethodGet && item.Head == nil {
					p.Allow = append(p.Allow, http.MethodHead)
				}
			}
			sort.Strings(p.Allow)
			return nil, p
		}
		return nil, problemf(http.StatusNotFound, "no operation of the OpenAPI spec matches %s %s", method, path)
	}
	opMethod, op := operation(rt.Template)
	return &Operation{Method: opMethod, Template: rt.Template, Params: rt.Params, item: m.spec.doc.Paths.Value(rt.Template), op: op}, nil
}

// Selection chooses among the documented responses of an operation.
type Selection struct {
	// Status is a status code ("" for the lowest documented 2xx); it may
	// be documented exactly, by its range ("4XX") or by "default".
	Status string
	// Example names one of the media type's examples ("" for the first).
	Example string
	// Accept is the request's Accept header ("" accepts anything).
	Accept string
}

// Response is a mock response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
	// Warning is set when a body generated from the schema does not match
	// that schema (a schema the generator cannot satisfy).
	Warning string
}

// Respond renders the response sel chooses: its documented headers, the
// best media type for sel.Accept, and a body taken from the named or
// first example, else generated from the schema. The same operation and
// selection always give the same bytes.
func (m *Mock) Respond(o *Operation, sel Selection) (*Response, *Problem) {
	status, ref, p := selectResponse(o, sel.Status)
	if p != nil {
		return nil, p
	}
	out := &Response{Status: status, Header: http.Header{}}
	resp := ref.Value
	for _, name := range sortedMapKeys(resp.Headers) {
		h := resp.Headers[name]
		if h == nil || h.Value == nil || strings.EqualFold(name, "Content-Type") {
			continue
		}
		if v := headerText(headerExample(h.Value)); v != "" || h.Value.Required {
			out.Header.Set(name, v)
		}
	}
	if len(resp.Content) == 0 {
		return out, nil
	}
	cands := negotiate(resp.Content, sel.Accept)
	if len(cands) == 0 {
		return nil, problemf(http.StatusNotAcceptable, "%s %d has no media type matching Accept %q; documented: %s",
			o.Name(), status, sel.Accept, strings.Join(sortedMapKeys(resp.Content), ", "))
	}
	var (
		media     *openapi3.MediaType
		ct        string
		value     any
		generated bool
	)
	for _, c := range cands {
		media, ct = resp.Content[c.key], c.contentType
		if value, generated, p = responseExample(o, media, sel.Example); p != nil {
			return nil, p
		}
		if renderable(value, ct) {
			break
		}
		media = nil
	}
	if media == nil {
		return nil, problemf(http.StatusNotAcceptable, "%s %d: no example of a media type matching Accept %q can be sent as such "+
			"(a structured example is only rendered as JSON): add a string example", o.Name(), status, sel.Accept)
	}
	out.Header.Set("Content-Type", ct)
	body, err := encodeBody(value, ct)
	if err != nil {
		return nil, problemf(http.StatusInternalServerError, "the example of %s %d can not be encoded: %v", o.Name(), status, err)
	}
	out.Body = body
	if generated && isJSON(mediaType(ct)) && media.Schema != nil && media.Schema.Value != nil {
		out.Warning = selfCheck(media.Schema.Value, body)
		if out.Warning != "" {
			out.Warning = fmt.Sprintf("the body generated for %s %d does not match its schema: %s", o.Name(), status, out.Warning)
		}
	}
	return out, nil
}

// selectResponse returns the status and documented response of code, or
// of the lowest documented 2xx, else of the lowest documented status.
func selectResponse(o *Operation, code string) (int, *openapi3.ResponseRef, *Problem) {
	if o.op.Responses == nil || o.op.Responses.Len() == 0 {
		return 0, nil, problemf(http.StatusInternalServerError, "%s documents no response", o.Name())
	}
	if code != "" {
		status, err := strconv.Atoi(code)
		if err != nil || status < 200 || status > 599 {
			return 0, nil, problemf(http.StatusBadRequest, "Prefer code=%s is not a final HTTP status (200-599)", code)
		}
		if _, ref := documentedResponse(o.op.Responses, status); ref != nil && ref.Value != nil {
			return status, ref, nil
		}
		return 0, nil, problemf(http.StatusBadRequest, "%s does not document status %d; documented: %s",
			o.Name(), status, strings.Join(sortedMapKeys(o.op.Responses.Map()), ", "))
	}
	status := 0
	if c := successStatus(o.op); c != "" {
		status, _ = strconv.Atoi(c)
	} else {
		for _, k := range sortedMapKeys(o.op.Responses.Map()) {
			// Ranges ("4XX") stand for their first status.
			if n, err := strconv.Atoi(strings.NewReplacer("X", "0", "x", "0").Replace(k)); err == nil && len(k) == 3 {
				status = n
				break
			}
		}
		if status == 0 {
			status = http.StatusOK // only "default" is documented
		}
	}
	_, ref := documentedResponse(o.op.Responses, status)
	if ref == nil || ref.Value == nil {
		return 0, nil, problemf(http.StatusInternalServerError, "%s documents no usable response", o.Name())
	}
	return status, ref, nil
}

// candidate is a documented media type (key) acceptable to a request,
// with the concrete content type to send for it.
type candidate struct{ key, contentType string }

// negotiate returns the documented media types acceptable to accept, best
// first. Without Accept every media type is, the preferred first (JSON
// first); a range of quality 0 excludes the types it matches at least as
// specifically as the range accepting them.
func negotiate(content openapi3.Content, accept string) []candidate {
	keys := sortedMapKeys(content)
	sort.SliceStable(keys, func(i, j int) bool { return mediaRank(mediaType(keys[i])) < mediaRank(mediaType(keys[j])) })
	var out []candidate
	if strings.TrimSpace(accept) == "" {
		for _, k := range keys {
			out = append(out, candidate{k, concreteType(k, "")})
		}
		return out
	}
	ranges, excluded := acceptRanges(accept)
	seen := map[string]bool{}
	for _, r := range ranges {
		for _, k := range keys {
			if seen[k] || !mediaMatch(r, mediaType(k)) {
				continue
			}
			ct := concreteType(k, r)
			if slices.ContainsFunc(excluded, func(x string) bool { return mediaMatch(x, mediaType(ct)) && specificity(x) >= specificity(r) }) {
				continue
			}
			seen[k] = true
			out = append(out, candidate{k, ct})
		}
	}
	return out
}

// specificity ranks a media range: 0 for "*/*", 1 for "type/*", 2 for a
// media type.
func specificity(r string) int {
	switch {
	case r == "*/*":
		return 0
	case strings.HasSuffix(r, "/*"):
		return 1
	}
	return 2
}

// renderable reports whether an example can be sent as content type ct:
// anything as JSON, only a scalar as another media type.
func renderable(v any, ct string) bool {
	if isJSON(mediaType(ct)) {
		return true
	}
	switch v.(type) {
	case nil, string, bool, float64, int64, int, json.Number:
		return true
	}
	return false
}

// acceptRanges returns the media ranges of an Accept header, by
// decreasing quality, and apart the ranges of quality 0.
func acceptRanges(accept string) (ranges, excluded []string) {
	type rng struct {
		mt string
		q  float64
	}
	var rs []rng
	for part := range strings.SplitSeq(accept, ",") {
		fields := strings.Split(part, ";")
		mt := strings.ToLower(strings.TrimSpace(fields[0]))
		if mt == "" {
			continue
		}
		if mt == "*" {
			mt = "*/*"
		}
		q := 1.0
		for _, f := range fields[1:] {
			if k, v, ok := strings.Cut(strings.TrimSpace(f), "="); ok && strings.EqualFold(strings.TrimSpace(k), "q") {
				if x, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = x
				}
			}
		}
		if q > 0 {
			rs = append(rs, rng{mt, q})
		} else {
			excluded = append(excluded, mt)
		}
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].q > rs[j].q })
	for _, r := range rs {
		ranges = append(ranges, r.mt)
	}
	return ranges, excluded
}

// mediaMatch reports whether media types a and b match, either being a
// range ("*/*", "type/*").
func mediaMatch(a, b string) bool {
	at, as, _ := strings.Cut(a, "/")
	bt, bs, _ := strings.Cut(b, "/")
	return (at == "*" || bt == "*" || at == bt) && (as == "*" || bs == "*" || as == bs)
}

// concreteType is the content type to send for the documented media type
// key, requested by the range accepted: key itself unless it is a range.
func concreteType(key, accepted string) string {
	if !strings.Contains(mediaType(key), "*") {
		return key
	}
	rng := mediaType(key)
	if accepted != "" {
		if !strings.Contains(accepted, "*") {
			return accepted
		}
		if rng == "*/*" {
			rng = accepted // the narrower range
		}
	}
	switch rng {
	case "*/*", "application/*":
		return "application/json"
	case "text/*":
		return "text/plain"
	}
	return "application/octet-stream"
}

// responseExample returns the named example, else the media type's
// example or first example, else one generated from its schema
// (generated true).
func responseExample(o *Operation, media *openapi3.MediaType, name string) (value any, generated bool, p *Problem) {
	if media == nil {
		return nil, false, nil
	}
	if name != "" {
		if ex := media.Examples[name]; ex != nil && ex.Value != nil {
			return ex.Value.Value, false, nil
		}
		names := sortedMapKeys(media.Examples)
		if len(names) == 0 {
			return nil, false, problemf(http.StatusBadRequest, "%s has no example %q: it documents no named example", o.Name(), name)
		}
		return nil, false, problemf(http.StatusBadRequest, "%s has no example %q; documented: %s", o.Name(), name, strings.Join(names, ", "))
	}
	if media.Example != nil {
		return media.Example, false, nil
	}
	for _, k := range sortedMapKeys(media.Examples) {
		if ex := media.Examples[k]; ex != nil && ex.Value != nil && ex.Value.Value != nil {
			return ex.Value.Value, false, nil
		}
	}
	return ResponseExample(media.Schema), true, nil
}

// headerExample is the example of a response header, else one generated
// from its schema.
func headerExample(h *openapi3.Header) any {
	if h.Example != nil {
		return h.Example
	}
	for _, k := range sortedMapKeys(h.Examples) {
		if ex := h.Examples[k]; ex != nil && ex.Value != nil && ex.Value.Value != nil {
			return ex.Value.Value
		}
	}
	return ResponseExample(h.Schema)
}

// headerText formats a header example in the simple style: array items
// and object keys and values joined by commas.
func headerText(v any) string {
	switch x := v.(type) {
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = scalarText(e)
		}
		return strings.Join(parts, ",")
	case map[string]any:
		var parts []string
		for _, k := range sortedMapKeys(x) {
			parts = append(parts, k, scalarText(x[k]))
		}
		return strings.Join(parts, ",")
	}
	return scalarText(v)
}

// encodeBody encodes an example for content type ct: JSON for a JSON
// media type, the text of a string, JSON for other composite values.
func encodeBody(v any, ct string) ([]byte, error) {
	if !isJSON(mediaType(ct)) {
		if v == nil {
			return nil, nil
		}
		return []byte(scalarText(v)), nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// selfCheck validates a generated JSON body against its schema, as the
// response validator would; it returns the reasons it fails, or "".
func selfCheck(schema *openapi3.Schema, body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return err.Error()
	}
	err := schema.VisitJSON(value, openapi3.MultiErrors(), openapi3.VisitAsResponse())
	if err == nil {
		return ""
	}
	var reasons []string
	for _, e := range flatten(err) {
		reasons = append(reasons, schemaReason(e))
	}
	return strings.Join(reasons, "; ")
}

// ValidateRequest checks the parameters and the body of r, whose body
// must be readable again (the server reads it first, bounded), against
// the operation: nil when valid, a 415 problem for an undocumented
// content type, else a 422 listing the violations. Security requirements
// are not enforced.
func (m *Mock) ValidateRequest(ctx context.Context, o *Operation, r *http.Request) (p *Problem) {
	defer func() {
		if x := recover(); x != nil {
			p = problemf(http.StatusInternalServerError, "the request could not be checked against the spec: %v", x)
		}
	}()
	if rb := o.op.RequestBody; rb != nil && rb.Value != nil && len(rb.Value.Content) > 0 && hasBody(r) {
		if ct := r.Header.Get("Content-Type"); rb.Value.Content.Get(ct) == nil {
			return problemf(http.StatusUnsupportedMediaType, "%s does not accept Content-Type %q; documented: %s",
				o.Name(), ct, strings.Join(sortedMapKeys(rb.Value.Content), ", "))
		}
	}
	route := &routers.Route{Spec: m.spec.checkDoc, Path: o.Template, PathItem: o.item, Method: o.Method, Operation: o.op}
	in := &openapi3filter.RequestValidationInput{
		Request:    r,
		PathParams: o.Params,
		Route:      route,
		Options:    &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, MultiError: true},
	}
	err := openapi3filter.ValidateRequest(ctx, in)
	if err == nil {
		return nil
	}
	p = problemf(http.StatusUnprocessableEntity, "the request does not match %s", o.Name())
	for _, e := range requestErrors(err) {
		var re *openapi3filter.RequestError
		if !errors.As(e, &re) {
			p.Violations = append(p.Violations, e.Error())
			continue
		}
		p.Violations = append(p.Violations, requestViolations(re)...)
	}
	return p
}

// hasBody reports whether r has a non-empty body, reading it again
// through GetBody when it can.
func hasBody(r *http.Request) bool {
	if r.GetBody == nil {
		return r.ContentLength > 0
	}
	b, err := r.GetBody()
	if err != nil {
		return false
	}
	defer b.Close() //nolint:errcheck // in memory
	n, _ := b.Read(make([]byte, 1))
	return n > 0
}

// requestErrors flattens the top-level multi-error of ValidateRequest,
// keeping each RequestError whole (errors.As would unwrap a RequestError
// down to the multi-error of its schema errors).
func requestErrors(err error) []error {
	if me, ok := err.(openapi3.MultiError); ok { //nolint:errorlint // only the top-level multi-error is split

		var out []error
		for _, e := range me {
			out = append(out, requestErrors(e)...)
		}
		return out
	}
	return []error{err}
}

// requestViolations describes one request error: where (parameter or
// body) and each schema reason, with its instance path for a body.
func requestViolations(re *openapi3filter.RequestError) []string {
	where := "request"
	switch {
	case re.Parameter != nil:
		where = fmt.Sprintf("%s parameter %q", re.Parameter.In, re.Parameter.Name)
	case re.RequestBody != nil:
		where = "body"
	}
	if re.Err == nil {
		return []string{where + ": " + re.Reason}
	}
	if errors.Is(re.Err, openapi3filter.ErrInvalidRequired) {
		return []string{where + ": required but missing"}
	}
	var out []string
	for _, e := range flatten(re.Err) {
		at := where
		var se *openapi3.SchemaError
		if errors.As(e, &se) && re.RequestBody != nil {
			if ptr := se.JSONPointer(); len(ptr) > 0 {
				at += " /" + strings.Join(escapeAll(ptr), "/")
			}
		}
		out = append(out, at+": "+schemaReason(e))
	}
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
