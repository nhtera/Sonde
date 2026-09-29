// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package postman imports a Postman Collection v2.1 (or v2.0, which the
// same schema also parses) into Sonde request files: folders become
// directories, collection and environment variables become a sonde.yaml
// skeleton, and pre-request/test scripts are kept as comments, never
// executed. See docs/guides/migrate-from-postman.md for the mapping this
// package implements.
package postman

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// Group values of Options.Group.
const (
	GroupRequest = "request"
	GroupFolder  = "folder"
)

// EnvironmentFile is one `--environment FILE` given to Import: the file's
// name (used to derive a fallback environment name, and in warnings) and
// its raw Postman environment JSON.
type EnvironmentFile struct {
	FileName string
	Data     []byte
}

// Options configure Import.
type Options struct {
	// Group lays files out one per request (GroupRequest, the default) or
	// one per folder, chaining its requests in order (GroupFolder).
	Group        string
	Environments []EnvironmentFile
	Dialect      syntax.Dialect
}

// collection is the top-level Postman Collection v2.1 document (schema
// https://schema.postman.com/json/collection/v2.1.0/collection.json); only
// the fields Import uses are kept, and every field tolerates the shapes
// real exports vary on (a string or an object, present or absent).
type collection struct {
	Info     collectionInfo `json:"info"`
	Item     []item         `json:"item"`
	Auth     *auth          `json:"auth"`
	Event    []event        `json:"event"`
	Variable []variable     `json:"variable"`
}

type collectionInfo struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

// item is one entry of an "item" array: either a request (Request set) or
// a folder (Item set, possibly empty).
type item struct {
	Name        string     `json:"name"`
	Description flexString `json:"description"`
	Item        []item     `json:"item"`
	Request     *request   `json:"request"`
	Event       []event    `json:"event"`
	Variable    []variable `json:"variable"`
	Auth        *auth      `json:"auth"`
	Disabled    bool       `json:"disabled"`
}

func (it item) isFolder() bool { return it.Request == nil }

type request struct {
	Method      string          `json:"method"`
	Header      headerList      `json:"header"`
	Body        *body           `json:"body"`
	URL         json.RawMessage `json:"url"`
	Auth        *auth           `json:"auth"`
	Description flexString      `json:"description"`
}

// UnmarshalJSON accepts request's usual object shape, and the bare-URL
// string both v2.0 and v2.1 also allow (equivalent to {"method": "GET",
// "url": the string}).
func (r *request) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		raw, err := json.Marshal(s)
		if err != nil {
			return err
		}
		*r = request{Method: "GET", URL: raw}
		return nil
	}
	type alias request // avoid recursing back into this UnmarshalJSON
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*r = request(a)
	return nil
}

type header struct {
	Key      string     `json:"key"`
	Value    flexString `json:"value"`
	Disabled bool       `json:"disabled"`
}

// headerList accepts the usual array of {key, value} objects, and the bare
// "Key: value\nKey2: value2" string the schema also allows.
type headerList []header

func (h *headerList) UnmarshalJSON(b []byte) error {
	var arr []header
	if json.Unmarshal(b, &arr) == nil {
		*h = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	var out []header
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out = append(out, header{Key: strings.TrimSpace(key), Value: flexString(strings.TrimSpace(value))})
	}
	*h = out
	return nil
}

type variable struct {
	Key      string     `json:"key"`
	Value    flexString `json:"value"`
	Type     string     `json:"type"`
	Disabled bool       `json:"disabled"`
}

type event struct {
	Listen   string  `json:"listen"`
	Script   *script `json:"script"`
	Disabled bool    `json:"disabled"`
}

type script struct {
	Exec json.RawMessage `json:"exec"`
}

// flexString unmarshals a Postman field written as a string, a number, a
// boolean or an object ({content: "..."} for a description, most often) as
// plain text, so the tolerant structs above never fail to parse over an
// unexpected shape.
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = flexString(stringify(v))
	return nil
}

func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case map[string]any:
		if c, ok := x["content"].(string); ok {
			return c
		}
		if c, ok := x["value"].(string); ok {
			return c
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	// A bare JSON string re-encodes with quotes; every other scalar or
	// object is used as is.
	var s string
	if json.Unmarshal(b, &s) == nil {
		return s
	}
	return string(b)
}

