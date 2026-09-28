// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// Result is one file's JSON result: the schema shared by `--json` and
// `--report-json` (docs/report-json.md). Field names, order and nesting
// reproduce the upstream 8.0.1 JSON export byte-for-byte (verified against
// testdata/conformance/hurl/tests_ok/json_output and
// tests_failed/assert_value_error): every struct's JSON keys are declared
// in the alphabetical order the reference serializer produces.
type Result struct {
	Cookies  []Cookie `json:"cookies"`
	Entries  []Entry  `json:"entries"`
	Filename string   `json:"filename"`
	// Sonde holds data without an upstream equivalent; absent otherwise.
	Sonde   *Sonde `json:"sonde,omitempty"`
	Success bool   `json:"success"`
	Time    int64  `json:"time"` // milliseconds
}

// Sonde is the `sonde` object of a result.
type Sonde struct {
	Iteration *Iteration `json:"iteration,omitempty"`
}

// Iteration identifies the data row a file ran with (`--data`).
type Iteration struct {
	Row int `json:"row"` // 1-based
}

// Cookie is one entry of the cookie store at the end of a run.
type Cookie struct {
	Domain           string `json:"domain"`
	Expires          int64  `json:"expires"`
	HTTPS            bool   `json:"https"`
	IncludeSubdomain bool   `json:"include_subdomain"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	Value            string `json:"value"`
}

// NameValue is the shape shared by header, request-cookie and query-string
// entries.
type NameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Entry is one attempt of an entry (one per retry, one per repetition).
type Entry struct {
	Asserts  []Assert  `json:"asserts"`
	Calls    []Call    `json:"calls"`
	Captures []Capture `json:"captures"`
	CurlCmd  string    `json:"curl_cmd"`
	Index    int       `json:"index"`
	Line     int       `json:"line"`
	// Sonde holds data without an upstream equivalent; absent otherwise.
	Sonde *EntrySonde `json:"sonde,omitempty"`
	Time  int64       `json:"time"` // milliseconds
}

// EntrySonde is the `sonde` object of an entry.
type EntrySonde struct {
	Contract *Contract   `json:"contract,omitempty"`
	GRPC     *GRPCStatus `json:"grpc,omitempty"`
	Stream   *Stream     `json:"stream,omitempty"`
}

// GRPCStatus is the status of a gRPC call.
type GRPCStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"` // "OK", "NOT_FOUND"…
}

// Stream is what a streamed entry exchanged after its response headers:
// Server-Sent Events, WebSocket messages or gRPC replies.
type Stream struct {
	Messages   []StreamMessage `json:"messages"`
	Protocol   string          `json:"protocol"` // "sse", "websocket" or "grpc"
	Received   int             `json:"received"`
	Sent       int             `json:"sent"`
	StopReason string          `json:"stop_reason,omitempty"` // absent when the stream failed
}

// StreamMessage is one message of a stream. Data is text, or base64 when
// Binary is set.
type StreamMessage struct {
	Binary    bool   `json:"binary,omitempty"`
	Data      string `json:"data"`
	Direction string `json:"direction"` // "sent" or "received"
	Event     string `json:"event,omitempty"`
	ID        string `json:"id,omitempty"`
	Retry     *int   `json:"retry,omitempty"`
	Time      int64  `json:"time"` // milliseconds since the response headers
}

// Contract holds the contract findings of an attempt's final response.
type Contract struct {
	Violations []Violation `json:"violations"`
}

// Violation is a response not conforming to the contract (--openapi).
type Violation struct {
	InstancePath string `json:"instance_path,omitempty"`
	Kind         string `json:"kind"`
	Message      string `json:"message"`
	Operation    string `json:"operation,omitempty"`
	SpecPointer  string `json:"spec_pointer,omitempty"`
	Warning      bool   `json:"warning,omitempty"`
}

// Assert is the outcome of one assert (implicit or explicit).
type Assert struct {
	Line    int    `json:"line"`
	Message string `json:"message,omitempty"`
	Success bool   `json:"success"`
}

