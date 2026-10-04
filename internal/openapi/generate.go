// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// Grouping values of GenerateOptions.Group.
const (
	GroupTag  = "tag"
	GroupPath = "path"
	GroupFlat = "flat"
)

// GenerateOptions configure Generate.
type GenerateOptions struct {
	// Group lays files out: in a directory per first tag (GroupTag), per
	// first path segment (GroupPath), or all in one (GroupFlat).
	Group string
	// BaseURLVar is the variable prefixing every URL (default "base_url").
	BaseURLVar string
	Dialect    syntax.Dialect
}

// Generated is the output of Generate.
type Generated struct {
	Files []GeneratedFile
	// Variables are the values of the variables the files use: the base
	// URL (from the first server) and the path parameters (from their
	// examples), for a sonde.yaml environment.
	Variables map[string]string
	// Warnings, by kind, and the operations skipped with their reasons.
	Warnings []Note
	Skipped  []Note
}

// GeneratedFile is the request file of one operation.
type GeneratedFile struct {
	// Path is relative, slash-separated, without extension.
	Path string
	File *syntax.File
}

// Note is a warning (Kind, Message) or a skipped operation (Kind: its
// name, Message: the reason).
type Note struct{ Kind, Message string }

// Generate returns one request file per operation of the spec: path
// parameters as variables, required query parameters and headers with
// example values, security as variable placeholders, a body from the
// examples or the schema, and the lowest documented 2xx status as the
// expected response.
func (s *Spec) Generate(opt GenerateOptions) (gen *Generated, err error) {
	defer func() {
		if p := recover(); p != nil {
			gen, err = nil, fmt.Errorf("malformed spec: %v", p)
		}
	}()
	switch opt.Group {
	case "":
		opt.Group = GroupTag
	case GroupTag, GroupPath, GroupFlat:
	default:
		return nil, fmt.Errorf("invalid --group %q: expected tag, path or flat", opt.Group)
	}
	if opt.BaseURLVar == "" {
		opt.BaseURLVar = "base_url"
	}
	if !validVarName(opt.BaseURLVar) {
		return nil, fmt.Errorf("invalid variable name %q", opt.BaseURLVar)
	}
	g := &generator{spec: s, opt: opt, out: &Generated{Variables: map[string]string{}}, warned: map[string]bool{}}
	g.base = g.baseURL()
	g.out.Variables[opt.BaseURLVar] = g.base
	for _, t := range s.templates {
		item := s.doc.Paths.Value(t)
		ops := item.Operations()
		methods := make([]string, 0, len(ops))
		for m := range ops {
			methods = append(methods, m)
		}
		sort.Strings(methods)
		for _, m := range methods {
			g.operation(t, m, item, ops[m])
		}
	}
	return g.out, nil
}

type generator struct {
	spec   *Spec
	opt    GenerateOptions
	out    *Generated
	warned map[string]bool
	base   string // the base URL variable's value
}

func (g *generator) warn(kind, msg string) {
	if !g.warned[kind+msg] {
		g.warned[kind+msg] = true
		g.out.Warnings = append(g.out.Warnings, Note{kind, msg})
	}
}

// baseURL is the first server's URL, made absolute: the spec's, else the
// one most operations declare (theirs or their path's; on a tie, the
// lowest).
func (g *generator) baseURL() string {
	first := ""
	if len(g.spec.servers) > 0 {
		first = g.spec.servers[0]
	} else {
		uses := map[string]int{}
		for _, t := range g.spec.templates {
			item := g.spec.doc.Paths.Value(t)
			for _, op := range item.Operations() {
				if u := ownServer(item, op); u != "" {
					uses[u]++
				}
			}
		}
		for u, n := range uses {
			if first == "" || n > uses[first] || n == uses[first] && u < first {
				first = u
			}
		}
	}
	if first == "" {
		g.warn("server", "the spec has no servers: base_url is http://localhost")
		return "http://localhost"
	}
	return g.absolute(first)
}

// absolute makes a server URL absolute, on http://localhost.
func (g *generator) absolute(server string) string {
	u := strings.TrimRight(server, "/")
	if !strings.Contains(u, "://") {
		g.warn("server", fmt.Sprintf("the server %q is relative: it is http://localhost%s", server, u))
		u = "http://localhost" + u
	}
	return u
}