// walker accumulates the Output of one Import: generated files, warnings,
// skipped items and the variables the sonde.yaml skeleton is built from.
type walker struct {
	dialect     syntax.Dialect
	out         convert.Output
	collVars    map[string]string
	collSecrets map[string]bool
	warned      map[string]bool
	// varNames maps a sanitized (VariableName) collection/folder variable
	// name to the first raw Postman name that produced it, to warn when a
	// second, different raw name collides with it.
	varNames map[string]string
	// pending holds a collection's or folder's script comments, in
	// --group request mode, until the next request built successfully:
	// that request has no chained file of its own to attach them to
	// (--group folder does), so they land on the first request they would
	// apply to instead, per docs/guides/migrate-from-postman.md.
	pending []string
	// taken holds every sanitized variable name the collection, its
	// folders and the --environment files define, collected before the
	// walk; pathVars holds the literal values of path variables (:id)
	// turned into {{id}}, added to every environment by finish. A path
	// variable never takes a name from taken, so its value cannot be
	// replaced by an unrelated variable of the same name.
	taken    map[string]bool
	pathVars map[string]string
}

func newWalker(dialect syntax.Dialect) *walker {
	return &walker{
		dialect: dialect, collVars: map[string]string{}, collSecrets: map[string]bool{},
		warned: map[string]bool{}, varNames: map[string]string{},
		taken: map[string]bool{}, pathVars: map[string]string{},
	}
}

// reserveNames records in w.taken every variable name items (recursively)
// and envFiles define. An environment file that fails to parse is skipped
// here: finish reports its error.
func (w *walker) reserveNames(vars []variable, items []item, envFiles []EnvironmentFile) {
	for _, v := range vars {
		w.taken[convert.VariableName(v.Key)] = true
	}
	for _, it := range items {
		w.reserveNames(it.Variable, it.Item, nil)
	}
	for _, ef := range envFiles {
		var env envFile
		if json.Unmarshal(ef.Data, &env) != nil {
			continue
		}
		for _, v := range env.Values {
			w.taken[convert.VariableName(v.Key)] = true
		}
	}
}

// checkNameCollision warns when raw and an earlier, different raw name of
// the same kind ("variable", ...) both sanitize to name.
func (w *walker) checkNameCollision(kind, raw, name string) {
	if prev, ok := w.varNames[name]; ok {
		if prev != raw {
			w.warn(convert.WarnUnsupported, fmt.Sprintf("%s %q and %q both become %q; the later one wins", kind, prev, raw, name))
		}
		return
	}
	w.varNames[name] = raw
}

func (w *walker) warn(kind, msg string) {
	key := kind + "\x00" + msg
	if w.warned[key] {
		return
	}
	w.warned[key] = true
	w.out.Warnings = append(w.out.Warnings, convert.Warning{Kind: kind, Message: msg})
}

func (w *walker) addWarnings(ws []convert.Warning) {
	for _, x := range ws {
		w.warn(x.Kind, x.Message)
	}
}

func (w *walker) skip(name, reason string) {
	w.out.Skipped = append(w.out.Skipped, convert.Skipped{Name: name, Reason: reason})
}

// mergeVariables folds a collection or folder "variable" array into the
// flat variable set the sonde.yaml skeleton is built from: a folder's
// variable overrides one of the same name from the collection or an
// earlier sibling, since Sonde environments have no per-directory scope
// (docs/guides/migrate-from-postman.md documents this simplification). A
// secret-typed variable's value is never merged in: only its name is
// recorded, for a secrets stub.
func (w *walker) mergeVariables(vars []variable) {
	for _, v := range vars {
		if v.Disabled {
			continue
		}
		name := convert.VariableName(v.Key)
		w.checkNameCollision("variable", v.Key, name)
		if strings.EqualFold(v.Type, "secret") {
			w.collSecrets[name] = true
			delete(w.collVars, name)
			w.warn(convert.WarnSecret, fmt.Sprintf("collection variable %q is a secret; add its value to the environment's secrets file", v.Key))
			continue
		}
		w.collVars[name] = string(v.Value)
	}
}

func joinPath(segs []string) string { return strings.Join(segs, "/") }

func appended(segs []string, s string) []string {
	out := make([]string, len(segs), len(segs)+1)
	copy(out, segs)
	return append(out, s)
}

