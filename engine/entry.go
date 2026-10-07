// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/filter"
	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/predicate"
	"github.com/nhtera/sonde/internal/query"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// asRunErr converts an evaluation error to a runtime error.
func asRunErr(err error, span syntax.Span) *runerr.Error {
	var re *runerr.Error
	if errors.As(err, &re) {
		return re
	}
	e := runerr.New(span, runerr.HTTP, false)
	e.Value, e.Reason = "Runtime error", err.Error()
	return e
}

// runEntry runs one attempt of an entry.
func (u *unit) runEntry(ctx context.Context, e *syntax.Entry, index int, eo *entryOptions) *EntryResult {
	res := &EntryResult{Index: index, Line: e.Request.Method.Span.Start.Line, Compressed: eo.http.Compressed}
	fail := func(err error, span syntax.Span) *EntryResult {
		res.Errors = append(res.Errors, &Error{run: asRunErr(err, span)})
		return res
	}
	resp := e.Response
	// Logs written as they come may already hold a value a later `redact`
	// capture makes secret; buffered logs are redacted when shown.
	if resp != nil && u.verbosity >= Brief && !u.runner.opt.BufferedLogs {
		for _, c := range captures(resp) {
			if c.Redact {
				// The entry is located at the capture, as the reference does.
				res.Line = c.Name.Span.Start.Line
				return fail(runerr.New(c.Name.Span, runerr.PossibleLoggedSecret, false), c.Name.Span)
			}
		}
	}
	spec, err := u.buildRequest(e.Request)
	if err != nil {
		return fail(err, e.Request.URL.Span)
	}
	set, clearAll := cookieCommands(e.Request)
	for _, s := range set {
		if err := u.client.AddCookie(s); err != nil {
			u.log(LogWarning, fmt.Sprintf("Cookie string can not be parsed: '%s'", s))
		}
	}
	if clearAll {
		u.client.ClearCookies()
	}
	opts := eo.http
	opts.Verbose = u.verbosity >= Verbose
	if u.runner.hostEvents {
		call := 0
		opts.OnSend = func(req exchange.Request) {
			call++
			u.emit(requestSent{index: index, call: call, req: req})
		}
	}
	res.Curl = u.entryCurl(e, spec, &opts, eo)
	u.logRequest(spec, res.Curl)

	calls, rerr := u.send(ctx, e, index, spec, &opts, eo)
	for _, c := range calls {
		res.Calls = append(res.Calls, Call(c))
		res.TransferDuration += c.Timings.Total
	}
	if rerr != nil {
		if rerr.Kind == runerr.Stream || rerr.Kind == runerr.GRPC { // a WebSocket or gRPC call that failed after its response
			u.logResponses(res.Calls)
			u.logStream(res.Calls)
		}
		res.Errors = append(res.Errors, &Error{run: rerr})
		return res
	}
	u.logResponses(res.Calls)
	u.logStream(res.Calls)
	responses := make([]*exchange.Response, len(calls))
	for i, c := range calls {
		responses[i] = c.Response
	}
	final := responses[len(responses)-1]
	noAssert := u.runner.opt.NoAssert

	if !noAssert && resp != nil {
		res.Asserts = append(res.Asserts, versionStatusAsserts(resp, final)...)
		if errs := assertErrors(res.Asserts); len(errs) > 0 {
			res.Errors = errs
			u.debug("")
			return res
		}
	}
	qctx := query.NewContext(responses, u.env)
	qctx.NoJSONPathCoercion = eo.noJSONPathCoercion
	if resp != nil {
		caps, secret, err := u.captures(resp, qctx)
		res.Captures = caps
		if slices.Contains(secret, true) && u.redacted != nil {
			u.redacted[res] = secret
		}
		u.logCaptures(caps)
		if err != nil {
			res.Errors = append(res.Errors, &Error{run: err})
			return res
		}
	}
	u.debug("")
	if !noAssert && resp != nil {
		u.warnDeprecated(resp)
		res.Asserts = append(res.Asserts, u.asserts(resp, final, qctx)...)
	}
	if !noAssert {
		u.validateContract(ctx, res, e)
	}
	res.Errors = assertErrors(res.Asserts)
	return res
}

