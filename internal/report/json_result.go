// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/value"
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
	Contract *Contract `json:"contract,omitempty"`
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
	Cookies     []ResponseCookie `json:"cookies"`
	Headers     []NameValue      `json:"headers"`
	HTTPVersion string           `json:"http_version"`
	Status      int              `json:"status"`
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
		je, err := toEntry(res, e, redact, store)
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

func toEntry(res *engine.UnitResult, e *engine.EntryResult, redact func(string) string, store BodyStore) (Entry, error) {
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
			ja.Message = redact(a.Err.Render(res.File, string(res.Source), e.Line))
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
	return je, nil
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

// toValue projects a captured value.Value the way the upstream CLI's own
// JSON capture serializer does (json/value.rs's to_json): a natural JSON
// type where one exists, base64 for bytes, a big integer as a bare
// (unquoted) JSON number, and a small `{"type": "<kind>", ...}` fallback
// object for nodeset/unit/http-response, which have no natural JSON form.
func toValue(v value.Value, redact func(string) string) any {
	switch v := v.(type) {
	case value.Null:
		return nil
	case value.Bool:
		return bool(v)
	case value.Int:
		return int64(v)
	case value.BigInt:
		return json.Number(string(v))
	case value.Float:
		return float64(v)
	case value.String:
		return redact(string(v))
	case value.Bytes:
		return redact(base64.StdEncoding.EncodeToString(v))
	case value.Date:
		// Same textual form as everywhere else a Date is displayed
		// (internal/value.Display), matching the upstream chrono
		// DateTime Display trait this serializer's to_string() uses.
		return value.Display(v)
	case value.Regex:
		return v.Source
	case value.List:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = toValue(e, redact)
		}
		return out
	case value.Object:
		out := make(orderedObject, len(v))
		for i, m := range v {
			out[i] = objectMember{Key: m.Key, Value: toValue(m.Value, redact)}
		}
		return out
	case value.Nodeset:
		// serde_json serializes an ad-hoc map with keys in alphabetical
		// order (it is a BTreeMap without the preserve_order feature),
		// regardless of insertion order, the same rule
		// docs/architecture.md and this file's own struct field order
		// already follow for every other object.
		return orderedObject{
			{Key: "size", Value: int64(v)},
			{Key: "type", Value: "nodeset"},
		}
	case value.Unit:
		return orderedObject{{Key: "type", Value: "unit"}}
	case value.HTTPResponse:
		location := "None"
		if v.HasLocation {
			location = v.Location
		}
		return orderedObject{
			{Key: "location", Value: location},
			{Key: "status", Value: int64(v.Status)},
		}
	}
	return redact(value.Display(v))
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
