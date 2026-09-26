// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"fmt"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// walkResult accumulates one import run's output while walkItems recurses
// through the item tree (single file: nested "items"; directory: the tree
// dirread.go built from the file system).
type walkResult struct {
	files     []convert.GeneratedFile
	skipped   []convert.Skipped
	warnings  []convert.Warning
	varLayers [][]variable // root-first, folded into the "default" environment
	dialect   syntax.Dialect
}

// walkItems visits items (already the direct children of the collection
// root or of one folder) in seq order, resolving inherited headers/auth
// (anc) and emitting one GeneratedFile per http/graphql leaf under
// pathPrefix.
func (wr *walkResult) walkItems(items []item, anc ancestorState, pathPrefix []string) {
	for _, it := range sortItems(items) {
		name := it.Info.Name
		if name == "" {
			name = "request"
		}
		fullName := joinName(pathPrefix, name)
		wr.warnings = append(wr.warnings, decodeWarnings(it.decodeErrors)...)
		switch it.Info.Type {
		case "folder":
			wr.varLayers = append(wr.varLayers, it.Request.Variables)
			wr.walkItems(it.Items, anc.extend(it.Request), append(append([]string{}, pathPrefix...), name))
		case "http":
			wr.buildHTTP(it, anc, pathPrefix, name, fullName)
		case "graphql":
			wr.buildGraphQL(it, anc, pathPrefix, name, fullName)
		case "grpc", "websocket", "script", "app":
			wr.skipped = append(wr.skipped, convert.Skipped{Name: fullName,
				Reason: fmt.Sprintf("opencollection: %s items are not requests Sonde can import", it.Info.Type)})
		default:
			wr.skipped = append(wr.skipped, convert.Skipped{Name: fullName,
				Reason: fmt.Sprintf("opencollection: unrecognized item type %q", it.Info.Type)})
		}
	}
}

func (wr *walkResult) buildHTTP(it item, anc ancestorState, pathPrefix []string, name, fullName string) {
	if it.HTTP == nil {
		wr.skipped = append(wr.skipped, convert.Skipped{Name: fullName, Reason: "opencollection: http item has no http block"})
		return
	}
	wr.varLayers = append(wr.varLayers, it.Runtime.Variables)
	h := it.HTTP

	var e syntax.EntrySpec
	e.Method = strings.ToUpper(h.Method)
	if e.Method == "" {
		e.Method = "GET"
	}
	var warns []convert.Warning
	e.URL, warns = convert.ParseText(rewritePathParams(h.URL, h.Params))
	warns = append(warns, applyHeadersParams(&e, anc, h.Headers, h.Params)...)
	resolved := resolveAuth(anc.auth, h.Auth)
	warns = append(warns, applyAuth(fullName, resolved, &e)...)

	variantBody, w := resolveVariant(fullName, h.Body)
	warns = append(warns, w...)
	warns = append(warns, buildBody(fullName, variantBody, &e)...)
	warns = append(warns, settingsWarnings(fullName, it.Settings)...)

	comments, resp, w := buildRuntime(fullName, it.Runtime)
	warns = append(warns, w...)
	e.Comments = withDocs(it.Docs, comments)
	e.Response = resp

	wr.emit(e, pathPrefix, name, fullName, warns)
}

func (wr *walkResult) buildGraphQL(it item, anc ancestorState, pathPrefix []string, name, fullName string) {
	if it.GraphQL == nil {
		wr.skipped = append(wr.skipped, convert.Skipped{Name: fullName, Reason: "opencollection: graphql item has no graphql block"})
		return
	}
	wr.varLayers = append(wr.varLayers, it.Runtime.Variables)
	g := it.GraphQL

	var e syntax.EntrySpec
	e.Method = "POST"
	var warns []convert.Warning
	e.URL, warns = convert.ParseText(rewritePathParams(g.URL, g.Params))
	warns = append(warns, applyHeadersParams(&e, anc, g.Headers, g.Params)...)
	resolved := resolveAuth(anc.auth, g.Auth)
	warns = append(warns, applyAuth(fullName, resolved, &e)...)

	var query, variables string
	if g.Body != nil {
		query, variables = g.Body.Query, g.Body.Variables
	}
	if query != "" {
		var w0 []convert.Warning
		e.Body, w0 = buildGraphQL(fullName, query, variables)
		warns = append(warns, w0...)
	}
	warns = append(warns, settingsWarnings(fullName, it.Settings)...)

	comments, resp, w := buildRuntime(fullName, it.Runtime)
	warns = append(warns, w...)
	e.Comments = withDocs(it.Docs, comments)
	e.Response = resp

	wr.emit(e, pathPrefix, name, fullName, warns)
}

// emit finishes e (BuildFile, which also validates it) and records the
// generated file, or a Skipped entry if e turned out not to render to
// valid source (a name or value no escaping rule can make safe).
func (wr *walkResult) emit(e syntax.EntrySpec, pathPrefix []string, name, fullName string, warns []convert.Warning) {
	f, err := syntax.BuildFile([]syntax.EntrySpec{e}, wr.dialect)
	if err != nil {
		wr.skipped = append(wr.skipped, convert.Skipped{Name: fullName, Reason: "opencollection: " + err.Error()})
		return
	}
	path := strings.Join(append(append([]string{}, pathPrefix...), name), "/")
	wr.files = append(wr.files, convert.GeneratedFile{Path: path, File: f})
	wr.warnings = append(wr.warnings, warns...)
}

