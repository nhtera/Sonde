// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// Lifting: a credential written in a pasted command (an Authorization
// header, a password, an API key) becomes a {{name}} in the request, its
// value going to the environment's secrets file, never to the project's
// tracked files. Each one is offered; the user picks which to lift.

// Candidate is a value that can be lifted (the value itself never leaves
// Go).
type Candidate struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`  // the variable it becomes
	Where string `json:"where"` // "Authorization header", "query api_key"…
}

// candidate is a Candidate with its value and where it is in the source.
type candidate struct {
	Candidate
	value      string
	start, end int // byte offsets of the value in the source
}

// keyName is a header or query key that holds a credential.
var keyName = regexp.MustCompile(`(?i)api[-_]?key|apikey|token|secret|access[-_]?key|x-auth`)

// candidates lists the values of src (formatted request text) that look
// like credentials, in source order (names avoid taken ones and each
// other), and where one is written with escapes, so not offered.
func candidates(src []byte, d syntax.Dialect, taken map[string]bool) ([]candidate, []string, error) {
	f, err := syntax.Parse("lift", src, d)
	if err != nil {
		return nil, nil, err
	}
	var out []candidate
	var escaped []string
	add := func(name, where string, t *syntax.Template, skip int) {
		value, ok := literal(t)
		if !ok {
			if t != nil && !hasPlaceholder(t) {
				escaped = append(escaped, where)
			}
			return
		}
		if len(value) <= skip || strings.Contains(value[skip:], "{{") {
			return
		}
		out = append(out, candidate{
			Candidate: Candidate{Name: name, Where: where},
			value:     value[skip:],
			start:     t.Span.Start.Offset + skip,
			end:       t.Span.End.Offset,
		})
	}
	for _, e := range f.Entries {
		r := e.Request
		// Query parameters written in the URL.
		if u, ok := literal(r.URL); ok {
			if q := strings.IndexByte(u, '?'); q >= 0 {
				off := r.URL.Span.Start.Offset + q + 1
				query, _, _ := strings.Cut(u[q+1:], "#")
				for _, part := range strings.Split(query, "&") {
					if key, value, found := strings.Cut(part, "="); found && value != "" && keyName.MatchString(key) && !strings.Contains(value, "{{") {
						start := off + len(key) + 1
						out = append(out, candidate{Candidate: Candidate{Name: convert.VariableName(strings.ToLower(key)), Where: "query " + key}, value: value, start: start, end: start + len(value)})
					}
					off += len(part) + 1
				}
			}
		}
		for _, h := range r.Headers {
			key, ok := literal(h.Key)
			if !ok {
				continue
			}
			switch value, _ := literal(h.Value); {
			case strings.EqualFold(key, "Authorization"):
				// "Bearer <token>": the token; a value without a scheme: all of it.
				if scheme, rest, found := strings.Cut(value, " "); found && rest != "" && !strings.ContainsAny(scheme, "=:") {
					name := "token"
					if strings.EqualFold(scheme, "Basic") {
						name = "basic_auth"
					}
					add(name, "Authorization header", h.Value, len(scheme)+1)
				} else {
					add("authorization", "Authorization header", h.Value, 0)
				}
			case keyName.MatchString(key):
				add(convert.VariableName(strings.ToLower(key)), key+" header", h.Value, 0)
			}
		}
		for _, sec := range r.Sections {
			switch sec.Kind {
			case syntax.SectionQueryParams:
				for _, kv := range sec.KeyValues {
					if key, ok := literal(kv.Key); ok && keyName.MatchString(key) {
						add(convert.VariableName(strings.ToLower(key)), "query "+key, kv.Value, 0)
					}
				}
			case syntax.SectionBasicAuth:
				for _, kv := range sec.KeyValues {
					add("password", "password of "+text(kv.Key), kv.Value, 0)
				}
			case syntax.SectionOptions:
				for _, o := range sec.Options {
					if t, ok := o.Value.(*syntax.Template); ok && o.Name == "user" {
						// user:password — the password.
						if v, ok := literal(t); ok {
							if i := strings.IndexByte(v, ':'); i >= 0 {
								add("password", "password of "+v[:i], t, i+1)
							}
						}
					}
				}
			}
		}
	}
	used := map[string]bool{}
	for i := range out {
		out[i].ID = i
		out[i].Name = unique(out[i].Name, func(n string) bool { return taken[n] || used[n] })
		used[out[i].Name] = true
	}
	return out, escaped, nil
}

// hasPlaceholder reports whether t holds a {{placeholder}}.
func hasPlaceholder(t *syntax.Template) bool {
	for _, el := range t.Elements {
		if _, ok := el.(*syntax.Placeholder); ok {
			return true
		}
	}
	return false
}

// literal returns t's text when it is all literal and written as is (no
// escapes, no placeholders): its source span is then its value.
func literal(t *syntax.Template) (string, bool) {
	if t == nil {
		return "", false
	}
	var b strings.Builder
	for _, el := range t.Elements {
		s, ok := el.(*syntax.TemplateString)
		if !ok || s.Source != s.Value {
			return "", false
		}
		b.WriteString(s.Value)
	}
	return b.String(), t.Delimiter == 0
}

func text(t *syntax.Template) string {
	s, _ := literal(t)
	return s
}

// unique returns name, or name_2, name_3… the first that is not taken.
func unique(name string, taken func(string) bool) string {
	out := name
	for i := 2; taken(out); i++ {
		out = name + "_" + strconv.Itoa(i)
	}
	return out
}

// lift replaces the candidates picked (by ID) with their {{name}} in src
// and returns the new source with the values lifted, by name. The result
// must parse with the same entries.
func lift(src []byte, d syntax.Dialect, cands []candidate, picked []int) ([]byte, map[string]string, error) {
	var chosen []candidate
	for _, c := range cands {
		for _, id := range picked {
			if c.ID == id {
				chosen = append(chosen, c)
			}
		}
	}
	if len(chosen) == 0 {
		return src, nil, nil
	}
	sort.Slice(chosen, func(i, j int) bool { return chosen[i].start > chosen[j].start })
	out := append([]byte(nil), src...)
	values := map[string]string{}
	for _, c := range chosen {
		out = append(out[:c.start], append([]byte("{{"+c.Name+"}}"), out[c.end:]...)...)
		values[c.Name] = c.value
	}
	before, err := syntax.Parse("lift", src, d)
	if err != nil {
		return nil, nil, err
	}
	after, err := syntax.Parse("lift", out, d)
	if err != nil || len(after.Entries) != len(before.Entries) {
		return nil, nil, fmt.Errorf("lifting the secrets changed the requests: %v", err)
	}
	return out, values, nil
}