// Capture is a captured variable, projected through toValue.
type Capture struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// Call is one HTTP exchange (redirects produce several per entry).
type Call struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
	Timings  Timings  `json:"timings"`
}

// Request is the request side of a Call.
type Request struct {
	Cookies     []NameValue `json:"cookies"`
	Headers     []NameValue `json:"headers"`
	Method      string      `json:"method"`
	QueryString []NameValue `json:"query_string"`
	URL         string      `json:"url"`
}

// ResponseCookie is a Set-Cookie header, parsed into its attributes. A
// boolean attribute (secure, httponly) is Rust's `Option<bool>`: it is
// either true or entirely absent, so a plain bool with omitempty matches
// it exactly. max_age is a decimal string, not a number (the reference
// struct keeps it as `Option<String>`, "maybe ... should be u64" per its
// own comment).
type ResponseCookie struct {
	Domain   string `json:"domain,omitempty"`
	Expires  string `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httponly,omitempty"`
	MaxAge   string `json:"max_age,omitempty"`
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	SameSite string `json:"same_site,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Value    string `json:"value"`
}

// Response is the response side of a Call. Body is empty for `--json`
// (single line, no report directory to store bodies in) and a
// "store/<file>" reference for `--report-json` (set by BodyStore). Field
// order is alphabetical by JSON key, matching the reference serializer
// (see Result's doc comment) — "body" sorts before "cookies".
type Response struct {
	Body        string           `json:"body,omitempty"`
	Certificate *Certificate     `json:"certificate,omitempty"`
	Cookies     []ResponseCookie `json:"cookies"`
	Headers     []NameValue      `json:"headers"`
	HTTPVersion string           `json:"http_version"`
	Status      int              `json:"status"`
}

// Certificate is the server certificate of an HTTPS response; dates are
// "2006-01-02 15:04:05 UTC" and the value is PEM.
type Certificate struct {
	ExpireDate     string `json:"expire_date,omitempty"`
	Issuer         string `json:"issuer,omitempty"`
	SerialNumber   string `json:"serial_number,omitempty"`
	StartDate      string `json:"start_date,omitempty"`
	Subject        string `json:"subject,omitempty"`
	SubjectAltName string `json:"subject_alt_name,omitempty"`
	Value          string `json:"value,omitempty"`
}

// Timings are the phases of a transfer, in milliseconds (except the two
// timestamps).
type Timings struct {
	AppConnect    int64  `json:"app_connect"`
	BeginCall     string `json:"begin_call"`
	Connect       int64  `json:"connect"`
	EndCall       string `json:"end_call"`
	NameLookup    int64  `json:"name_lookup"`
	PreTransfer   int64  `json:"pre_transfer"`
	StartTransfer int64  `json:"start_transfer"`
	Total         int64  `json:"total"`
}

// BodyStore saves an HTTP response body (as transferred, before content
// decoding) and returns the path the JSON report should reference for it
// (e.g. "store/<id>_response.json"). contentType is the response's
// Content-Type header value, used to pick a file extension.
//
// Passed to JSON only by --report-json (internal/report's WriteJSON);
// --json (one line, no report directory) always passes a nil store, and
// every Response.Body is then left empty.
type BodyStore func(body []byte, contentType string) (path string, err error)

// JSON converts res to the shared --json/--report-json result shape.
// redact masks secret values everywhere a string is emitted. store is nil
// for --json; WriteJSON supplies one that saves each call's response body
// under a report's store/ directory.
func JSON(res *engine.UnitResult, redact func(string) string, store BodyStore) (Result, error) {
	redact = forResult(res, redact)
	jr := Result{
		Filename: res.File,
		Success:  res.Success,
		Time:     res.Duration.Milliseconds(),
	}
	if res.Row > 0 {
		jr.Sonde = &Sonde{Iteration: &Iteration{Row: res.Row}}
	}
	for _, c := range res.Cookies {
		jr.Cookies = append(jr.Cookies, Cookie{
			Domain: c.Domain, Expires: c.Expires, HTTPS: c.HTTPS,
			IncludeSubdomain: c.IncludeSubdomain, Name: c.Name, Path: c.Path, Value: redact(c.Value),
		})
	}
	if jr.Cookies == nil {
		jr.Cookies = []Cookie{}
	}
	for _, e := range res.Entries {
		je, err := toEntry(e, redact, store)
		if err != nil {
			return Result{}, err
		}
		jr.Entries = append(jr.Entries, je)
	}
	if jr.Entries == nil {
		jr.Entries = []Entry{}
	}
	return jr, nil
}

func toEntry(e *engine.EntryResult, redact func(string) string, store BodyStore) (Entry, error) {
	je := Entry{Index: e.Index, Line: e.Line, Time: e.TransferDuration.Milliseconds()}
	for _, c := range e.Calls {
		jc, err := toCall(c, redact, store)
		if err != nil {
			return Entry{}, err
		}
		je.Calls = append(je.Calls, jc)
	}
	if je.Calls == nil {
		je.Calls = []Call{}
	}
	je.CurlCmd = e.Curl
	if je.CurlCmd == "" {
		je.CurlCmd = "curl"
	}
	je.CurlCmd = redact(je.CurlCmd)
	for _, c := range e.Captures {
		je.Captures = append(je.Captures, Capture{Name: c.Name, Value: toValue(c.Value, redact)})
	}
	if je.Captures == nil {
		je.Captures = []Capture{}
	}
	for _, a := range e.Asserts {
		ja := Assert{Line: a.Line, Success: a.Err == nil}
		if a.Err != nil {
			ja.Message = redact(a.Err.Render())
		}
		je.Asserts = append(je.Asserts, ja)
	}
	if je.Asserts == nil {
		je.Asserts = []Assert{}
	}
	if len(e.Violations) > 0 {
		c := &Contract{}
		for _, v := range e.Violations {
			c.Violations = append(c.Violations, Violation{
				InstancePath: redact(v.InstancePath), Kind: string(v.Kind), Message: redact(v.Message),
				Operation: redact(v.Operation), SpecPointer: redact(v.SpecPointer), Warning: v.Warning,
			})
		}
		je.Sonde = &EntrySonde{Contract: c}
	}
	if n := len(e.Calls); n > 0 && e.Calls[n-1].Response != nil && e.Calls[n-1].Response.Stream != nil {
		if je.Sonde == nil {
			je.Sonde = &EntrySonde{}
		}
		je.Sonde.Stream = toStream(e.Calls[n-1].Response.Stream, redact)
	}
	if n := len(e.Calls); n > 0 && e.Calls[n-1].Response != nil && e.Calls[n-1].Response.GRPC != nil {
		if je.Sonde == nil {
			je.Sonde = &EntrySonde{}
		}
		st := e.Calls[n-1].Response.GRPC
		je.Sonde.GRPC = &GRPCStatus{Code: st.Code, Message: redact(st.Message), Status: st.Status}
	}
	return je, nil
}

func toStream(s *exchange.Stream, redact func(string) string) *Stream {
	js := &Stream{Protocol: string(s.Protocol), StopReason: string(s.StopReason), Messages: []StreamMessage{}}
	for _, m := range s.Messages {
		jm := StreamMessage{
			Binary: m.Binary, Direction: m.Direction.String(),
			Event: redact(m.Event), ID: redact(m.ID), Retry: m.Retry, Time: m.At.Milliseconds(),
		}
		if m.Binary {
			// Redacted before encoding, as a binary body is.
			jm.Data = base64.StdEncoding.EncodeToString([]byte(redact(string(m.Data))))
		} else {
			jm.Data = redact(string(m.Data))
		}
		if m.Direction == exchange.Sent {
			js.Sent++
		} else {
			js.Received++
		}
		js.Messages = append(js.Messages, jm)
	}
	return js
}

func toCall(c engine.Call, redact func(string) string, store BodyStore) (Call, error) {
	req := Request{
		Method:      c.Request.Method,
		URL:         redact(c.Request.URL),
		Cookies:     []NameValue{},
		Headers:     []NameValue{},
		QueryString: queryStringOf(c.Request.URL, redact),
	}
	for _, h := range c.Request.Headers {
		req.Headers = append(req.Headers, NameValue{Name: h.Name, Value: redact(h.Value)})
	}
	if cookieHeader, ok := c.Request.Headers.Get("Cookie"); ok {
		req.Cookies = splitCookieHeader(redact(cookieHeader))
	}

	resp := Response{Headers: []NameValue{}, Cookies: []ResponseCookie{}}
	if c.Response != nil {
		resp.HTTPVersion = c.Response.Version
		resp.Status = c.Response.Status
		for _, h := range c.Response.Headers {
			resp.Headers = append(resp.Headers, NameValue{Name: h.Name, Value: redact(h.Value)})
		}
		for _, ck := range c.Response.Cookies() {
			resp.Cookies = append(resp.Cookies, toResponseCookie(ck, redact))
		}
		if cert := c.Response.Certificate; cert != nil {
			resp.Certificate = toCertificate(cert, redact)
		}
		if store != nil {
			ct, _ := c.Response.ContentType()
			path, err := store(c.Response.Body, ct)
			if err != nil {
				return Call{}, err
			}
			resp.Body = path
		}
	}

	t := c.Timings
	return Call{
		Request:  req,
		Response: resp,
		Timings: Timings{
			AppConnect:    t.AppConnect.Milliseconds(),
			BeginCall:     t.Begin.UTC().Format("2006-01-02T15:04:05.000000Z"),
			Connect:       t.Connect.Milliseconds(),
			EndCall:       t.End.UTC().Format("2006-01-02T15:04:05.000000Z"),
			NameLookup:    t.NameLookup.Milliseconds(),
			PreTransfer:   t.PreTransfer.Milliseconds(),
			StartTransfer: t.StartTransfer.Milliseconds(),
			Total:         t.Total.Milliseconds(),
		},
	}, nil
}

// toCertificate projects a server certificate.
func toCertificate(c *exchange.CertInfo, redact func(string) string) *Certificate {
	date := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	return &Certificate{
		ExpireDate:     date(c.ExpireDate),
		Issuer:         redact(c.Issuer),
		SerialNumber:   redact(c.SerialNumber),
		StartDate:      date(c.StartDate),
		Subject:        redact(c.Subject),
		SubjectAltName: redact(c.SubjectAltName),
		Value:          redact(c.Value),
	}
}

// toValue projects a captured value the way the upstream CLI's own JSON
// capture serializer does (json/value.rs's to_json): a natural JSON type
// where one exists, base64 for bytes, a big integer as a bare (unquoted)
// JSON number, and a small `{"type": "<kind>", ...}` fallback object for
// nodeset/unit/http-response, which have no natural JSON form.
func toValue(v engine.Value, redact func(string) string) any {
	switch v.Kind() {
	case engine.ValueNull:
		return nil
	case engine.ValueBool:
		b, _ := v.Bool()
		return b
	case engine.ValueInteger:
		if i, ok := v.Int(); ok {
			return i
		}
		return json.Number(v.String())
	case engine.ValueFloat:
		f, _ := v.Float()
		return f
	case engine.ValueString:
		s, _ := v.Text()
		return redact(s)
	case engine.ValueBytes:
		b, _ := v.Bytes()
		return redact(base64.StdEncoding.EncodeToString(b))
	case engine.ValueDate:
		// Same textual form as everywhere else a date is displayed,
		// matching the upstream chrono DateTime Display trait this
		// serializer's to_string() uses.
		return v.String()
	case engine.ValueRegex:
		src, _ := v.Regex()
		return src
	case engine.ValueList:
		l := v.List()
		out := make([]any, len(l))
		for i, e := range l {
			out[i] = toValue(e, redact)
		}
		return out
	case engine.ValueObject:
		fields := v.Fields()
		out := make(orderedObject, len(fields))
		for i, f := range fields {
			out[i] = objectMember{Key: f.Key, Value: toValue(f.Value, redact)}
		}
		return out
	case engine.ValueNodeset:
		// serde_json serializes an ad-hoc map with keys in alphabetical
		// order (it is a BTreeMap without the preserve_order feature),
		// regardless of insertion order, the same rule
		// docs/architecture.md and this file's own struct field order
		// already follow for every other object.
		return orderedObject{
			{Key: "size", Value: int64(v.Len())},
			{Key: "type", Value: "nodeset"},
		}
	case engine.ValueUnit:
		return orderedObject{{Key: "type", Value: "unit"}}
	case engine.ValueRedirect:
		r, _ := v.Redirect()
		location := "None"
		if r.HasLocation {
			location = r.Location
		}
		return orderedObject{
			{Key: "location", Value: location},
			{Key: "status", Value: int64(r.Status)},
		}
	}
	return redact(v.String())
}

// orderedObject is a JSON object that marshals its members in the given
// order, unlike a Go map (which encoding/json always sorts by key):
// captured value.Object values keep the member order they were built
// with, and the upstream serializer preserves it too.
type orderedObject []objectMember

type objectMember struct {
	Key   string
	Value any
}

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalNoEscape(m.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := marshalNoEscape(m.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalNoEscape is json.Marshal without HTML escaping, matching the
// top-level encoder callers use for a Result so a nested value.Object or
// `<`, `>`, `&` in a member key comes out the same way whether it is
// nested or not.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// splitCookieHeader breaks a "Cookie: a=1; b=2" value into name/value pairs.
func splitCookieHeader(v string) []NameValue {
	var out []NameValue
	for part := range strings.SplitSeq(v, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, val, _ := strings.Cut(part, "=")
		out = append(out, NameValue{Name: strings.TrimSpace(name), Value: val})
	}
	return out
}

// toResponseCookie projects an exchange.Cookie (a parsed Set-Cookie
// header) onto the reference JSON export's cookie shape; redact is
// applied only to the cookie's own value, matching the reference
// implementation. max_age is the raw "Max-Age" attribute text, not
// re-parsed: exchange.Cookie already gives us that via Attr.
func toResponseCookie(c exchange.Cookie, redact func(string) string) ResponseCookie {
	jc := ResponseCookie{Name: c.Name, Value: redact(c.Value)}
	jc.Expires, _ = c.Attr("Expires")
	jc.MaxAge, _ = c.Attr("Max-Age")
	jc.Domain, _ = c.Attr("Domain")
	jc.Path, _ = c.Attr("Path")
	jc.Secure = c.Flag("Secure")
	jc.HTTPOnly = c.Flag("HttpOnly")
	jc.SameSite, _ = c.Attr("SameSite")
	return jc
}

// queryStringOf returns the query parameters of rawURL as ordered pairs,
// redacted like every other string field (request.query_params() ->
// ParamJson::from_param in the reference implementation).
func queryStringOf(rawURL string, redact func(string) string) []NameValue {
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return []NameValue{}
	}
	out := []NameValue{}
	for pair := range strings.SplitSeq(u.RawQuery, "&") {
		if pair == "" {
			continue
		}
		name, val, _ := strings.Cut(pair, "=")
		n, err1 := url.QueryUnescape(name)
		v, err2 := url.QueryUnescape(val)
		if err1 != nil {
			n = name
		}
		if err2 != nil {
			v = val
		}
		out = append(out, NameValue{Name: n, Value: redact(v)})
	}
	return out
}

// MarshalJSONLine encodes r the way `--json`/`--report-json` do: one
// compact JSON value, HTML escaping turned off (so `<`, `>` and `&` are
// written as-is, matching the reference serializer).
func MarshalJSONLine(r Result) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
