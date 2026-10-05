// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// counts sums up the planned import for its result tile.
func (p *plan) counts() Counts {
	c := Counts{Files: len(p.out.Files), Requests: p.res.RequestCount}
	for _, f := range p.out.Files {
		for _, e := range f.File.Entries {
			if e.Response != nil {
				c.StatusChecks++
			}
		}
	}
	for _, x := range p.out.Extra {
		if x.Keep {
			c.SecretStubs++
		}
	}
	for _, w := range p.out.Warnings {
		if w.Kind == convert.WarnScript {
			c.Scripts++
		}
	}
	// Environments: those of the sonde.yaml the import writes, or adds to
	// the project's (none it has already: it keeps its own).
	if p.res.Project != "" {
		c.Environments = len(p.environments())
	} else if p.edited != "" {
		c.Environments = len(p.importEnvs) - len(p.envsKept)
		c.SecretNames = p.secretNames
	}
	if p.postOpts.Group != "" {
		c.PathVariables = p.pathVariables()
	}
	return c
}

// environments lists the environments of the converter's sonde.yaml.
func (p *plan) environments() []string {
	var y struct {
		Environments map[string]any `yaml:"environments"`
	}
	if p.out.ProjectYAML == nil || yaml.Unmarshal(p.out.ProjectYAML, &y) != nil {
		return nil
	}
	out := make([]string, 0, len(y.Environments))
	for name := range y.Environments {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// pathVariables counts the path variables of a Postman collection the
// import wrote as {{name}} (a segment can keep its value instead).
func (p *plan) pathVariables() int {
	n := 0
	for _, name := range pathVariables(p.data) {
		for _, f := range p.out.Files {
			if strings.Contains(string(syntax.Format(f.File)), "/{{"+name+"}}") {
				n++
				break
			}
		}
	}
	return n
}

// pathVariables lists the distinct path variables (`:id` segments, by
// variable name) of a Postman collection's requests.
func pathVariables(data []byte) []string {
	var doc any
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	names := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, x := range t {
				walk(x)
			}
		case map[string]any:
			if req, ok := t["request"].(map[string]any); ok {
				for _, seg := range urlPath(req["url"]) {
					if key, ok := strings.CutPrefix(seg, ":"); ok && pathKey(key) {
						names[convert.VariableName(key)] = true
					}
				}
			}
			for _, k := range []string{"item", "requests", "folders"} {
				walk(t[k])
			}
		}
	}
	walk(doc)
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// urlPath is the path segments of a Postman url (a string or an object).
func urlPath(u any) []string {
	switch t := u.(type) {
	case string:
		raw, _, _ := strings.Cut(t, "?")
		if _, rest, ok := strings.Cut(raw, "://"); ok {
			raw = rest
		}
		return strings.Split(raw, "/")
	case map[string]any:
		if segs, ok := t["path"].([]any); ok {
			var out []string
			for _, s := range segs {
				if str, ok := s.(string); ok {
					out = append(out, str)
				}
			}
			return out
		}
		if raw, ok := t["raw"].(string); ok {
			return urlPath(raw)
		}
	}
	return nil
}

// pathKey reports whether key (after the ':') names a path variable: a
// letter or '_' first, then letters, digits, '_' or '-'.
func pathKey(key string) bool {
	for i, r := range key {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
		later := i > 0 && (r >= '0' && r <= '9' || r == '-')
		if !letter && !later {
			return false
		}
	}
	return key != ""
}

// collection names a Postman collection and counts its folders (items
// holding items), for the dialog's title.
func collection(data []byte) (name string, folders int) {
	var doc struct {
		Info struct {
			Name string `json:"name"`
		} `json:"info"`
		Item []any `json:"item"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return "", 0
	}
	var walk func(items []any)
	walk = func(items []any) {
		for _, it := range items {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if sub, ok := m["item"].([]any); ok {
				folders++
				walk(sub)
			}
		}
	}
	walk(doc.Item)
	return strings.TrimSpace(doc.Info.Name), folders
}

// placeholder is a {{variable}}, its name captured.
var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][\w.-]*)\s*\}\}`)

// used lists the variables the request files use (outside comments) and
// none of them captures, by name, with the number of files using each.
func (p *plan) used() []Used {
	files := map[string]int{}
	captured := map[string]bool{}
	for _, f := range p.files {
		if ext := path.Ext(f.Path); ext != ".hurl" && ext != ".sonde" {
			continue
		}
		for _, n := range captureNames(f.Path, string(f.Data)) {
			captured[n] = true
		}
		seen := map[string]bool{}
		for _, line := range strings.Split(string(f.Data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			for _, m := range placeholder.FindAllStringSubmatch(line, -1) {
				if !seen[m[1]] && m[1] != "newDate" && m[1] != "newUuid" {
					seen[m[1]] = true
					files[m[1]]++
				}
			}
		}
	}
	out := []Used{}
	for name, n := range files {
		if !captured[name] {
			out = append(out, Used{Name: name, Files: n})
		}
	}
	slices.SortFunc(out, func(a, b Used) int { return strings.Compare(a.Name, b.Name) })
	return out
}