// applyHeadersParams resolves headers (merged with the inherited anc.headers)
// and params (query -> [Query], path -> an [Options] variable default,
// mapping doc "Request fields") onto e.
func applyHeadersParams(e *syntax.EntrySpec, anc ancestorState, headers []header, params []param) []convert.Warning {
	var warns []convert.Warning
	for _, h := range mergeHeaders(anc.headers, headers) {
		key, kw := convert.ParseText(h.Name)
		val, vw := convert.ParseText(h.Value)
		warns = append(append(warns, kw...), vw...)
		e.Headers = append(e.Headers, syntax.Field{Key: key, Value: val})
	}
	for _, p := range params {
		if p.Disabled || p.Name == "" {
			continue
		}
		val, w := convert.ParseText(p.Value)
		warns = append(warns, w...)
		switch p.Type {
		case "query":
			key, kw := convert.ParseText(p.Name)
			warns = append(warns, kw...)
			e.Query = append(e.Query, syntax.Field{Key: key, Value: val})
		case "path":
			// The URL param's own {{name}} must match VariableName(p.Name)
			// exactly (rewritePathParams built it that way), so the
			// [Options] default's name is not run through ParseText here.
			e.Options = append(e.Options, syntax.VariableOption(convert.VariableName(p.Name), val))
		}
	}
	return warns
}

// rewritePathParams rewrites a ":name" URL segment to "{{name}}" for every
// path param named in params (mapping doc, "Request fields": OpenCollection
// has no [Options]-style default-value URL syntax of its own).
func rewritePathParams(url string, params []param) string {
	for _, p := range params {
		if p.Type == "path" && p.Name != "" {
			url = replaceSegmentToken(url, ":"+p.Name, "{{"+p.Name+"}}")
		}
	}
	return url
}

// replaceSegmentToken replaces every whole occurrence of old in s with
// new: old must start at the beginning of s or right after a '/', and end
// at the end of s or right before one of "/?#" — never a bare substring
// match, so a ":user" param can't also rewrite half of a ":userId"
// segment (mapping doc, "Request fields").
func replaceSegmentToken(s, old, replacement string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, old)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		before := i == 0 || s[i-1] == '/'
		after := i+len(old) == len(s) || strings.ContainsRune("/?#", rune(s[i+len(old)]))
		if before && after {
			b.WriteString(s[:i])
			b.WriteString(replacement)
		} else {
			// Not a whole-segment match: keep it and everything already
			// scanned, and keep looking after it.
			b.WriteString(s[:i+len(old)])
		}
		s = s[i+len(old):]
	}
}

// withDocs prepends d, if set, to comments as the first comment line.
func withDocs(d description, comments []string) []string {
	if d == "" {
		return comments
	}
	return append([]string{string(d)}, comments...)
}

// settingsWarnings names every settings.* field it.Settings sets: none of
// them has a Sonde equivalent (mapping doc, "Request fields"). Every field
// of settings is a bare yaml.Node, so presence is Kind != 0 regardless of
// the value's own shape.
func settingsWarnings(name string, s settings) []convert.Warning {
	var fields []string
	for _, f := range []struct {
		name string
		node yaml.Node
	}{
		{"encodeUrl", s.EncodeURL}, {"timeout", s.Timeout},
		{"followRedirects", s.FollowRedirects}, {"maxRedirects", s.MaxRedirects},
		{"omitHeaders", s.OmitHeaders},
	} {
		if f.node.Kind != 0 {
			fields = append(fields, f.name)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return []convert.Warning{{Kind: convert.WarnUnsupportedOption,
		Message: fmt.Sprintf("%s: settings (%s) have no Sonde equivalent", name, strings.Join(fields, ", "))}}
}

// sortItems orders siblings by info.seq (a positive seq wins ties over an
// absent/non-positive one), then by name, so import order (and so which
// duplicate name keeps its bare path, docs/guides/import-export.md
// "Naming") is deterministic (mapping doc, "Ordering").
func sortItems(items []item) []item {
	out := append([]item(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := out[i].Info.seqValue(), out[j].Info.seqValue()
		hi, hj := si > 0, sj > 0
		if hi != hj {
			return hi
		}
		if hi && si != sj {
			return si < sj
		}
		return out[i].Info.Name < out[j].Info.Name
	})
	return out
}

// decodeWarnings converts errs (document.decodeErrors or
// item.decodeErrors — one malformed sibling item's decode failure) to
// Warnings.
func decodeWarnings(errs []string) []convert.Warning {
	if len(errs) == 0 {
		return nil
	}
	warns := make([]convert.Warning, len(errs))
	for i, e := range errs {
		warns[i] = convert.Warning{Kind: convert.WarnUnsupported, Message: "opencollection: " + e}
	}
	return warns
}

func joinName(prefix []string, name string) string {
	if len(prefix) == 0 {
		return name
	}
	return strings.Join(prefix, "/") + "/" + name
}