// ownServer is the first server an operation (op, when not nil) or else
// its path item declares, overriding the spec's; "" when neither does.
func ownServer(item *openapi3.PathItem, op *openapi3.Operation) string {
	if op != nil && op.Servers != nil {
		for _, srv := range *op.Servers {
			if srv != nil {
				return serverURL(srv)
			}
		}
	}
	for _, srv := range item.Servers {
		if srv != nil {
			return serverURL(srv)
		}
	}
	return ""
}

func (g *generator) operation(template, method string, item *openapi3.PathItem, op *openapi3.Operation) {
	name := method + " " + template
	params := mergeParams(item.Parameters, op.Parameters)
	e := syntax.EntrySpec{Method: method}
	if id := strings.Join(strings.Fields(op.OperationID), " "); id != "" {
		e.Comments = append(e.Comments, id)
	}
	if op.Summary != "" {
		e.Comments = append(e.Comments, strings.Join(strings.Fields(op.Summary), " "))
	}
	if op.Deprecated {
		e.Comments = append(e.Comments, "deprecated")
	}
	e.URL = g.url(template, params)
	if strings.Contains(template, "//") {
		// Most often a variable exported while it was empty.
		g.warn("path", fmt.Sprintf("%s: the path has an empty segment", name))
	}
	if own := ownServer(item, op); own != "" {
		// An operation on a server of its own sends there, not to base_url.
		if u := g.absolute(own); u != g.base {
			e.URL[0] = syntax.Lit(u)
		}
	}
	for _, p := range params {
		if p == nil || p.Value == nil || !p.Value.Required {
			continue
		}
		switch p.Value.In {
		case openapi3.ParameterInQuery:
			e.Query = append(e.Query, syntax.Field{Key: syntax.PlainText(p.Value.Name), Value: g.text(paramExample(p.Value))})
		case openapi3.ParameterInHeader:
			e.Headers = append(e.Headers, syntax.Field{Key: syntax.PlainText(p.Value.Name), Value: g.text(paramExample(p.Value))})
		case openapi3.ParameterInCookie:
			e.Cookies = append(e.Cookies, syntax.Field{Key: syntax.PlainText(p.Value.Name), Value: g.text(paramExample(p.Value))})
		}
	}
	g.security(&e, op)
	if err := g.body(&e, name, op); err != nil {
		g.out.Skipped = append(g.out.Skipped, Note{name, err.Error()})
		return
	}
	if status := successStatus(op); status != "" {
		e.Response = &syntax.ResponseSpec{Status: status}
	}
	f, err := syntax.BuildFile([]syntax.EntrySpec{e}, g.opt.Dialect)
	if err != nil {
		g.out.Skipped = append(g.out.Skipped, Note{name, err.Error()})
		return
	}
	g.out.Files = append(g.out.Files, GeneratedFile{Path: g.filePath(template, method, op), File: f})
}

// mergeParams returns the path item's parameters overridden by the
// operation's (same location and name), then the operation's others.
func mergeParams(item, op openapi3.Parameters) openapi3.Parameters {
	var out openapi3.Parameters
	for _, p := range item {
		if p == nil || p.Value == nil || op.GetByInAndName(p.Value.In, p.Value.Name) == nil {
			out = append(out, p)
		}
	}
	return append(out, op...)
}

// url is {{base_url}} followed by the template, its parameters as
// variables whose values come from the parameters' examples.
func (g *generator) url(template string, params openapi3.Parameters) syntax.Text {
	t := syntax.Text{syntax.Var(g.opt.BaseURLVar)}
	last := 0
	for _, m := range paramRe.FindAllStringSubmatchIndex(template, -1) {
		t = append(t, syntax.Lit(template[last:m[0]]))
		pname := template[m[2]:m[3]]
		v := varName(pname)
		t = append(t, syntax.Var(v))
		if _, ok := g.out.Variables[v]; !ok {
			val := "1"
			if p := params.GetByInAndName(openapi3.ParameterInPath, pname); p != nil {
				val = scalarText(paramExample(p))
			}
			g.out.Variables[v] = url.PathEscape(val)
		}
		last = m[1]
	}
	return append(t, syntax.Lit(template[last:]))
}

