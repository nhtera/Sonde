// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// decodeBody interprets b.rawData now that b.Type is known: the schema
// gives "data" a different shape per body type (mapping doc, "Request
// fields"). A shape that does not match its type (a malformed input, never
// trusted) leaves the body empty rather than erroring; the caller still
// gets a request file, just without that body.
func decodeBody(b *body) error {
	if b.rawData.Kind == 0 {
		return nil
	}
	switch b.Type {
	case "json", "xml", "text", "sparql":
		var s string
		if b.rawData.Decode(&s) == nil {
			b.Data = s
		}
	case "form-urlencoded":
		var fields []formField
		if b.rawData.Decode(&fields) == nil {
			b.FormData = fields
		}
	case "multipart-form":
		var fields []multipartField
		if b.rawData.Decode(&fields) == nil {
			b.MultipartData = fields
		}
	case "file":
		var fields []fileField
		if b.rawData.Decode(&fields) == nil {
			b.FileData = fields
		}
	}
	return nil
}

// resolveVariant picks the body variant to import when b has a non-empty
// Variants list (mapping doc, "Request fields"): the one marked
// Selected, else the first, with a warning naming the ones dropped.
func resolveVariant(name string, b *body) (*body, []convert.Warning) {
	if b == nil || len(b.Variants) == 0 {
		return b, nil
	}
	chosen := b.Variants[0]
	for _, v := range b.Variants {
		if v.Selected {
			chosen = v
			break
		}
	}
	if len(b.Variants) > 1 {
		return chosen.Body, []convert.Warning{{Kind: convert.WarnUnsupportedBody,
			Message: fmt.Sprintf("%s: only the %q body variant is imported, %d other(s) dropped", name, chosen.Title, len(b.Variants)-1)}}
	}
	return chosen.Body, nil
}

// buildBody converts b (already variant-resolved) to a syntax.BodySpec on
// e, warning where the mapping is lossy (mapping doc, "Request fields").
func buildBody(name string, b *body, e *syntax.EntrySpec) []convert.Warning {
	if b == nil {
		return nil
	}
	var warns []convert.Warning
	switch b.Type {
	case "json":
		warns = append(warns, applyJSONBody(b.Data, e)...)
	case "xml":
		t, w := convert.ParseText(b.Data)
		warns = append(warns, w...)
		e.Body = syntax.TextBody(t, "xml")
	case "text", "sparql":
		t, w := convert.ParseText(b.Data)
		warns = append(warns, w...)
		e.Body = syntax.TextBody(t, "")
	case "form-urlencoded":
		for _, f := range b.FormData {
			if f.Disabled {
				continue
			}
			key, kw := convert.ParseText(f.Name)
			value, vw := convert.ParseText(f.Value)
			warns = append(append(warns, kw...), vw...)
			e.Form = append(e.Form, syntax.Field{Key: key, Value: value})
		}
	case "multipart-form":
		for _, f := range b.MultipartData {
			if f.Disabled {
				continue
			}
			key, kw := convert.ParseText(f.Name)
			value, vw := convert.ParseText(f.Value)
			warns = append(append(warns, kw...), vw...)
			mf := syntax.MultipartField{Key: key}
			if f.Type == "file" {
				mf.File = &syntax.MultipartFile{Name: value, ContentType: syntax.PlainText(f.ContentType)}
			} else {
				mf.Value = value
			}
			e.Multipart = append(e.Multipart, mf)
		}
	case "file":
		// Handled below, for every b.Type == "file", regardless of which
		// switch case matched (a file body's other fields are all in
		// FileData, not Data).
	case "":
		// No body.
	default:
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedBody,
			Message: fmt.Sprintf("%s: body type %q has no Sonde equivalent", name, b.Type)})
	}
	if b.Type == "file" {
		f, ok := selectedFile(b.FileData)
		if ok {
			path, w := convert.ParseText(f.FilePath)
			warns = append(warns, w...)
			e.Body = syntax.FileBody(path)
		}
		if len(b.FileData) > 1 {
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedBody,
				Message: fmt.Sprintf("%s: only the selected file is imported, %d other(s) dropped", name, len(b.FileData)-1)})
		}
	}
	return warns
}