// Import converts a Postman Collection v2.1 document (v2.0 parses the same
// way) to Sonde request files, never executing pre-request/test scripts and
// never panicking on malformed input.
func Import(data []byte, opts Options) (out convert.Output, err error) {
	defer func() {
		if p := recover(); p != nil {
			out, err = convert.Output{}, fmt.Errorf("postman: malformed collection: %v", p)
		}
	}()

	group := opts.Group
	switch group {
	case "":
		group = GroupRequest
	case GroupRequest, GroupFolder:
	default:
		return convert.Output{}, fmt.Errorf("postman: --group must be %q or %q, got %q", GroupRequest, GroupFolder, group)
	}

	col, err := parseCollection(data)
	if err != nil {
		return convert.Output{}, err
	}

	w := newWalker(opts.Dialect)
	w.reserveNames(col.Variable, col.Item, opts.Environments)
	w.mergeVariables(col.Variable)
	root := authState{}.resolve(col.Auth)

	switch group {
	case GroupRequest:
		w.warnEvents("collection", col.Event)
		w.pending = append(w.pending, scriptComments(col.Event)...)
		w.importRequests(col.Item, nil, root)
	case GroupFolder:
		name := strings.TrimSpace(col.Info.Name)
		if name == "" {
			name = "collection"
		}
		w.warnEvents("collection", col.Event)
		w.importFolder(col.Item, nil, root, name, scriptComments(col.Event))
	}

	if err := w.finish(opts.Environments); err != nil {
		return convert.Output{}, err
	}
	return w.out, nil
}

// parseCollection unmarshals data, rejecting the legacy Collection v1
// shape (a top-level "requests" array, no "item") with a clear error.
func parseCollection(data []byte) (*collection, error) {
	var probe struct {
		Info     *collectionInfo `json:"info"`
		Requests json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("postman: invalid JSON: %w", err)
	}
	if probe.Info == nil {
		return nil, fmt.Errorf("postman: not a Postman collection (missing \"info\")")
	}
	if probe.Requests != nil || strings.Contains(probe.Info.Schema, "/v1") {
		return nil, fmt.Errorf("postman: Collection v1 is not supported; re-export as v2.1 (%s)",
			"https://schema.postman.com/json/collection/v2.1.0/collection.json")
	}
	var col collection
	if err := json.Unmarshal(data, &col); err != nil {
		return nil, fmt.Errorf("postman: %w", err)
	}
	return &col, nil
}

// importRequests walks items in --group request mode: every request is its
// own file at prefix/.../name; folders become directories and never a file
// of their own, so a folder's own scripts are queued (w.pending) onto the
// first request built after them instead.
func (w *walker) importRequests(items []item, prefix []string, parent authState) {
	for _, it := range items {
		w.mergeVariables(it.Variable)
		path := appended(prefix, it.Name)
		name := joinPath(path)
		if it.isFolder() {
			folderAuth := parent.resolve(it.Auth)
			w.warnEvents(name, it.Event)
			w.pending = append(w.pending, scriptComments(it.Event)...)
			if len(it.Item) == 0 {
				w.skip(name, "empty folder")
				continue
			}
			w.importRequests(it.Item, path, folderAuth)
			continue
		}
		if it.Disabled {
			w.skip(name, "disabled")
			continue
		}
		reqAuth := parent.resolve(it.Request.Auth)
		e := w.buildEntry(it, reqAuth, name)
		if len(w.pending) > 0 {
			e.Comments = append(append([]string(nil), w.pending...), e.Comments...)
		}
		f, err := syntax.BuildFile([]syntax.EntrySpec{e}, w.dialect)
		if err != nil {
			w.skip(name, err.Error())
			continue
		}
		w.pending = nil // attached to the file just written; don't repeat it
		w.out.Files = append(w.out.Files, convert.GeneratedFile{Path: name, File: f})
	}
}

