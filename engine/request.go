// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/base64"
	neturl "net/url"
	"os"
	"path"
	"strings"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/httpx"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
)

// buildRequest renders the request of an entry.
func (u *unit) buildRequest(req *syntax.Request) (*httpx.RequestSpec, error) {
	url, err := u.env.Render(req.URL)
	if err != nil {
		return nil, err
	}
	// forExport (only ever set by RenderCurl, see curl_export.go) skips
	// this check: a run always sends the URL it renders, so an invalid
	// one must fail there, but an export never sends anything, and a
	// URL that is only invalid because of an undefined variable's
	// literal {{name}} placeholder (see template.Env.Missing) is still
	// worth printing as the curl command it would be, once that
	// variable is actually set.
	if reason := checkURL(url); reason != "" && !u.forExport {
		e := runerr.New(req.URL.Span, runerr.InvalidURL, false)
		e.Value, e.Reason = url, reason
		return nil, e
	}
	if parsed, err := neturl.Parse(url); err == nil && parsed.User != nil {
		pass, _ := parsed.User.Password()
		u.protectCredentials(parsed.User.Username() + ":" + pass)
	}
	spec := &httpx.RequestSpec{Method: req.Method.Value, URL: url}
	for _, kv := range req.Headers {
		name, value, err := u.keyValue(kv)
		if err != nil {
			return nil, err
		}
		spec.Headers = append(spec.Headers, exchange.Header{Name: name, Value: value})
	}
	for _, s := range req.Sections {
		switch s.Kind {
		case syntax.SectionBasicAuth:
			if len(s.KeyValues) == 0 {
				continue
			}
			user, password, err := u.keyValue(s.KeyValues[0])
			if err != nil {
				return nil, err
			}
			u.protectCredentials(user + ":" + password)
			auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
			spec.Headers = append(spec.Headers, exchange.Header{Name: "Authorization", Value: "Basic " + auth})
		}
	}
	for _, s := range req.Sections {
		for _, kv := range s.KeyValues {
			if s.Kind == syntax.SectionBasicAuth {
				continue
			}
			name, value, err := u.keyValue(kv)
			if err != nil {
				return nil, err
			}
			switch s.Kind {
			case syntax.SectionQueryParams:
				spec.Query = append(spec.Query, httpx.Param{Name: name, Value: value})
			case syntax.SectionFormParams:
				spec.Form = append(spec.Form, httpx.Param{Name: name, Value: value})
			case syntax.SectionCookies:
				spec.Cookies = append(spec.Cookies, httpx.RequestCookie{Name: name, Value: value})
			}
		}
	}
	if req.Body != nil {
		if spec.Body, err = u.body(req.Body.Value); err != nil {
			return nil, err
		}
	}
	for _, s := range req.Sections {
		if s.Kind != syntax.SectionMultipart {
			continue
		}
		for _, p := range s.Multipart {
			part, err := u.multipartParam(p)
			if err != nil {
				return nil, err
			}
			spec.Multipart = append(spec.Multipart, part)
		}
	}
	spec.ImplicitContentType = implicitContentType(spec, req.Body)
	return spec, nil
}

// checkURL returns why a URL is not an absolute http(s) URL, or "".
func checkURL(url string) string {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return ""
	}
	if scheme, _, ok := strings.Cut(url, "://"); ok && scheme != "" && strings.Trim(scheme, "abcdefghijklmnopqrstuvwxyz") == "" {
		return "Only <http://> and <https://> schemes are supported"
	}
	return "Missing scheme <http://> or <https://>"
}

// readBodyFile reads a request body's referenced file (a FileRef body or a
// multipart file field) exactly like u.env.File, except that RenderCurl
// (forExport, see curl_export.go) never opens it blindly: a Stat check
// first — which, unlike opening the file, cannot block — refuses a FIFO
// or other special file with the same "file can not be read" error a run
// would give for one it genuinely couldn't read, instead of hanging an
// export a caller has no way to cancel. A real run always reads the file
// directly, unaffected. A file this check can't even resolve (a bad
// template, or one outside the sandbox root) falls through to env.File,
// which reports that exactly as a run would.
func (u *unit) readBodyFile(name *syntax.Template) ([]byte, error) {
	if u.forExport {
		if path, err := u.env.Render(name); err == nil {
			if abs, perr := u.root.Path(path); perr == nil {
				if info, serr := os.Stat(abs); serr != nil || !info.Mode().IsRegular() {
					rerr := runerr.New(name.Span, runerr.FileReadAccess, false)
					rerr.Value = path
					return nil, rerr
				}
			}
		}
	}
	return u.env.File(name)
}

func (u *unit) keyValue(kv *syntax.KeyValue) (string, string, error) {
	name, err := u.env.Render(kv.Key)
	if err != nil {
		return "", "", err
	}
	v, err := u.env.Render(kv.Value)
	return name, v, err
}