// assertErrors returns the failures of asserts, as assert errors.
func assertErrors(asserts []Assert) []*Error {
	var errs []*Error
	for _, a := range asserts {
		if a.Err != nil {
			e := *a.Err.run
			e.Assert = true
			errs = append(errs, &Error{run: &e})
		}
	}
	return errs
}

func captures(r *syntax.Response) []*syntax.Capture {
	var cs []*syntax.Capture
	for _, s := range r.Sections {
		cs = append(cs, s.Captures...)
	}
	return cs
}

func explicitAsserts(r *syntax.Response) []*syntax.Assert {
	var as []*syntax.Assert
	for _, s := range r.Sections {
		as = append(as, s.Asserts...)
	}
	return as
}

// versionStatusAsserts checks the version and status line of the response.
func versionStatusAsserts(r *syntax.Response, final *exchange.Response) []Assert {
	v := Assert{Line: r.Version.Span.Start.Line}
	if r.Version.Value != "HTTP" && final.Version != r.Version.Value {
		e := runerr.New(r.Version.Span, runerr.AssertVersion, false)
		e.Actual = final.Version
		v.Err = &Error{run: e}
	}
	asserts := []Assert{v}
	if r.Status.Value != "*" {
		s := Assert{Line: r.Status.Span.Start.Line}
		if want, _ := strconv.Atoi(r.Status.Value); want != final.Status {
			e := runerr.New(r.Status.Span, runerr.AssertStatus, false)
			e.Actual = strconv.Itoa(final.Status)
			s.Err = &Error{run: e}
		}
		asserts = append(asserts, s)
	}
	return asserts
}

// captures evaluates the captures of a response and defines their
// variables; `redact` captures become secrets, and secret tells which
// captures they are.
func (u *unit) captures(r *syntax.Response, qctx *query.Context) (caps []Capture, secret []bool, _ *runerr.Error) {
	for _, c := range captures(r) {
		name, err := u.env.Render(c.Name)
		if err != nil {
			return caps, secret, asRunErr(err, c.Name.Span)
		}
		v, err := qctx.Eval(c.Query)
		if err != nil {
			return caps, secret, asRunErr(err, c.Query.Span)
		}
		if v == nil {
			return caps, secret, runerr.New(c.Query.Span, runerr.NoQueryResult, false)
		}
		if len(c.Filters) > 0 {
			if v, err = filter.ApplyOptions(c.Filters, v, u.env, filter.Options{NoJSONPathCoercion: qctx.NoJSONPathCoercion}); err != nil {
				return caps, secret, asRunErr(err, c.Query.Span)
			}
			if v == nil {
				span := syntax.Span{Start: c.Filters[0].Filter.Span.Start, End: c.Filters[len(c.Filters)-1].Filter.Span.End}
				return caps, secret, runerr.New(span, runerr.NoFilterResult, false)
			}
		}
		if c.Redact {
			s, ok := v.(value.String)
			if !ok {
				e := runerr.New(c.Name.Span, runerr.UnsupportedSecretType, false)
				e.Actual = v.Kind().String()
				return caps, secret, e
			}
			u.env.Vars.SetSecret(name, string(s))
			u.addSecret(name, string(s))
		} else {
			u.env.Vars.Set(name, v)
		}
		caps = append(caps, Capture{Name: name, Value: Value{v: v}})
		secret = append(secret, c.Redact)
	}
	return caps, secret, nil
}

// asserts evaluates the implicit header and body asserts, then the explicit
// asserts.
func (u *unit) asserts(r *syntax.Response, final *exchange.Response, qctx *query.Context) []Assert {
	var asserts []Assert
	for _, h := range r.Headers {
		asserts = append(asserts, u.headerAssert(h, final))
	}
	if r.Body != nil {
		asserts = append(asserts, u.bodyAssert(r.Body, final))
	}
	for _, a := range explicitAsserts(r) {
		as := Assert{Line: a.Predicate.Func.Span.Start.Line}
		if err := u.explicitAssert(a, qctx); err != nil {
			as.Err = &Error{run: asRunErr(err, a.Query.Span)}
		}
		asserts = append(asserts, as)
	}
	return asserts
}