// security adds the credentials of the operation's first security
// requirement as variable placeholders.
func (g *generator) security(e *syntax.EntrySpec, op *openapi3.Operation) {
	reqs := g.spec.doc.Security
	if op.Security != nil {
		reqs = *op.Security
	}
	if len(reqs) == 0 || g.spec.doc.Components == nil {
		return
	}
	names := make([]string, 0, len(reqs[0]))
	for n := range reqs[0] {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ref := g.spec.doc.Components.SecuritySchemes[n]
		if ref == nil || ref.Value == nil {
			continue
		}
		s := ref.Value
		switch {
		case s.Type == "http" && strings.EqualFold(s.Scheme, "basic"):
			e.BasicAuth = &syntax.BasicAuth{User: syntax.Text{syntax.Var("username")}, Password: syntax.Text{syntax.Var("password")}}
			g.warn("auth", "set the username and password variables (secrets: --secret password=...)")
		case s.Type == "http" && strings.EqualFold(s.Scheme, "bearer"), s.Type == "oauth2", s.Type == "openIdConnect":
			e.Headers = append(e.Headers, syntax.Field{Key: syntax.PlainText("Authorization"), Value: syntax.Text{syntax.Lit("Bearer "), syntax.Var("token")}})
			g.warn("auth", "set the token variable (a secret: --secret token=...)")
		case s.Type == "apiKey":
			v := varName(s.Name)
			f := syntax.Field{Key: syntax.PlainText(s.Name), Value: syntax.Text{syntax.Var(v)}}
			switch s.In {
			case "header":
				e.Headers = append(e.Headers, f)
			case "query":
				e.Query = append(e.Query, f)
			case "cookie":
				e.Cookies = append(e.Cookies, f)
			}
			g.warn("auth", fmt.Sprintf("set the %s variable (a secret: --secret %s=...)", v, v))
		default:
			g.warn("auth", fmt.Sprintf("security scheme %q (%s) is not supported: add its credentials by hand", n, s.Type))
		}
	}
}

// body sets the request body from the first supported media type.
func (g *generator) body(e *syntax.EntrySpec, name string, op *openapi3.Operation) error {
	if op.RequestBody == nil || op.RequestBody.Value == nil || len(op.RequestBody.Value.Content) == 0 {
		return nil
	}
	content := op.RequestBody.Value.Content
	types := make([]string, 0, len(content))
	for mt := range content {
		types = append(types, mt)
	}
	sort.Slice(types, func(i, j int) bool {
		return mediaRank(types[i]) < mediaRank(types[j]) || (mediaRank(types[i]) == mediaRank(types[j]) && types[i] < types[j])
	})
	mt := types[0]
	media := content[mt]
	v := mediaExample(media)
	switch mediaRank(mt) {
	case 0, 1:
		if mt != "application/json" {
			e.Headers = append(e.Headers, syntax.KV("Content-Type", mt))
		}
		b, err := syntax.JSONBody(jsonValue(v, g.note))
		if err != nil {
			return err
		}
		e.Body = b
	case 2:
		for _, k := range sortedKeys(v) {
			e.Form = append(e.Form, syntax.Field{Key: syntax.PlainText(k), Value: g.text(v.(map[string]any)[k])})
		}
	case 3:
		obj, _ := v.(map[string]any)
		for _, k := range sortedKeys(v) {
			f := syntax.MultipartField{Key: syntax.PlainText(k)}
			if isBinary(media, k) {
				f.File = &syntax.MultipartFile{Name: syntax.PlainText(k + ".bin")}
				g.warn("body", fmt.Sprintf("%s: multipart field %q reads the file %s.bin", name, k, k))
			} else {
				f.Value = g.text(obj[k])
			}
			e.Multipart = append(e.Multipart, f)
		}
	case 4:
		e.Headers = append(e.Headers, syntax.KV("Content-Type", mt))
		if text := scalarText(v); strings.Contains(text, "{{") {
			e.Body = syntax.TextBody(g.text(v), "")
		} else {
			e.Body = syntax.RawTextBody(text, "")
		}
	default:
		g.warn("body", fmt.Sprintf("%s: request body %s is not generated", name, mt))
	}
	return nil
}

// mediaRank orders request media types by preference: JSON, JSON-like,
// form, multipart, text, other.
func mediaRank(mt string) int {
	switch {
	case mt == "application/json":
		return 0
	case strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "/json"):
		return 1
	case mt == "application/x-www-form-urlencoded":
		return 2
	case mt == "multipart/form-data":
		return 3
	case strings.HasPrefix(mt, "text/") || strings.HasSuffix(mt, "+xml") || strings.HasSuffix(mt, "/xml"):
		return 4
	}
	return 5
}