// importFolder walks items in --group folder mode: fileName's file chains
// every request directly inside items (not those of a nested folder, which
// gets its own file); leading is prepended, as comments, to that file's
// first entry. Every entry is validated on its own (a single-entry
// BuildFile) before joining the chain, so one bad request is skipped
// rather than sinking every other request of the folder.
func (w *walker) importFolder(items []item, prefix []string, parent authState, fileName string, leading []string) {
	var entries []syntax.EntrySpec
	for _, it := range items {
		w.mergeVariables(it.Variable)
		path := appended(prefix, it.Name)
		name := joinPath(path)
		if it.isFolder() {
			folderAuth := parent.resolve(it.Auth)
			w.warnEvents(name, it.Event)
			if len(it.Item) == 0 {
				w.skip(name, "empty folder")
				continue
			}
			w.importFolder(it.Item, path, folderAuth, name, scriptComments(it.Event))
			continue
		}
		if it.Disabled {
			w.skip(name, "disabled")
			continue
		}
		reqAuth := parent.resolve(it.Request.Auth)
		e := w.buildEntry(it, reqAuth, name)
		if _, err := syntax.BuildFile([]syntax.EntrySpec{e}, w.dialect); err != nil {
			w.skip(name, err.Error())
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return
	}
	if len(leading) > 0 {
		entries[0].Comments = append(append([]string(nil), leading...), entries[0].Comments...)
	}
	f, err := syntax.BuildFile(entries, w.dialect)
	if err != nil {
		// Every entry above already validated on its own; this should not
		// happen, but never silently drop the whole folder over it.
		w.skip(fileName, err.Error())
		return
	}
	w.out.Files = append(w.out.Files, convert.GeneratedFile{Path: fileName, File: f})
}

// buildEntry renders one request item to an EntrySpec: comments (name,
// description, kept scripts), URL, headers, auth, body and, when a "test"
// script has a recognized status assertion, the expected response.
func (w *walker) buildEntry(it item, auth authState, name string) syntax.EntrySpec {
	req := it.Request
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "GET"
	}
	e := syntax.EntrySpec{Method: method}
	if it.Name != "" {
		e.Comments = append(e.Comments, it.Name)
	}
	if d := firstLine(string(it.Description)); d != "" {
		e.Comments = append(e.Comments, d)
	}
	if d := firstLine(string(req.Description)); d != "" && d != firstLine(string(it.Description)) {
		e.Comments = append(e.Comments, d)
	}

	e.URL = w.urlText(req.URL)
	for _, h := range req.Header {
		if h.Disabled {
			continue
		}
		kt, kw := convert.ParseText(h.Key)
		vt, vw := convert.ParseText(string(h.Value))
		w.addWarnings(kw)
		w.addWarnings(vw)
		e.Headers = append(e.Headers, syntax.Field{Key: kt, Value: vt})
	}
	w.applyAuth(&e, auth, name)
	w.applyBody(&e, req.Body, hasContentTypeHeader(req.Header), name)
	w.applyEvents(&e, it.Event, name)
	return e
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// urlText renders a Postman url field, a string or an object with a "raw"
// (or, failing that, its parts), as a syntax.Text with {{variable}}
// placeholders parsed out.
func (w *walker) urlText(raw json.RawMessage) syntax.Text {
	if len(raw) == 0 {
		return syntax.PlainText("")
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		t, ws := convert.ParseText(w.pathVariables(s, nil))
		w.addWarnings(ws)
		return t
	}
	var obj urlObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		w.warn(convert.WarnUnsupported, "a request URL could not be read; it is left empty")
		return syntax.PlainText("")
	}
	s = obj.Raw
	if s == "" {
		s = reconstructURL(obj)
	}
	t, ws := convert.ParseText(w.pathVariables(s, obj.Variable))
	w.addWarnings(ws)
	return t
}

type urlObject struct {
	Raw      string          `json:"raw"`
	Protocol string          `json:"protocol"`
	Host     json.RawMessage `json:"host"`
	Path     json.RawMessage `json:"path"`
	Port     string          `json:"port"`
	Query    []queryParam    `json:"query"`
	Variable []variable      `json:"variable"`
}

// pathVariables rewrites every path segment of u written as a Postman path
// variable (":id", the whole segment) with the value vars gives it, since
// Sonde sends a ":id" segment as is:
//   - a value holding a {{template}} replaces the segment;
//   - a literal value becomes {{id}}, with the value added to every
//     environment (w.pathVars), unless another variable already uses that
//     name or another request gave id a different value: the literal then
//     replaces the segment, so this request still sends what Postman did;
//   - no value becomes {{id}}, with a warning to set it.
//
// The query string and fragment are left alone.
func (w *walker) pathVariables(u string, vars []variable) string {
	values := map[string]string{}
	for _, v := range vars {
		if _, ok := values[v.Key]; !ok {
			values[v.Key] = string(v.Value)
		}
	}
	path, rest := u, ""
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		path, rest = u[:i], u[i:]
	}
	segs := strings.Split(path, "/")
	for i, seg := range segs {
		key, ok := strings.CutPrefix(seg, ":")
		if !ok || !isPathVariableKey(key) {
			continue
		}
		segs[i] = w.pathVariable(key, values[key])
	}
	return strings.Join(segs, "/") + rest
}

// pathVariable returns the text that replaces the ":key" segment whose
// Postman value is value (see pathVariables).
func (w *walker) pathVariable(key, value string) string {
	name := convert.VariableName(key)
	placeholder := "{{" + name + "}}"
	switch {
	case strings.Contains(value, "{{"):
		return value
	case value == "":
		w.warn(convert.WarnUnsupported, fmt.Sprintf("path variable :%s has no value; set %s before running", key, name))
		return placeholder
	}
	if prev, ok := w.pathVars[name]; w.taken[name] || ok && prev != value {
		return value
	}
	w.pathVars[name] = value
	return placeholder
}

