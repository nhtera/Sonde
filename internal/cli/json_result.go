// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

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

// jsonResult is one file's --json output object. Field names, order and
// nesting reproduce the upstream 8.0.1 JSON export byte-for-byte (verified
// against testdata/conformance/hurl/tests_ok/json_output and
// tests_failed/assert_value_error): every struct's JSON keys are declared
// in the alphabetical order the reference serializer produces.
type jsonResult struct {
	Cookies  []jsonCookie `json:"cookies"`
	Entries  []jsonEntry  `json:"entries"`
	Filename string       `json:"filename"`
	Success  bool         `json:"success"`
	Time     int64        `json:"time"` // milliseconds
}

type jsonCookie struct {
	Domain           string `json:"domain"`
	Expires          int64  `json:"expires"`
	HTTPS            bool   `json:"https"`
	IncludeSubdomain bool   `json:"include_subdomain"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	Value            string `json:"value"`
}

// jsonNameValue is the shape shared by header, request-cookie and
// query-string entries.
type jsonNameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type jsonEntry struct {
	Asserts  []jsonAssert  `json:"asserts"`
	Calls    []jsonCall    `json:"calls"`
	Captures []jsonCapture `json:"captures"`
	CurlCmd  string        `json:"curl_cmd"`
	Index    int           `json:"index"`
	Line     int           `json:"line"`
	Time     int64         `json:"time"` // milliseconds
}

type jsonAssert struct {
	Line    int    `json:"line"`
	Message string `json:"message,omitempty"`
	Success bool   `json:"success"`
}

type jsonCapture struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type jsonCall struct {
	Request  jsonRequest  `json:"request"`
	Response jsonResponse `json:"response"`
	Timings  jsonTimings  `json:"timings"`
}

type jsonRequest struct {
	Cookies     []jsonNameValue `json:"cookies"`
	Headers     []jsonNameValue `json:"headers"`
	Method      string          `json:"method"`
	QueryString []jsonNameValue `json:"query_string"`
	URL         string          `json:"url"`
}

// jsonResponseCookie is a Set-Cookie header, parsed into its attributes.
// A boolean attribute (secure, httponly) is Rust's `Option<bool>`: it is
// either true or entirely absent, so a plain bool with omitempty matches
// it exactly. max_age is a decimal string, not a number (the reference
// struct keeps it as `Option<String>`, "maybe ... should be u64" per its
// own comment).
type jsonResponseCookie struct {
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

type jsonResponse struct {
	Cookies     []jsonResponseCookie `json:"cookies"`
	Headers     []jsonNameValue      `json:"headers"`
	HTTPVersion string               `json:"http_version"`
	Status      int                  `json:"status"`
}

type jsonTimings struct {
	AppConnect    int64  `json:"app_connect"`
	BeginCall     string `json:"begin_call"`
	Connect       int64  `json:"connect"`
	EndCall       string `json:"end_call"`
	NameLookup    int64  `json:"name_lookup"`
	PreTransfer   int64  `json:"pre_transfer"`
	StartTransfer int64  `json:"start_transfer"`
	Total         int64  `json:"total"`
}

// toJSONResult converts an engine.UnitResult to the --json/--report-json
// shape. redact masks secret values everywhere text is emitted.
func toJSONResult(res *engine.UnitResult, redact func(string) string) jsonResult {
	jr := jsonResult{
		Filename: res.File,
		Success:  res.Success,
		Time:     res.Duration.Milliseconds(),
	}
	for _, c := range res.Cookies {
		jr.Cookies = append(jr.Cookies, jsonCookie{
			Domain: c.Domain, Expires: c.Expires, HTTPS: c.HTTPS,
			IncludeSubdomain: c.IncludeSubdomain, Name: c.Name, Path: c.Path, Value: redact(c.Value),
		})
	}
	if jr.Cookies == nil {
		jr.Cookies = []jsonCookie{}
	}
	for _, e := range res.Entries {
		jr.Entries = append(jr.Entries, toJSONEntry(res, e, redact))
	}
	if jr.Entries == nil {
		jr.Entries = []jsonEntry{}
	}
	return jr
}

func toJSONEntry(res *engine.UnitResult, e *engine.EntryResult, redact func(string) string) jsonEntry {
	je := jsonEntry{Index: e.Index, Line: e.Line, Time: e.TransferDuration.Milliseconds()}
	for _, c := range e.Calls {
		je.Calls = append(je.Calls, toJSONCall(c, redact))
	}
	if je.Calls == nil {
		je.Calls = []jsonCall{}
	}
	je.CurlCmd = e.Curl
	if je.CurlCmd == "" {
		je.CurlCmd = "curl"
	}
	je.CurlCmd = redact(je.CurlCmd)
	for _, c := range e.Captures {
		je.Captures = append(je.Captures, jsonCapture{Name: c.Name, Value: toJSONValue(c.Value, redact)})
	}
	if je.Captures == nil {
		je.Captures = []jsonCapture{}
	}
	seen := map[int]bool{}
	for _, err := range e.Errors {
		seen[err.Span.Start.Line] = true
	}
	for _, a := range e.Asserts {
		ja := jsonAssert{Line: a.Line, Success: a.Err == nil}
		if a.Err != nil {
			ja.Message = redact(a.Err.Render(res.File, string(res.Source), e.Line))
		}
		je.Asserts = append(je.Asserts, ja)
	}
	if je.Asserts == nil {
		je.Asserts = []jsonAssert{}
	}
	return je
}

func toJSONCall(c engine.Call, redact func(string) string) jsonCall {
	req := jsonRequest{
		Method:      c.Request.Method,
		URL:         redact(c.Request.URL),
		Cookies:     []jsonNameValue{},
		Headers:     []jsonNameValue{},
		QueryString: queryStringOf(c.Request.URL, redact),
	}
	for _, h := range c.Request.Headers {
		req.Headers = append(req.Headers, jsonNameValue{Name: h.Name, Value: redact(h.Value)})
	}
	if cookieHeader, ok := c.Request.Headers.Get("Cookie"); ok {
		req.Cookies = splitCookieHeader(redact(cookieHeader))
	}

	resp := jsonResponse{Headers: []jsonNameValue{}, Cookies: []jsonResponseCookie{}}
	if c.Response != nil {
		resp.HTTPVersion = c.Response.Version
		resp.Status = c.Response.Status
		for _, h := range c.Response.Headers {
			resp.Headers = append(resp.Headers, jsonNameValue{Name: h.Name, Value: redact(h.Value)})
		}
		for _, ck := range c.Response.Cookies() {
			resp.Cookies = append(resp.Cookies, toJSONResponseCookie(ck, redact))
		}
	}

	t := c.Timings
	return jsonCall{
		Request:  req,
		Response: resp,
		Timings: jsonTimings{
			AppConnect:    t.AppConnect.Milliseconds(),
			BeginCall:     t.Begin.UTC().Format("2006-01-02T15:04:05.000000Z"),
			Connect:       t.Connect.Milliseconds(),
			EndCall:       t.End.UTC().Format("2006-01-02T15:04:05.000000Z"),
			NameLookup:    t.NameLookup.Milliseconds(),
			PreTransfer:   t.PreTransfer.Milliseconds(),
			StartTransfer: t.StartTransfer.Milliseconds(),
			Total:         t.Total.Milliseconds(),
		},
	}
}

// toJSONValue projects a captured value.Value the way the upstream CLI's
// own JSON capture serializer does (json/value.rs's to_json): a natural
// JSON type where one exists, base64 for bytes, a big integer as a bare
// (unquoted) JSON number, and a small `{"type": "<kind>", ...}` fallback
// object for nodeset/unit/http-response, which have no natural JSON form.
func toJSONValue(v value.Value, redact func(string) string) any {
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
			out[i] = toJSONValue(e, redact)
		}
		return out
	case value.Object:
		out := make(orderedJSONObject, len(v))
		for i, m := range v {
			out[i] = jsonMember{Key: m.Key, Value: toJSONValue(m.Value, redact)}
		}
		return out
	case value.Nodeset:
		// serde_json serializes an ad-hoc map with keys in alphabetical
		// order (it is a BTreeMap without the preserve_order feature),
		// regardless of insertion order, the same rule
		// docs/architecture.md and this file's own struct field order
		// already follow for every other object.
		return orderedJSONObject{
			{Key: "size", Value: int64(v)},
			{Key: "type", Value: "nodeset"},
		}
	case value.Unit:
		return orderedJSONObject{{Key: "type", Value: "unit"}}
	case value.HTTPResponse:
		location := "None"
		if v.HasLocation {
			location = v.Location
		}
		return orderedJSONObject{
			{Key: "location", Value: location},
			{Key: "status", Value: int64(v.Status)},
		}
	}
	return redact(value.Display(v))
}

// orderedJSONObject is a JSON object that marshals its members in the
// given order, unlike a Go map (which encoding/json always sorts by key):
// captured value.Object values keep the member order they were built
// with, and the upstream serializer preserves it too.
type orderedJSONObject []jsonMember

type jsonMember struct {
	Key   string
	Value any
}

func (o orderedJSONObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalJSONNoEscape(m.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := marshalJSONNoEscape(m.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalJSONNoEscape is json.Marshal without HTML escaping, matching the
// top-level encoder in writeJSONLine so a nested value.Object or `<`, `>`,
// `&` in a member key comes out the same way whether it is nested or not.
func marshalJSONNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// splitCookieHeader breaks a "Cookie: a=1; b=2" value into name/value pairs.
func splitCookieHeader(v string) []jsonNameValue {
	var out []jsonNameValue
	for part := range strings.SplitSeq(v, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, val, _ := strings.Cut(part, "=")
		out = append(out, jsonNameValue{Name: strings.TrimSpace(name), Value: val})
	}
	return out
}

// toJSONResponseCookie projects an exchange.Cookie (a parsed Set-Cookie
// header) onto the reference JSON export's cookie shape; redact is
// applied only to the cookie's own value, matching the reference
// implementation. max_age is the raw "Max-Age" attribute text, not
// re-parsed: exchange.Cookie already gives us that via Attr.
func toJSONResponseCookie(c exchange.Cookie, redact func(string) string) jsonResponseCookie {
	jc := jsonResponseCookie{Name: c.Name, Value: redact(c.Value)}
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
func queryStringOf(rawURL string, redact func(string) string) []jsonNameValue {
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return []jsonNameValue{}
	}
	out := []jsonNameValue{}
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
		out = append(out, jsonNameValue{Name: n, Value: redact(v)})
	}
	return out
}
