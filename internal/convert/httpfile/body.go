// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// buildBody converts a parsed body into a syntax.BodySpec (JSON, XML,
// GraphQL, file or raw text), [Form] fields (application/x-www-form-
// urlencoded) or [Multipart] fields (a simple multipart/form-data body);
// exactly one of the first two return values, or the third, is set. Every
// "{{name}}" placeholder in the body stays a live Sonde variable.
func buildBody(b *rawBody, contentType string) (*syntax.BodySpec, []syntax.Field, []syntax.MultipartField, []convert.Warning) {
	switch b.kind {
	case bodyFileRef:
		pathText, warns := parseRequestText(b.path)
		return syntax.FileBody(pathText), nil, nil, warns
	case bodyFileRefSubstituted:
		pathText, warns := parseRequestText(b.path)
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedBody,
			Message: "\"<@ " + b.path + "\" has the source tool substitute its own variables inside the file " +
				"before sending; Sonde's file body sends it as is, so those \"{{...}}\" are not substituted"})
		return syntax.FileBody(pathText), nil, nil, warns
	}

	base, params := splitContentType(contentType)
	switch {
	case base == "application/x-www-form-urlencoded":
		form, warns := buildForm(b.text)
		return nil, form, nil, warns
	case strings.HasPrefix(base, "multipart/"):
		if fields, warns, ok := buildMultipart(b.text, params); ok {
			return nil, nil, fields, warns
		}
		return templatedBody(b.text, "")
	case base == "" && looksLikeJSON(b.text), strings.Contains(base, "json"):
		return buildJSONBody(b.text)
	case base == "application/xml", base == "text/xml", strings.HasSuffix(base, "+xml"):
		return templatedBody(b.text, "xml")
	default:
		// Includes "application/graphql": its body is the raw query text,
		// sent as is (never JSON-wrapped the way a ```graphql``` fence
		// would render it); a request whose GraphQL query and variables
		// arrive as JSON already matches the "json" case above instead.
		return templatedBody(b.text, "")
	}
}

// splitContentType splits a Content-Type header value into its base type,
// lowercased, and its raw parameter string (everything after the first
// ';'), e.g. "multipart/form-data; boundary=X" -> ("multipart/form-data",
// "boundary=X").
func splitContentType(ct string) (base, params string) {
	base, params, _ = strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(base)), strings.TrimSpace(params)
}

// looksLikeJSON reports whether text, with no Content-Type to go by, is
// plausibly a JSON body worth trying to parse as one.
func looksLikeJSON(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}

// templatedBody builds a body from text via syntax.TextBody, keeping every
// "{{name}}" placeholder in it live regardless of where it appears (unlike
// syntax.RawTextBody, TextBody takes a syntax.Text, so it can always tell a
// literal brace from a placeholder); lang is "", "xml" or "graphql".
func templatedBody(text, lang string) (*syntax.BodySpec, []syntax.Field, []syntax.MultipartField, []convert.Warning) {
	t, warns := parseRequestText(text)
	return syntax.TextBody(t, lang), nil, nil, warns
}

// buildForm parses an application/x-www-form-urlencoded body into [Form]
// fields.
func buildForm(text string) ([]syntax.Field, []convert.Warning) {
	var fields []syntax.Field
	var warns []convert.Warning
	for _, pair := range strings.Split(strings.TrimSpace(text), "&") {
		if pair == "" {
			continue
		}
		key, val, _ := strings.Cut(pair, "=")
		kt, kw := parseRequestText(formUnescape(key))
		vt, vw := parseRequestText(formUnescape(val))
		warns = append(warns, kw...)
		warns = append(warns, vw...)
		fields = append(fields, syntax.Field{Key: kt, Value: vt})
	}
	return fields, warns
}

// formUnescape decodes application/x-www-form-urlencoded percent-escapes
// and "+", falling back to s unchanged (rather than an error) on anything
// malformed, since "{{" and "}}" are never percent-encoded in a .http file.
func formUnescape(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if hi, ok := hexDigit(s[i+1]); ok {
				if lo, ok := hexDigit(s[i+2]); ok {
					b.WriteByte(hi<<4 | lo)
					i += 2
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}

// buildJSONBody rebuilds text as a JSON body via the shared convert.JSONBody
// (ordered members, duplicates kept, every string's "{{name}}" placeholders
// live). It falls back to templatedBody when text isn't exactly one JSON
// value, nests too deep, or has a placeholder in an object key — still
// keeping the placeholder live even though the content around it isn't
// valid JSON.
func buildJSONBody(text string) (*syntax.BodySpec, []syntax.Field, []syntax.MultipartField, []convert.Warning) {
	if body, warns, ok := convert.JSONBody(text); ok {
		return body, nil, nil, warns
	}
	return templatedBody(text, "json")
}