func (u *unit) explicitAssert(a *syntax.Assert, qctx *query.Context) error {
	v, err := qctx.Eval(a.Query)
	if err != nil {
		return err
	}
	if len(a.Filters) > 0 {
		if v == nil {
			return runerr.New(a.Filters[0].Filter.Span, runerr.FilterMissingInput, true)
		}
		if v, err = filter.ApplyOptions(a.Filters, v, u.env, filter.Options{InAssert: true, NoJSONPathCoercion: qctx.NoJSONPathCoercion}); err != nil {
			return err
		}
	}
	return predicate.Eval(a.Predicate, v, u.env)
}

// headerAssert checks a response header written in the entry.
func (u *unit) headerAssert(h *syntax.KeyValue, final *exchange.Response) Assert {
	as := Assert{Line: h.Key.Span.Start.Line}
	expected, err := u.env.Render(h.Value)
	if err != nil {
		as.Err = &Error{run: asRunErr(err, h.Key.Span)}
		return as
	}
	name, err := u.env.Render(h.Key)
	if err != nil {
		as.Line, as.Err = h.Value.Span.Start.Line, &Error{run: asRunErr(err, h.Value.Span)}
		return as
	}
	actuals := final.Headers.Values(name)
	if len(actuals) == 0 {
		as.Err = &Error{run: runerr.New(h.Key.Span, runerr.QueryHeaderNotFound, false)}
		return as
	}
	as.Line = h.Value.Span.Start.Line
	actual := actuals[0]
	if len(actuals) > 1 {
		quoted := make([]string, len(actuals))
		for i, a := range actuals {
			quoted[i] = `"` + a + `"`
		}
		actual = "[" + strings.Join(quoted, ", ") + "]"
		for _, a := range actuals {
			if a == expected {
				actual = a
				break
			}
		}
	}
	if actual != expected {
		e := runerr.New(h.Value.Span, runerr.AssertHeaderValue, false)
		e.Actual = actual
		as.Err = &Error{run: e}
	}
	return as
}

// bodyAssert compares the response body with the body written in the entry.
func (u *unit) bodyAssert(b *syntax.Body, final *exchange.Response) Assert {
	point := syntax.Span{Start: b.Space0.Span.End, End: b.Space0.Span.End}
	span := b.Space0.Span
	var expected value.Value
	var err error
	text := true
	switch v := b.Value.(type) {
	case *syntax.Template:
		var s string
		if v.Delimiter == '"' { // a JSON string
			s, err = u.env.RenderJSON(v, true)
		} else {
			span = v.Span
			s, err = u.env.Render(v)
		}
		expected = value.String(s)
	case *syntax.MultilineString:
		span = v.Value.Span
		var s string
		s, err = u.env.RenderMultiline(v)
		expected = value.String(s)
	case *syntax.XML:
		expected = value.String(v.Value)
	case *syntax.Base64:
		span, text = syntax.Span{Start: v.Space0.Span.End, End: v.Space1.Span.Start}, false
		expected = value.Bytes(v.Value)
	case *syntax.Hex:
		span, text = syntax.Span{Start: v.Space0.Span.End, End: v.Space1.Span.Start}, false
		expected = value.Bytes(v.Value)
	case *syntax.FileRef:
		text = false
		var data []byte
		data, err = u.env.File(v.Filename)
		expected = value.Bytes(data)
	case syntax.JSONValue:
		var s string
		s, err = u.env.RenderJSON(v, true)
		expected = value.String(s)
	}
	as := Assert{Line: span.Start.Line}
	if err != nil {
		as.Err = &Error{run: asRunErr(err, span)}
		return as
	}
	var actual value.Value
	if text {
		s, terr := final.Text()
		actual, err = value.String(s), terr
	} else {
		data, derr := final.DecodedBody()
		actual, err = value.Bytes(data), derr
	}
	if err != nil {
		e := runerr.New(point, runerr.HTTP, true)
		var be *exchange.BodyError
		if errors.As(err, &be) {
			e.Value, e.Reason = be.Description(), be.Message()
		}
		as.Err = &Error{run: e}
		return as
	}
	if value.Equal(actual, expected) {
		return as
	}
	es, eok := expected.(value.String)
	as2, aok := actual.(value.String)
	if eok && aok && (strings.Contains(string(es), "\n") || strings.Contains(string(as2), "\n")) {
		hunk, line := firstHunk(string(es), string(as2))
		at := syntax.Pos{Line: span.Start.Line + line, Col: 1}
		e := runerr.New(syntax.Span{Start: at, End: at}, runerr.AssertBodyDiff, false)
		e.Reason = hunk
		as.Err = &Error{run: e}
		return as
	}
	e := runerr.New(span, runerr.AssertBodyValue, false)
	e.Actual = value.Display(actual)
	as.Err = &Error{run: e}
	return as
}

