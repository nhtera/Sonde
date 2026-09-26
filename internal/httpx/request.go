// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// builtBody is the wire body of a request together with the Content-Type
// it implies when none was set explicitly.
type builtBody struct {
	data        []byte
	contentType string // empty: no implicit Content-Type
}

// buildBody encodes the body of spec: multipart wins over form, which wins
// over an explicit Body, matching how a request can only send one of them.
func buildBody(spec *RequestSpec) (builtBody, error) {
	switch {
	case len(spec.Multipart) > 0:
		return buildMultipartBody(spec.Multipart)
	case len(spec.Form) > 0:
		return builtBody{data: []byte(encodeParams(spec.Form)), contentType: "application/x-www-form-urlencoded"}, nil
	default:
		return builtBody{data: spec.Body.Data, contentType: spec.ImplicitContentType}, nil
	}
}

// buildMultipartBody writes params as a multipart/form-data body, in
// order; File parts carry their own Content-Type.
func buildMultipartBody(params []MultipartParam) (builtBody, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range params {
		switch {
		case p.Param != nil:
			fw, err := w.CreateFormField(p.Param.Name)
			if err != nil {
				return builtBody{}, otherError("could not build multipart body", err)
			}
			if _, err := fw.Write([]byte(p.Param.Value)); err != nil {
				return builtBody{}, otherError("could not build multipart body", err)
			}
		case p.File != nil:
			h := make(map[string][]string)
			disp := fmt.Sprintf(`form-data; name=%q; filename=%q`, p.File.Name, p.File.Filename)
			h["Content-Disposition"] = []string{disp}
			ct := p.File.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			h["Content-Type"] = []string{ct}
			fw, err := w.CreatePart(h)
			if err != nil {
				return builtBody{}, otherError("could not build multipart body", err)
			}
			if _, err := fw.Write(p.File.Data); err != nil {
				return builtBody{}, otherError("could not build multipart body", err)
			}
		}
	}
	if err := w.Close(); err != nil {
		return builtBody{}, otherError("could not build multipart body", err)
	}
	return builtBody{data: buf.Bytes(), contentType: w.FormDataContentType()}, nil
}

// encodeParams url-encodes params as an application/x-www-form-urlencoded
// or query string body: the value is percent-encoded, the name is not
// (matching curl's CURLOPT_URL_ENCODE, which only touches the value).
func encodeParams(params []Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.Name + "=" + curlEscape(p.Value)
	}
	return strings.Join(parts, "&")
}

// curlEscape percent-encodes every byte outside RFC 3986's unreserved set
// (ALPHA / DIGIT / "-" / "." / "_" / "~"), like curl_easy_escape.
func curlEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xf])
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	case c == '-' || c == '.' || c == '_' || c == '~':
		return true
	}
	return false
}

// buildURL composes the request URL: spec.URL as given, with spec.Query
// appended to its query string (curl url-encodes the value only).
func buildURL(spec *RequestSpec, pathAsIs bool) (*url.URL, error) {
	raw := spec.URL
	if len(spec.Query) > 0 {
		sep := "?"
		switch {
		case strings.HasSuffix(raw, "?"):
			sep = ""
		case strings.Contains(raw, "?"):
			sep = "&"
		}
		raw += sep + encodeParams(spec.Query)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, invalidURLError(raw, err.Error())
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		if u.Scheme != "" && strings.HasPrefix(raw, u.Scheme+"://") {
			return nil, invalidURLError(raw, "Only <http://> and <https://> schemes are supported")
		}
		return nil, invalidURLError(raw, "Missing scheme <http://> or <https://>")
	}
	if !pathAsIs {
		u.Path = removeDotSegments(u.Path)
	}
	return u, nil
}

// removeDotSegments implements RFC 3986 §5.2.4 on a URL path, the
// normalization curl applies unless --path-as-is is given.
func removeDotSegments(p string) string {
	if p == "" {
		return p
	}
	in := strings.Split(p, "/")
	out := make([]string, 0, len(in))
	for i, seg := range in {
		switch seg {
		case ".":
			// Drop; a trailing "." still yields a trailing slash.
			if i == len(in)-1 {
				out = append(out, "")
			}
		case "..":
			if len(out) > 1 {
				out = out[:len(out)-1]
			} else if len(out) == 1 && out[0] != "" {
				out = out[:0]
			}
			if i == len(in)-1 {
				out = append(out, "")
			}
		default:
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}

// headerValue looks up name (case-insensitive) in headers.
func headerValue(headers []exchange.Header, name string) (string, bool) {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value, true
		}
	}
	return "", false
}

// hasHeader reports whether name is present (case-insensitive).
func hasHeader(headers []exchange.Header, name string) bool {
	_, ok := headerValue(headers, name)
	return ok
}

// requestContext holds everything buildRequest needs beyond RequestSpec
// and Options: the running client's cookie jar, netrc and version.
type requestContext struct {
	client *Client
	now    time.Time
}

// preparedRequest is an *http.Request together with the pieces Execute
// needs to report the request as it was actually sent.
type preparedRequest struct {
	req     *http.Request
	headers []exchange.Header // in the order they were added
	body    []byte
}

