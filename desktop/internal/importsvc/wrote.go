// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"regexp"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// Wrote is a line of the collection and what the import wrote for it,
// for the result's "What the importer wrote".
type Wrote struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Kind is "converted" (written as request file syntax) or "comment"
	// (kept in a comment, never run).
	Kind string `json:"kind"`
}

// maxWrote is how many lines the result shows.
const maxWrote = 6

var (
	// The request and response lines of a written file.
	requestLineRE  = regexp.MustCompile(`^([A-Z]+) (\S+)$`)
	responseLineRE = regexp.MustCompile(`^HTTP(?:/\S+)? (\d{3})\b`)
	// The status checks the converter writes as the response line (as
	// internal/convert/postman reads them).
	statusCheckRE = regexp.MustCompile(`^pm\.response\.to\.have\.status\(\s*(\d{3})\s*[,)]|^pm\.expect\(pm\.response\.code\)\.to\.(?:eql|equal)\(\s*(\d{3})\s*\)`)
	// The base of a written URL: {{base_url}} or scheme://host.
	urlBaseRE = regexp.MustCompile(`^(?:\{\{[^}]*\}\}|[a-z]+://[^/]*)`)
)

// wrote lists, for a Postman import, status checks written as response
// lines, path variables written as {{name}}, then the test script
// statements kept as comments: converted lines first, at most maxWrote.
func (p *plan) wrote() []Wrote {
	vars := pathVariables(p.data)
	var converted, comments []Wrote
	// The same line in several requests is listed once.
	seen := map[Wrote]bool{}
	add := func(list *[]Wrote, w Wrote) {
		if !seen[w] {
			seen[w] = true
			*list = append(*list, w)
		}
	}
	for _, f := range p.out.Files {
		for _, e := range writtenEntries(string(syntax.Format(f.File))) {
			path := urlBaseRE.ReplaceAllString(e.url, "")
			from := path
			for _, v := range vars {
				from = strings.ReplaceAll(from, "/{{"+v+"}}", "/:"+v)
			}
			if from != path {
				add(&converted, Wrote{From: e.method + " " + from, To: e.method + " " + path, Kind: "converted"})
			}
			for _, s := range e.script {
				m := statusCheckRE.FindStringSubmatch(s)
				if m != nil && e.status != "" && (m[1] == e.status || m[2] == e.status) {
					add(&converted, Wrote{From: s, To: "HTTP " + e.status, Kind: "converted"})
				} else {
					add(&comments, Wrote{From: s, To: "# " + s, Kind: "comment"})
				}
			}
		}
	}
	// Both kinds shown when there are both.
	n := min(len(converted), maxWrote-min(len(comments), 2))
	out := append([]Wrote{}, converted[:n]...)
	return append(out, comments[:min(len(comments), maxWrote-n)]...)
}

// writtenEntry is an entry of a written file: its request line, expected
// status and the statements of its test script comment.
type writtenEntry struct {
	method, url, status string
	script              []string
}

// writtenEntries reads the entries of a file the converter wrote. Only
// pm.* statements of a test script are kept (not the pm.test wrapper).
func writtenEntries(text string) []writtenEntry {
	var out []writtenEntry
	var script []string
	test := false
	for _, l := range strings.Split(text, "\n") {
		if c, ok := strings.CutPrefix(l, "#"); ok {
			c = strings.TrimSpace(c)
			if strings.HasSuffix(c, "script (never executed):") {
				test = strings.HasPrefix(c, "test ")
				continue
			}
			s := strings.TrimSuffix(c, ";")
			if test && strings.HasPrefix(s, "pm.") && !strings.HasPrefix(s, "pm.test(") {
				script = append(script, s)
			}
			continue
		}
		if m := responseLineRE.FindStringSubmatch(l); m != nil && len(out) > 0 {
			out[len(out)-1].status = m[1]
			continue
		}
		if m := requestLineRE.FindStringSubmatch(l); m != nil {
			out = append(out, writtenEntry{method: m[1], url: m[2], script: script})
			script, test = nil, false
		}
	}
	return out
}