// warnDeprecated warns about deprecated predicates and filters.
func (u *unit) warnDeprecated(r *syntax.Response) {
	var includes, fmtCapture, decodeCapture, fmtAssert, decodeAssert bool
	has := func(items []*syntax.FilterItem, k syntax.FilterKind) bool {
		for _, it := range items {
			if it.Filter.Kind == k {
				return true
			}
		}
		return false
	}
	for _, a := range explicitAsserts(r) {
		includes = includes || a.Predicate.Func.Kind == syntax.PredicateInclude
		fmtAssert = fmtAssert || has(a.Filters, syntax.FilterFormat)
		decodeAssert = decodeAssert || has(a.Filters, syntax.FilterDecode)
	}
	for _, c := range captures(r) {
		fmtCapture = fmtCapture || has(c.Filters, syntax.FilterFormat)
		decodeCapture = decodeCapture || has(c.Filters, syntax.FilterDecode)
	}
	if includes {
		u.log(LogWarning, "<includes> predicate is now deprecated in favor of <contains> predicate")
	}
	if fmtCapture {
		u.log(LogWarning, "<format> filter in captures is now deprecated in favor of <dateFormat> filter")
	}
	if decodeCapture {
		u.log(LogWarning, "<decode> filter in captures is now deprecated in favor of <charsetDecode> filter")
	}
	if fmtAssert {
		u.log(LogWarning, "<format> filter in asserts is now deprecated in favor of <dateFormat> filter")
	}
	if decodeAssert {
		u.log(LogWarning, "<decode> filter in asserts is now deprecated in favor of <charsetDecode> filter")
	}
}

// logRequest logs the rendered request in verbose mode.
func (u *unit) logRequest(spec *httpx.RequestSpec, curl string) {
	if u.verbosity < Verbose {
		return
	}
	u.debug("")
	u.debugImportant("Cookie store:")
	for _, c := range u.client.Cookies() {
		u.debug(Cookie(c).Netscape())
	}
	u.debug("")
	u.debugImportant("Request:")
	u.debug(spec.Method + " " + spec.URL)
	for _, h := range spec.Headers {
		u.debug(h.Name + ": " + h.Value)
	}
	section := func(name string, params []httpx.Param) {
		if len(params) == 0 {
			return
		}
		u.debug("[" + name + "]")
		for _, p := range params {
			u.debug(p.Name + ": " + p.Value)
		}
	}
	section("Query", spec.Query)
	section("Form", spec.Form)
	if len(spec.Multipart) > 0 {
		u.debug("[Multipart]")
		for _, p := range spec.Multipart {
			if p.Param != nil {
				u.debug(p.Param.Name + ": " + p.Param.Value)
			} else {
				u.debug(fmt.Sprintf("%s: file,%s; %s", p.File.Name, p.File.Filename, p.File.ContentType))
			}
		}
	}
	if len(spec.Cookies) > 0 {
		u.debug("[Cookies]")
		for _, c := range spec.Cookies {
			u.debug(c.Name + "=" + c.Value)
		}
	}
	u.debug("")
	if curl != "" {
		u.debug("Request can be run with the following curl command:")
		u.debug(curl)
		u.debug("")
	}
}