// buildRequest turns spec into an *http.Request, applying implicit
// headers (Content-Type, User-Agent, Accept, Accept-Encoding, Cookie,
// Authorization) the way curl's easy handle does.
func buildRequest(ctx context.Context, spec *RequestSpec, opts *Options, cfg ClientConfig, rc requestContext) (*preparedRequest, error) {
	u, err := buildURL(spec, opts.PathAsIs)
	if err != nil {
		return nil, err
	}

	body, err := buildBody(spec)
	if err != nil {
		return nil, err
	}

	// Header order follows curl: Host, credentials from the URL, Accept,
	// cookies, the entry and command line headers, then the implicit
	// Content-Type, User-Agent, Authorization and Accept-Encoding.
	custom := make([]exchange.Header, 0, len(spec.Headers)+len(opts.Headers))
	custom = append(custom, spec.Headers...)
	custom = append(custom, opts.Headers...)
	var headers []exchange.Header
	if u.User != nil {
		if !hasHeader(custom, "Authorization") {
			password, _ := u.User.Password()
			headers = append(headers, exchange.Header{Name: "Authorization", Value: basicAuth(u.User.Username() + ":" + password)})
		}
		u.User = nil
	}
	if !hasHeader(custom, "Accept") {
		headers = append(headers, exchange.Header{Name: "Accept", Value: "*/*"})
	}
	if !hasHeader(custom, "Cookie") {
		if v := buildCookieHeader(rc, u, spec.Cookies); v != "" {
			headers = append(headers, exchange.Header{Name: "Cookie", Value: v})
		}
	}
	headers = append(headers, custom...)
	multipartType := ""
	if !hasHeader(custom, "Content-Type") && strings.HasPrefix(body.contentType, "multipart/") {
		// Sent after Content-Length, as curl does for its own form bodies.
		multipartType, body.contentType = body.contentType, ""
	}
	if !hasHeader(custom, "Content-Type") && body.contentType != "" {
		headers = append(headers, exchange.Header{Name: "Content-Type", Value: body.contentType})
	}
	if !hasHeader(custom, "User-Agent") {
		ua := opts.UserAgent
		if ua == "" {
			ua = cfg.UserAgent
		}
		if ua == "" {
			ua = "sonde/" + cfg.Version
		}
		headers = append(headers, exchange.Header{Name: "User-Agent", Value: ua})
	}
	if !hasHeader(headers, "Authorization") {
		if auth, ok, err := buildAuthorization(rc, u, opts); err != nil {
			return nil, err
		} else if ok {
			headers = append(headers, exchange.Header{Name: "Authorization", Value: auth})
		}
	}
	if opts.Compressed && !hasHeader(custom, "Accept-Encoding") {
		headers = append(headers, exchange.Header{Name: "Accept-Encoding", Value: "gzip, deflate, br"})
	}

	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}
	var bodyReader *bytes.Reader
	if len(body.data) > 0 {
		bodyReader = bytes.NewReader(body.data)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, invalidURLError(u.String(), err.Error())
	}
	if len(body.data) > 0 {
		req.ContentLength = int64(len(body.data))
	}
	for _, h := range headers {
		req.Header.Add(h.Name, h.Value)
	}
	if v := req.Header.Get("Host"); v != "" {
		req.Host = v
		req.Header.Del("Host")
	}
	sent := append([]exchange.Header{{Name: "Host", Value: hostHeader(req)}}, headers...)
	if req.ContentLength > 0 {
		sent = append(sent, exchange.Header{Name: "Content-Length", Value: fmt.Sprint(req.ContentLength)})
	}
	if multipartType != "" {
		req.Header.Set("Content-Type", multipartType)
		sent = append(sent, exchange.Header{Name: "Content-Type", Value: multipartType})
	}
	return &preparedRequest{req: req, headers: sent, body: body.data}, nil
}

// hostHeader returns the Host header value a request will be sent with.
func hostHeader(req *http.Request) string {
	if req.Host != "" {
		return req.Host
	}
	return req.URL.Host
}

// buildCookieHeader returns the jar's cookies for u followed by the
// entry's [Cookies] (both are sent, even with the same name).
func buildCookieHeader(rc requestContext, u *url.URL, extra []RequestCookie) string {
	cookies := rc.client.jar.forRequest(u, rc.now)
	for _, e := range extra {
		cookies = append(cookies, Cookie{Name: e.Name, Value: e.Value})
	}
	return cookieHeader(cookies)
}

// buildAuthorization resolves the Authorization header from --user or
// netrc; digest/ntlm/negotiate/aws-sigv4 are rejected earlier in Execute.
func buildAuthorization(rc requestContext, u *url.URL, opts *Options) (string, bool, error) {
	if opts.User != "" {
		return basicAuth(opts.User), true, nil
	}
	f, err := rc.client.netrcFor(opts)
	if err != nil {
		return "", false, err
	}
	if f == nil {
		return "", false, nil
	}
	login, password, ok := f.lookup(u.Hostname())
	if !ok {
		return "", false, nil
	}
	if isRerouted(u, opts) && !opts.NetrcAllowReroute {
		rc.client.warnOnce("netrc credentials for " + u.Hostname() +
			" not sent: the connection is rerouted (resolve, connect-to or proxy); use --netrc-allow-reroute to send them")
		return "", false, nil
	}
	return basicAuth(login + ":" + password), true, nil
}

// isRerouted reports whether the connection for u's host will actually go
// somewhere else, through resolve, connect-to or a proxy.
func isRerouted(u *url.URL, opts *Options) bool {
	if opts.Proxy != "" || environmentProxy(u, opts.NoProxy) != nil {
		return true
	}
	host, port := u.Hostname(), effectivePort(u)
	if _, _, ok := matchConnectTo(parseConnectTo(opts.ConnectTo), host, port); ok {
		return true
	}
	if _, ok := matchResolve(parseResolve(opts.Resolve), host, port); ok {
		return true
	}
	return false
}

func basicAuth(userPass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(userPass))
}