// body renders a request body.
func (u *unit) body(b syntax.Bytes) (httpx.Body, error) {
	switch b := b.(type) {
	case *syntax.Template:
		if b.Delimiter == '"' { // a JSON string
			s, err := u.env.RenderJSON(b, true)
			return httpx.Body{Kind: httpx.BodyText, Data: []byte(s)}, err
		}
		s, err := u.env.Render(b)
		return httpx.Body{Kind: httpx.BodyText, Data: []byte(s)}, err
	case *syntax.MultilineString:
		s, err := u.env.RenderMultiline(b)
		return httpx.Body{Kind: httpx.BodyText, Data: []byte(s)}, err
	case *syntax.XML:
		return httpx.Body{Kind: httpx.BodyText, Data: []byte(b.Value)}, nil
	case *syntax.Base64:
		return httpx.Body{Kind: httpx.BodyBinary, Data: b.Value}, nil
	case *syntax.Hex:
		return httpx.Body{Kind: httpx.BodyBinary, Data: b.Value}, nil
	case *syntax.FileRef:
		data, err := u.readBodyFile(b.Filename)
		if err != nil {
			return httpx.Body{}, err
		}
		name, err := u.env.Render(b.Filename)
		return httpx.Body{Kind: httpx.BodyFile, Data: data, Filename: name}, err
	case syntax.JSONValue:
		s, err := u.env.RenderJSON(b, true)
		return httpx.Body{Kind: httpx.BodyText, Data: []byte(s)}, err
	}
	return httpx.Body{}, nil
}

func (u *unit) multipartParam(p syntax.MultipartParam) (httpx.MultipartParam, error) {
	switch p := p.(type) {
	case *syntax.KeyValue:
		name, v, err := u.keyValue(p)
		if err != nil {
			return httpx.MultipartParam{}, err
		}
		return httpx.MultipartParam{Param: &httpx.Param{Name: name, Value: v}}, nil
	case *syntax.FilenameParam:
		name, err := u.env.Render(p.Key)
		if err != nil {
			return httpx.MultipartParam{}, err
		}
		filename, err := u.env.Render(p.Value.Filename)
		if err != nil {
			return httpx.MultipartParam{}, err
		}
		data, err := u.readBodyFile(p.Value.Filename)
		if err != nil {
			return httpx.MultipartParam{}, err
		}
		ct := contentTypeByExtension(filename)
		if p.Value.ContentType != nil {
			if ct, err = u.env.Render(p.Value.ContentType); err != nil {
				return httpx.MultipartParam{}, err
			}
		}
		return httpx.MultipartParam{File: &httpx.FileParam{Name: name, Filename: filename, Data: data, ContentType: ct}}, nil
	}
	return httpx.MultipartParam{}, nil
}

// contentTypeByExtension guesses the content type of a multipart file.
func contentTypeByExtension(name string) string {
	switch strings.TrimPrefix(path.Ext(name), ".") {
	case "gif":
		return "image/gif"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "svg":
		return "image/svg+xml"
	case "txt":
		return "text/plain"
	case "htm", "html":
		return "text/html"
	case "pdf":
		return "application/pdf"
	case "xml":
		return "application/xml"
	}
	return "application/octet-stream"
}

// implicitContentType is the Content-Type implied by the request content.
func implicitContentType(spec *httpx.RequestSpec, body *syntax.Body) string {
	switch {
	case len(spec.Form) > 0:
		return "application/x-www-form-urlencoded"
	case len(spec.Multipart) > 0:
		return "multipart/form-data"
	case body == nil:
		return ""
	}
	switch b := body.Value.(type) {
	case *syntax.XML:
		return "application/xml"
	case *syntax.MultilineString:
		switch b.Kind {
		case syntax.MultilineJSON, syntax.MultilineGraphQL:
			return "application/json"
		case syntax.MultilineXML:
			return "application/xml"
		}
	case *syntax.Template:
		if b.Delimiter == '"' {
			return "application/json"
		}
	case syntax.JSONValue:
		return "application/json"
	}
	return ""
}

// cookieCommands returns the `@cookie_storage_set:` values and whether
// `@cookie_storage_clear` appears in the comments before a request.
func cookieCommands(req *syntax.Request) (set []string, clearAll bool) {
	for _, lt := range req.LineTerminators {
		if lt.Comment == nil {
			continue
		}
		c := lt.Comment.Value
		// The value starts at a fixed offset of the comment text (after
		// `#`), as in `# @cookie_storage_set: <cookie>`.
		const offset = len("#@cookie_storage_set:")
		if strings.Contains(c, "@cookie_storage_set:") && set == nil && len(c) >= offset {
			set = []string{strings.TrimSpace(c[offset:])}
		}
		if strings.Contains(c, "@cookie_storage_clear") {
			clearAll = true
		}
	}
	return set, clearAll
}