// logResponses logs the request and response headers of every call.
func (u *unit) logResponses(calls []Call) {
	if u.verbosity < Brief {
		return
	}
	for i, c := range calls {
		if i > 0 {
			u.debug("")
			u.debug("=> Redirect to " + c.Request.URL)
			u.debug("")
		}
		target := c.Request.URL
		if i := strings.Index(target, "://"); i >= 0 {
			if j := strings.IndexByte(target[i+3:], '/'); j >= 0 {
				target = target[i+3+j:]
			} else {
				target = "/"
			}
		}
		u.log(LogRequestLine, fmt.Sprintf("%s %s %s", c.Request.Method, target, c.Response.Version))
		for _, h := range c.Request.Headers {
			u.log(LogRequest, displaySafe(h.Name+": "+h.Value))
		}
		u.log(LogRequest, "")
		u.debugImportant("Response:")
		u.debug("")
		status := fmt.Sprintf("%s %d", c.Response.Version, c.Response.Status)
		if c.Response.Reason != "" {
			status += " " + c.Response.Reason
		}
		u.log(LogResponseLine, displaySafe(status))
		for _, h := range c.Response.Headers {
			u.log(LogResponse, displaySafe(h.Name+": "+h.Value))
		}
		u.log(LogResponse, "")
		u.debug(fmt.Sprintf("Received %d bytes in %d ms", len(c.Response.Body), c.Timings.Total.Milliseconds()))
	}
}

func (u *unit) logCaptures(caps []Capture) {
	if u.verbosity < Verbose || len(caps) == 0 {
		return
	}
	u.debugImportant("Captures:")
	for _, c := range caps {
		u.log(LogCapture, c.Name+": "+value.Display(c.Value.v))
	}
}

// writeFailedBody writes the body of a failed entry (fail-with-body) to
// its `output`, else standard output, which then gets a final newline
// (unless the body ends with one) so that the errors start on their own
// line.
func (u *unit) writeFailedBody(res *EntryResult, eo *entryOptions) {
	out := eo.output
	if out == nil {
		out = &outputTarget{name: "-"}
	}
	_ = u.writeOutput(res, out, true) // an error is added to res, which the caller logs
}

// writeOutput writes the final response body of res to out; failed adds
// the final newline of writeFailedBody. An error is added to res and
// returned, for the caller to log.
func (u *unit) writeOutput(res *EntryResult, out *outputTarget, failed bool) *runerr.Error {
	if len(res.Calls) == 0 {
		return nil
	}
	resp := res.Calls[len(res.Calls)-1].Response
	newline := failed && out.name == "-" && !bytes.HasSuffix(resp.Body, []byte("\n"))
	body := resp.Body
	if res.Compressed {
		var err error
		if body, err = resp.DecodedBody(); err != nil {
			re := runerr.New(out.span, runerr.HTTP, false)
			var be *exchange.BodyError
			if errors.As(err, &be) {
				re.Value, re.Reason = be.Description(), be.Message()
			}
			res.Errors = append(res.Errors, &Error{run: re})
			if newline && u.io.stdout != nil {
				_, _ = u.io.stdout.Write([]byte("\n"))
			}
			return re
		}
	}
	if out.name == "-" {
		if u.io.stdout != nil {
			if render := u.runner.opt.StdoutBody; render != nil {
				body = render(resp, body)
			}
			if newline {
				// One write, so that a shared stdout keeps them together.
				body = append(body[:len(body):len(body)], '\n')
			}
			_, _ = u.io.stdout.Write(body)
		}
		return nil
	}
	if err := u.root.WriteFile(out.name, body); err != nil {
		kind := runerr.FileWriteAccess
		if errors.Is(err, sandbox.ErrDenied) {
			kind = runerr.UnauthorizedFileAccess
		}
		re := runerr.New(out.span, kind, false)
		re.Value, re.Reason = out.name, err.Error()
		var pe *fs.PathError
		if kind == runerr.FileWriteAccess && errors.As(err, &pe) {
			re.Value, re.Reason = u.resolvedPath(out.name), pe.Err.Error()
		}
		res.Errors = append(res.Errors, &Error{run: re})
		return re
	}
	return nil
}

// displaySafe escapes what a terminal would interpret in a header shown
// in the logs: control characters other than a tab (C0, DEL and C1) as
// \xHH or \u00HH, and bytes that are not UTF-8 as \xHH. A server can no
// longer move the cursor or set the clipboard through a header.
func displaySafe(s string) string {
	safe := true
	for _, r := range s {
		if r == utf8.RuneError || (r < 0x20 && r != '\t') || (r >= 0x7f && r < 0xa0) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size <= 1:
			fmt.Fprintf(&b, "\\x%02x", s[i])
		case (r < 0x20 && r != '\t') || r == 0x7f:
			fmt.Fprintf(&b, "\\x%02x", r)
		case r >= 0x80 && r < 0xa0:
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}