// isPathVariableKey reports whether key (after the ':') names a Postman
// path variable: a letter or '_' first, then letters, digits, '_' or '-'.
// It keeps a segment such as ":" or ":8080" as it is.
func isPathVariableKey(key string) bool {
	for i, r := range key {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
		digitOrDash := r >= '0' && r <= '9' || r == '-'
		if !letter && (i == 0 || !digitOrDash) {
			return false
		}
	}
	return key != ""
}

type queryParam struct {
	Key      flexString `json:"key"`
	Value    flexString `json:"value"`
	Disabled bool       `json:"disabled"`
}

func reconstructURL(o urlObject) string {
	var b strings.Builder
	if o.Protocol != "" {
		b.WriteString(o.Protocol)
		b.WriteString("://")
	}
	b.WriteString(strings.Join(stringOrArray(o.Host), "."))
	if o.Port != "" {
		b.WriteString(":")
		b.WriteString(o.Port)
	}
	if segs := stringOrArray(o.Path); len(segs) > 0 {
		b.WriteString("/")
		b.WriteString(strings.Join(segs, "/"))
	}
	var q []string
	for _, p := range o.Query {
		if !p.Disabled {
			q = append(q, string(p.Key)+"="+string(p.Value))
		}
	}
	if len(q) > 0 {
		b.WriteString("?")
		b.WriteString(strings.Join(q, "&"))
	}
	return b.String()
}

// stringOrArray decodes a field written as a single string or an array of
// strings (Postman's url.host and url.path), never failing on either shape.
func stringOrArray(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []string{s}
	}
	var arr []flexString
	if json.Unmarshal(raw, &arr) == nil {
		out := make([]string, len(arr))
		for i, v := range arr {
			out[i] = string(v)
		}
		return out
	}
	return nil
}

// finish builds the sonde.yaml skeleton and secrets stubs from the
// collection's variables (the "collection" environment) and every
// --environment file (its own environment, its values overriding the
// collection's), and appends them to w.out.
func (w *walker) finish(envFiles []EnvironmentFile) error {
	used := map[string]bool{"collection": true} // sonde.yaml environment names
	stubUsed := map[string]bool{}               // secrets stub file paths (a separate namespace)
	envs := map[string]config.EnvironmentSkeleton{}

	// Path variable names never clash with a collection, folder or
	// environment variable (w.taken), so they join every environment as is.
	for name, value := range w.pathVars {
		w.collVars[name] = value
	}
	collSecretNames := sortedSet(w.collSecrets)
	sk, extra := buildEnvironment("collection", w.collVars, collSecretNames, stubUsed)
	envs["collection"] = sk
	if extra != nil {
		w.out.Extra = append(w.out.Extra, *extra)
	}
	defaultEnv := "collection"

	for i, ef := range envFiles {
		rawName, vars, secretNames, err := w.parseEnvironment(ef)
		if err != nil {
			return err
		}
		merged := make(map[string]string, len(w.collVars)+len(vars))
		for k, v := range w.collVars {
			merged[k] = v
		}
		for k, v := range vars {
			merged[k] = v
		}
		secrets := map[string]bool{}
		for _, n := range collSecretNames {
			secrets[n] = true
		}
		for _, n := range secretNames {
			secrets[n] = true
		}
		name := uniqueEnvName(rawName, used)
		sk, extra := buildEnvironment(name, merged, sortedSet(secrets), stubUsed)
		envs[name] = sk
		if extra != nil {
			w.out.Extra = append(w.out.Extra, *extra)
		}
		if i == 0 {
			defaultEnv = name
		}
	}

	proj, err := config.EmitProject(config.ProjectSkeleton{Environments: envs, DefaultEnv: defaultEnv})
	if err != nil {
		return err
	}
	w.out.ProjectYAML = proj
	return nil
}

// buildEnvironment renders one sonde.yaml environment: vars as its
// "variables", and, if secretNames is non-empty, a secrets stub (names
// only, no values) at convert.StubPath(name, stubUsed), referenced from
// its "secrets_files" — the same helper convert.Write's own sanitizing
// uses, so the two can never disagree on the path (H2).
func buildEnvironment(name string, vars map[string]string, secretNames []string, stubUsed map[string]bool) (config.EnvironmentSkeleton, *convert.RawFile) {
	sk := config.EnvironmentSkeleton{Variables: vars}
	if len(secretNames) == 0 {
		return sk, nil
	}
	stub := make(map[string]string, len(secretNames))
	for _, n := range secretNames {
		stub[n] = ""
	}
	path := convert.StubPath(name, stubUsed)
	sk.SecretsFiles = []string{path}
	return sk, &convert.RawFile{Path: path, Data: config.EmitVariables(stub), Keep: true}
}
