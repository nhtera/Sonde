// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

type body struct {
	Mode       string       `json:"mode"`
	Raw        string       `json:"raw"`
	Options    *bodyOptions `json:"options"`
	URLEncoded []kvParam    `json:"urlencoded"`
	FormData   []formParam  `json:"formdata"`
	File       *fileBody    `json:"file"`
	GraphQL    *graphqlBody `json:"graphql"`
	Disabled   bool         `json:"disabled"`
}

type bodyOptions struct {
	Raw *rawBodyOptions `json:"raw"`
}

type rawBodyOptions struct {
	Language string `json:"language"`
}

type kvParam struct {
	Key      flexString `json:"key"`
	Value    flexString `json:"value"`
	Disabled bool       `json:"disabled"`
}

type formParam struct {
	Key         flexString      `json:"key"`
	Value       flexString      `json:"value"`
	Type        string          `json:"type"` // "text" (default) or "file"
	Src         json.RawMessage `json:"src"`
	ContentType string          `json:"contentType"`
	Disabled    bool            `json:"disabled"`
}

// fileNames is the formdata file field's source: usually one path, but the
// schema allows several when Postman recorded a multi-file selection.
func (f formParam) fileNames() []string { return stringOrArray(f.Src) }

type fileBody struct {
	Src string `json:"src"`
}

type graphqlBody struct {
	Query     string          `json:"query"`
	Variables json.RawMessage `json:"variables"`
}

// rawContentType is the Content-Type Postman's client would send for a raw
// body of the given options.raw.language, used only when the request has
// no Content-Type header of its own.
var rawContentType = map[string]string{
	"json": "application/json", "xml": "application/xml",
	"html": "text/html", "javascript": "application/javascript", "text": "text/plain",
}

func hasContentTypeHeader(headers []header) bool {
	for _, h := range headers {
		if !h.Disabled && strings.EqualFold(strings.TrimSpace(h.Key), "content-type") {
			return true
		}
	}
	return false
}

// applyBody sets e's body from b, or leaves it unset for a disabled or
// empty body; an unsupported mode is warned about and left unset. hasCT
// reports whether the request already sets its own Content-Type header, so
// a raw body's inferred one is not added on top of it.
func (w *walker) applyBody(e *syntax.EntrySpec, b *body, hasCT bool, name string) {
	if b == nil || b.Disabled {
		return
	}
	switch b.Mode {
	case "", "raw":
		w.applyRawBody(e, b, hasCT, name)
	case "urlencoded":
		for _, f := range b.URLEncoded {
			if f.Disabled {
				continue
			}
			kt, kw := convert.ParseText(string(f.Key))
			vt, vw := convert.ParseText(string(f.Value))
			w.addWarnings(kw)
			w.addWarnings(vw)
			e.Form = append(e.Form, syntax.Field{Key: kt, Value: vt})
		}
	case "formdata":
		for _, f := range b.FormData {
			if f.Disabled {
				continue
			}
			w.applyFormField(e, f, name)
		}
	case "file":
		if b.File == nil || b.File.Src == "" {
			w.warn(convert.WarnUnsupportedBody, name+": file body has no source; skipped")
			return
		}
		st, sw := convert.ParseText(b.File.Src)
		w.addWarnings(sw)
		e.Body = syntax.FileBody(st)
	case "graphql":
		w.applyGraphQLBody(e, b.GraphQL, name)
	default:
		w.warn(convert.WarnUnsupportedBody, fmt.Sprintf("%s: body mode %q has no Sonde equivalent", name, b.Mode))
	}
}

func (w *walker) applyFormField(e *syntax.EntrySpec, f formParam, name string) {
	kt, kw := convert.ParseText(string(f.Key))
	w.addWarnings(kw)
	if f.Type != "file" {
		vt, vw := convert.ParseText(string(f.Value))
		w.addWarnings(vw)
		e.Multipart = append(e.Multipart, syntax.MultipartField{Key: kt, Value: vt})
		return
	}
	names := f.fileNames()
	if len(names) == 0 {
		w.warn(convert.WarnUnsupportedBody, fmt.Sprintf("%s: multipart field %q has no file selected; skipped", name, string(f.Key)))
		return
	}
	if len(names) > 1 {
		w.warn(convert.WarnUnsupportedBody, fmt.Sprintf("%s: multipart field %q has %d files; only %q is kept", name, string(f.Key), len(names), names[0]))
	}
	nt, nw := convert.ParseText(names[0])
	w.addWarnings(nw)
	mf := &syntax.MultipartFile{Name: nt}
	if f.ContentType != "" {
		ct, cw := convert.ParseText(f.ContentType)
		w.addWarnings(cw)
		mf.ContentType = ct
	}
	e.Multipart = append(e.Multipart, syntax.MultipartField{Key: kt, File: mf})
}

