// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// requestVarChainRE matches a REST Client request variable chain, e.g.
// "login.response.body.$.token" or "login.request.headers.Authorization":
// a reference to another request's own request or response, which Sonde has
// no equivalent for (it has no notion of one request file naming another).
var requestVarChainRE = regexp.MustCompile(`^[\w-]+\.(request|response)\.(body|headers)\b`)

// buildEntry converts one parsed request into a syntax.EntrySpec and the
// warnings its conversion produced. A non-empty skip reason means r has no
// Sonde equivalent at all; the returned EntrySpec and warnings are then
// both zero and the caller should record a convert.Skipped instead.
func buildEntry(r *request) (spec syntax.EntrySpec, warns []convert.Warning, skip string) {
	if r.method == "WEBSOCKET" {
		return syntax.EntrySpec{}, nil, "JetBrains WebSocket requests have no Sonde equivalent"
	}
	// JetBrains' GRAPHQL method is sent as a regular POST; its body (the
	// query text, plus an optional "variables" block) goes through the
	// same Content-Type-driven body handling as any other POST.
	method := r.method
	if method == "GRAPHQL" {
		method = "POST"
	}

	var comments []string

	if r.preScript != nil {
		comments = append(comments, scriptComment("pre-request script", r.preScript))
		warns = append(warns, convert.Warning{Kind: convert.WarnScript,
			Message: fmt.Sprintf("%s: pre-request script kept as a comment, never executed", entryLabel(r))})
	}
	if r.name != "" {
		comments = append(comments, "name: "+r.name)
	}
	comments = append(comments, r.comments...)
	if r.responseScript != nil {
		comments = append(comments, scriptComment("response handler", r.responseScript))
		warns = append(warns, convert.Warning{Kind: convert.WarnScript,
			Message: fmt.Sprintf("%s: response handler kept as a comment, never executed", entryLabel(r))})
	}
	for _, d := range r.unsupportedDirectives {
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedOption,
			Message: fmt.Sprintf("%s: unsupported directive %q", entryLabel(r), d)})
	}

	urlText, uw := parseRequestText(r.url)
	warns = append(warns, uw...)

	headers := make([]syntax.Field, 0, len(r.headers))
	var contentType string
	for _, h := range r.headers {
		if strings.EqualFold(h.key, "content-type") {
			contentType = h.value
		}
		kt, kw := parseRequestText(h.key)
		vt, vw := parseRequestText(h.value)
		warns = append(warns, kw...)
		warns = append(warns, vw...)
		headers = append(headers, syntax.Field{Key: kt, Value: vt})
	}

	spec = syntax.EntrySpec{Comments: comments, Method: method, URL: urlText, Headers: headers}

	if r.body != nil {
		body, form, multipart, bw := buildBody(r.body, contentType)
		spec.Body, spec.Form, spec.Multipart = body, form, multipart
		warns = append(warns, bw...)
	}

	// "max-time"/"connect-timeout" are the closest Sonde options to
	// "@timeout"/"@connection-timeout", not an exact equivalent: JetBrains
	// documents "@timeout" as an idle timeout between packets, while
	// "max-time" bounds the whole request.
	if r.hasTimeout {
		spec.Options = append(spec.Options, syntax.DurationOption("max-time", r.timeout))
	}
	if r.hasConnTimeout {
		spec.Options = append(spec.Options, syntax.DurationOption("connect-timeout", r.connTimeout))
	}
	if r.outputFile != nil {
		pathText, pw := parseRequestText(r.outputFile.path)
		warns = append(warns, pw...)
		spec.Options = append(spec.Options, syntax.FilenameOption("output", pathText))
		if !r.outputFile.overwrite {
			// JetBrains' plain ">>" never overwrites an existing file (it
			// adds a numeric suffix instead); Sonde's "output:" always
			// overwrites. Only ">>!" (overwrite=true) matches exactly.
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedOption,
				Message: fmt.Sprintf("%s: \">> %s\" never overwrites an existing file in JetBrains (it adds a "+
					"numeric suffix instead); Sonde's \"output:\" always overwrites", entryLabel(r), r.outputFile.path)})
		}
	}

	return spec, warns, ""
}

// entryLabel names r for a warning message: its "# @name"/"### name", or
// else its method and URL.
func entryLabel(r *request) string {
	if r.name != "" {
		return r.name
	}
	return r.method + " " + r.url
}

// scriptComment renders a never-executed script as one EntrySpec comment
// (writeEntry splits an embedded "\n" into separate "#" lines).
func scriptComment(label string, s *scriptBlock) string {
	if s.external != "" {
		return label + ": " + s.external + " (not executed)"
	}
	return label + " (not executed):\n" + s.inline
}

// parseRequestText builds a syntax.Text from s, as ParseText does, plus a
// WarnUnsupported warning for every "{{name}}" that looks like a REST
// Client request variable chain (referencing another request's own request
// or response), which Sonde has no equivalent for.
func parseRequestText(s string) (syntax.Text, []convert.Warning) {
	var warns []convert.Warning
	seen := map[string]bool{}
	rest := s
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			break
		}
		end := strings.Index(rest[open+2:], "}}")
		if end < 0 {
			break
		}
		name := strings.TrimSpace(rest[open+2 : open+2+end])
		if requestVarChainRE.MatchString(name) && !seen[name] {
			seen[name] = true
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
				Message: fmt.Sprintf("{{%s}} references another request's request or response; use a Sonde capture instead", name)})
		}
		rest = rest[open+2+end+2:]
	}
	text, pw := convert.ParseText(s)
	return text, append(warns, pw...)
}