func mediaExample(m *openapi3.MediaType) any {
	if m == nil {
		return nil
	}
	if m.Example != nil {
		return m.Example
	}
	if len(m.Examples) > 0 {
		keys := make([]string, 0, len(m.Examples))
		for k := range m.Examples {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if ex := m.Examples[keys[0]]; ex != nil && ex.Value != nil && ex.Value.Value != nil {
			return ex.Value.Value
		}
	}
	return Example(m.Schema)
}

func isBinary(m *openapi3.MediaType, prop string) bool {
	if m.Schema == nil || m.Schema.Value == nil {
		return false
	}
	p := m.Schema.Value.Properties[prop]
	return p != nil && p.Value != nil && (p.Value.Format == "binary" || p.Value.ContentMediaType != "")
}

func sortedKeys(v any) []string {
	obj, _ := v.(map[string]any)
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// successStatus is the lowest documented 2xx status ("200" for "2XX").
func successStatus(op *openapi3.Operation) string {
	if op.Responses == nil {
		return ""
	}
	best := ""
	for code := range op.Responses.Map() {
		c := code
		if strings.EqualFold(c, "2XX") {
			c = "200"
		}
		if len(c) == 3 && c[0] == '2' && (best == "" || c < best) {
			if _, err := strconv.Atoi(c); err == nil {
				best = c
			}
		}
	}
	return best
}

func (g *generator) filePath(template, method string, op *openapi3.Operation) string {
	name := kebab(strings.ReplaceAll(op.OperationID, "/", "-"))
	// A generated operationId (a path, a hash) loses to a shorter summary.
	if summary := strings.Join(strings.Fields(strings.ReplaceAll(op.Summary, "/", " ")), " "); summary != "" && (name == "" || len(summary) < len(name)) {
		name = summary
	}
	if name == "" {
		name = strings.ToLower(method) + "-" + strings.NewReplacer("{", "", "}", "", "/", "-").Replace(strings.Trim(template, "/"))
	}
	dir := ""
	switch g.opt.Group {
	case GroupTag:
		if len(op.Tags) > 0 {
			dir = op.Tags[0]
		}
	case GroupPath:
		if segs := splitPath(template); len(segs) > 0 && !strings.Contains(segs[0], "{") {
			dir = segs[0]
		}
	}
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// kebab converts camelCase to kebab-case ("listPets" -> "list-pets").
func kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := s[i-1]
			if prev >= 'a' && prev <= 'z' || prev >= '0' && prev <= '9' {
				b.WriteByte('-')
			}
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func paramExample(p *openapi3.Parameter) any {
	if p.Example != nil {
		return p.Example
	}
	if len(p.Examples) > 0 {
		keys := make([]string, 0, len(p.Examples))
		for k := range p.Examples {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if ex := p.Examples[keys[0]]; ex != nil && ex.Value != nil && ex.Value.Value != nil {
			return ex.Value.Value
		}
	}
	if v := Example(p.Schema); v != nil {
		return v
	}
	return "value"
}

// scalarText formats an example value as text: JSON for composite values.
func scalarText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// text is an example value as request text: a {{name}} in it stays a
// variable, as collection exporters write them into examples.
func (g *generator) text(v any) syntax.Text {
	t, warns := convert.ParseText(scalarText(v))
	g.note(warns)
	return t
}

// note records the warnings of convert.ParseText.
func (g *generator) note(warns []convert.Warning) {
	for _, w := range warns {
		g.warn(w.Kind, w.Message)
	}
}

// jsonValue converts an example to the types syntax.JSONBody accepts; a
// string with a {{name}} keeps it a variable (warnings go to note).
func jsonValue(v any, note func([]convert.Warning)) any {
	switch x := v.(type) {
	case int64:
		return json.Number(strconv.FormatInt(x, 10))
	case int:
		return json.Number(strconv.Itoa(x))
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return json.Number(strconv.FormatInt(int64(x), 10))
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = jsonValue(e, note)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = jsonValue(e, note)
		}
		return out
	case string:
		if !strings.Contains(x, "{{") {
			return x
		}
		t, warns := convert.ParseText(x)
		if note != nil {
			note(warns)
		}
		return t
	case nil, bool, json.Number:
		return x
	}
	// Other decoded shapes (such as typed slices) go through JSON.
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return jsonValue(out, note)
}

// varName makes a variable name of s: letters, digits, "_" and "-",
// not starting with a digit or "-".
func varName(s string) string {
	var b strings.Builder
	for i, r := range s {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && (r == '-' || (r >= '0' && r <= '9')))
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

func validVarName(s string) bool { return s != "" && varName(s) == s }