// applyRawBody builds a raw body: a JSON language whose text is exactly one
// JSON value is parsed and rebuilt with convert.JSONBody (ordered members,
// duplicates kept, {{variable}} inside a string value or key -- a key's is
// left for TextBody -- kept live); every other case, and JSON that cannot
// be rebuilt that way, is built with TextBody instead of RawTextBody so a
// {{variable}} anywhere in the text keeps substituting.
func (w *walker) applyRawBody(e *syntax.EntrySpec, b *body, hasCT bool, name string) {
	if b.Raw == "" {
		return
	}
	lang := ""
	if b.Options != nil && b.Options.Raw != nil {
		lang = strings.ToLower(strings.TrimSpace(b.Options.Raw.Language))
	}
	if !hasCT {
		if ct, ok := rawContentType[lang]; ok {
			e.Headers = append(e.Headers, syntax.KV("Content-Type", ct))
		}
	}
	if lang == "json" {
		if body, warns, ok := convert.JSONBody(b.Raw); ok {
			e.Body = body
			w.addWarnings(warns)
			return
		}
		w.warn(convert.WarnUnsupportedBody, name+": body is not valid JSON; kept as templated text")
		w.templatedBody(e, b.Raw, "json")
		return
	}
	sondeLang := ""
	if lang == "xml" {
		sondeLang = "xml"
	}
	// "text", "html", "javascript" and an unset language all have no
	// matching fenced-body language of their own: they become a plain
	// templated body (sondeLang ""), which still keeps {{variable}} live.
	w.templatedBody(e, b.Raw, sondeLang)
}

// templatedBody sets e's body to raw, with every {{variable}} in it parsed
// out and kept live via TextBody — unlike RawTextBody, which this package
// otherwise never uses for text a request actually sends, since it always
// makes a literal "{{" no longer substitute.
func (w *walker) templatedBody(e *syntax.EntrySpec, raw, lang string) {
	t, tw := convert.ParseText(raw)
	w.addWarnings(tw)
	e.Body = syntax.TextBody(t, lang)
}

// applyGraphQLBody builds a real GraphQL body via syntax.GraphQLBody: the
// query keeps its {{variable}} placeholders working, and a "variables"
// object (Postman writes it as a JSON-encoded string in most exports,
// sometimes as a plain object) is parsed and passed through as a JSON
// object of its own, not literal text. A "variables" value present but not
// a JSON object is dropped, with a warning, rather than guessed at.
func (w *walker) applyGraphQLBody(e *syntax.EntrySpec, g *graphqlBody, name string) {
	if g == nil || g.Query == "" {
		return
	}
	qt, qw := convert.ParseText(g.Query)
	w.addWarnings(qw)
	vars, ok := w.graphQLVariablesValue(g.Variables)
	if !ok {
		w.warn(convert.WarnUnsupportedBody, name+": graphql variables are not a JSON object; the body has no variables")
	}
	body, err := syntax.GraphQLBody(qt, vars)
	if err != nil {
		w.warn(convert.WarnUnsupportedBody, fmt.Sprintf("%s: %v", name, err))
		return
	}
	e.Body = body
}

// graphQLVariablesText normalizes a graphql body's "variables", written by
// Postman as a JSON-encoded string in most exports but sometimes as a plain
// object; "" (including "{}") omits the variables block entirely.
func graphQLVariablesText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		s = strings.TrimSpace(s)
	} else if v, err := json.Marshal(json.RawMessage(bytes.TrimSpace(raw))); err == nil {
		s = string(v)
	}
	if s == "" || s == "{}" || s == "null" {
		return ""
	}
	return s
}

// graphQLVariablesValue decodes a graphql body's "variables" to the object
// syntax.GraphQLBody accepts. (nil, true) means no variables were given at
// all (an empty, blank, "{}" or "null" value); (nil, false) means a
// variables value was given but is not a JSON object.
func (w *walker) graphQLVariablesValue(raw json.RawMessage) (any, bool) {
	text := graphQLVariablesText(raw)
	if text == "" {
		return nil, true
	}
	v, ok := parseJSONLoose(text)
	if !ok {
		return nil, false
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	return w.jsonToBuildValue(obj), true
}

// parseJSONLoose reports whether raw is exactly one valid JSON value (no
// trailing garbage), decoding numbers as json.Number so JSONBody can
// re-emit them without a float round trip. Only graphQLVariablesValue uses
// it now: the request body's own raw JSON goes through convert.JSONBody
// instead, for ordered members and duplicate-key handling.
func parseJSONLoose(raw string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, !dec.More()
}

// jsonToBuildValue converts a decoded JSON value (json.Number for numbers,
// map[string]any/[]any for objects/arrays) to the shape syntax.GraphQLBody
// accepts for its "variables" object, running every string through
// ParseText so a {{variable}} inside it keeps working.
func (w *walker) jsonToBuildValue(v any) any {
	switch x := v.(type) {
	case string:
		t, ws := convert.ParseText(x)
		w.addWarnings(ws)
		return t
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = w.jsonToBuildValue(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = w.jsonToBuildValue(e)
		}
		return out
	default: // nil, bool, json.Number
		return x
	}
}
