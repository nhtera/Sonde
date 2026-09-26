// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Router matches request URLs to the path templates of a spec: the
// request path loses the longest matching server base path, then goes to
// the template matching it best (literal segments win over templated
// ones, from left to right, as OpenAPI requires).
type Router struct {
	bases []string // server base paths, without trailing slash, longest first
	paths []*route
}

// Route is a matched path template.
type Route struct {
	// Template is the path as written in the spec ("/pets/{petId}").
	Template string
	// Params are the decoded values of the template's parameters.
	Params map[string]string
}

type route struct {
	template string
	segs     []segment
}

// segment is a path segment of a template: a literal, or a pattern with
// parameters.
type segment struct {
	literal string
	re      *regexp.Regexp // nil for a literal
	names   []string
	// whole is set when the segment is a single parameter ("{id}").
	whole bool
}

var paramRe = regexp.MustCompile(`\{([^{}/]+)\}`)

// NewRouter returns a router over templates, stripping the base path of
// each server URL (server variables replaced by their defaults) from
// request paths. Without servers the base path is "/".
func NewRouter(templates, servers []string) *Router {
	r := &Router{}
	seen := map[string]bool{}
	for _, s := range servers {
		b := basePath(s)
		if !seen[b] {
			seen[b] = true
			r.bases = append(r.bases, b)
		}
	}
	if len(r.bases) == 0 {
		r.bases = []string{""}
	}
	sort.SliceStable(r.bases, func(i, j int) bool { return len(r.bases[i]) > len(r.bases[j]) })
	for _, t := range templates {
		r.paths = append(r.paths, compileTemplate(t))
	}
	return r
}

// basePath returns the path of a server URL, without trailing slash: ""
// for a server at the root.
func basePath(server string) string {
	p := server
	if u, err := url.Parse(server); err == nil && (u.Scheme != "" || u.Host != "") {
		p = u.EscapedPath()
	} else if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimRight(p, "/")
	if p != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func compileTemplate(t string) *route {
	rt := &route{template: t}
	for _, s := range splitPath(t) {
		locs := paramRe.FindAllStringSubmatchIndex(s, -1)
		if len(locs) == 0 {
			rt.segs = append(rt.segs, segment{literal: s})
			continue
		}
		var b strings.Builder
		b.WriteString("^")
		seg := segment{whole: len(locs) == 1 && locs[0][0] == 0 && locs[0][1] == len(s)}
		last := 0
		for _, l := range locs {
			b.WriteString(regexp.QuoteMeta(s[last:l[0]]))
			b.WriteString("(.+?)")
			seg.names = append(seg.names, s[l[2]:l[3]])
			last = l[1]
		}
		b.WriteString(regexp.QuoteMeta(s[last:]))
		b.WriteString("$")
		seg.re = regexp.MustCompile(b.String())
		rt.segs = append(rt.segs, seg)
	}
	return rt
}

// splitPath splits a path into its segments, ignoring a trailing slash.
func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// Match returns the route of rawURL's path among the templates accept
// takes (nil: all), and ok false when no server base path prefixes it or
// no such template matches.
func (r *Router) Match(rawURL string, accept func(template string) bool) (Route, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Route{}, false
	}
	p := u.EscapedPath()
	for _, b := range r.bases {
		rest, ok := strings.CutPrefix(p, b)
		if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
			continue
		}
		if rt, ok := r.match(splitPath(rest), accept); ok {
			return rt, true
		}
	}
	return Route{}, false
}

// match picks the best template matching segs: at the first segment
// where two candidates differ, a literal beats a partial template, which
// beats a whole-segment parameter.
func (r *Router) match(segs []string, accept func(string) bool) (Route, bool) {
	var best *route
	var bestRank []int
	var bestParams map[string]string
	for _, rt := range r.paths {
		if accept != nil && !accept(rt.template) {
			continue
		}
		params, rank, ok := rt.match(segs)
		if !ok {
			continue
		}
		if best == nil || better(rank, bestRank) || (equalRank(rank, bestRank) && rt.template < best.template) {
			best, bestRank, bestParams = rt, rank, params
		}
	}
	if best == nil {
		return Route{}, false
	}
	return Route{Template: best.template, Params: bestParams}, true
}

func (rt *route) match(segs []string) (map[string]string, []int, bool) {
	if len(segs) != len(rt.segs) {
		return nil, nil, false
	}
	var params map[string]string
	rank := make([]int, len(segs))
	for i, s := range rt.segs {
		if s.re == nil {
			if segs[i] != s.literal {
				return nil, nil, false
			}
			rank[i] = 2
			continue
		}
		m := s.re.FindStringSubmatch(segs[i])
		if m == nil {
			return nil, nil, false
		}
		if params == nil {
			params = map[string]string{}
		}
		for j, name := range s.names {
			v, err := url.PathUnescape(m[j+1])
			if err != nil {
				v = m[j+1]
			}
			params[name] = v
		}
		if !s.whole {
			rank[i] = 1
		}
	}
	return params, rank, true
}

func better(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func equalRank(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