// applyJSONBody sets e.Body from data using convert.JSONBody, which keeps
// member order and duplicate keys and expands a "{{variable}}" inside a
// string value (mapping doc, "Request fields"). When data isn't rebuildable
// that way — not exactly one JSON value, deeply nested, or holding a
// placeholder in an object key — it falls back to syntax.TextBody, which
// keeps every "{{variable}}" in data live too, just without the canonical
// re-formatting or the ordered/duplicate-key handling.
func applyJSONBody(data string, e *syntax.EntrySpec) []convert.Warning {
	if b, warns, ok := convert.JSONBody(data); ok {
		e.Body = b
		return warns
	}
	t, warns := convert.ParseText(data)
	e.Body = syntax.TextBody(t, "json")
	return warns
}

// parseJSONLoose reports whether raw is exactly one valid JSON value (no
// trailing garbage), decoding numbers as json.Number so JSONBody can
// re-emit them without a float round trip. Used only for a graphql
// item's "variables" (buildGraphQL below) — a JSON *body*'s own rebuild
// goes through the shared convert.JSONBody instead (applyJSONBody), which
// additionally keeps member order and rejects a placeholder in an object
// key; "variables" objects are small enough in practice that this
// package does not duplicate that ordering/key-safety logic a second
// time for them.
func parseJSONLoose(raw string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, !dec.More()
}

// jsonToBuildValue converts a decoded JSON value (json.Number, bool, nil,
// map[string]any, []any) to the shape syntax.GraphQLBody's "variables"
// accepts, running every string through convert.ParseText so a
// "{{variable}}" inside it becomes a real placeholder rather than literal
// text.
func jsonToBuildValue(v any) (any, []convert.Warning) {
	switch x := v.(type) {
	case string:
		return convert.ParseText(x)
	case []any:
		out := make([]any, len(x))
		var warns []convert.Warning
		for i, elem := range x {
			cv, w := jsonToBuildValue(elem)
			out[i] = cv
			warns = append(warns, w...)
		}
		return out, warns
	case map[string]any:
		out := make(map[string]any, len(x))
		var warns []convert.Warning
		for k, elem := range x {
			cv, w := jsonToBuildValue(elem)
			out[k] = cv
			warns = append(warns, w...)
		}
		return out, warns
	default: // nil, bool, json.Number
		return x, nil
	}
}

// selectedFile picks the FileData entry marked Selected, else the first.
func selectedFile(files []fileField) (fileField, bool) {
	if len(files) == 0 {
		return fileField{}, false
	}
	for _, f := range files {
		if f.Selected {
			return f, true
		}
	}
	return files[0], true
}

// buildGraphQL renders a graphql item's query and variables (if any) with
// syntax.GraphQLBody (mapping doc, "Item kinds"): a "{{variable}}" in the
// query, or inside a variables string value, stays live either as a
// ```graphql``` block or, when GraphQLBody decides the query can't be
// written that way, as the JSON body a GraphQL request sends on the wire
// regardless — never a silent loss, since both forms substitute the same
// way. variables must decode to a JSON object; anything else (invalid
// JSON, or valid JSON that isn't an object) is warned about and sent
// without variables.
func buildGraphQL(name, query, variables string) (*syntax.BodySpec, []convert.Warning) {
	q, warns := convert.ParseText(query)
	var vars any
	if strings.TrimSpace(variables) != "" {
		v, ok := parseJSONLoose(variables)
		m, isObject := v.(map[string]any)
		switch {
		case !ok:
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedBody,
				Message: name + ": graphql body variables are not valid JSON; sent without variables"})
		case !isObject:
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedBody,
				Message: name + ": graphql body variables must be a JSON object; sent without variables"})
		default:
			cv, w := jsonToBuildValue(m)
			warns = append(warns, w...)
			vars = cv
		}
	}
	spec, err := syntax.GraphQLBody(q, vars)
	if err != nil {
		// GraphQLBody only rejects a variables value that isn't Members or
		// map[string]any, which the isObject check above already excludes;
		// kept defensive.
		return syntax.TextBody(q, "graphql"), warns
	}
	return spec, warns
}
