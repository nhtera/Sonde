// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package evaldiff

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/antchfx/xpath"

	"github.com/nhtera/sonde/internal/datefmt"
	"github.com/nhtera/sonde/internal/jsonpath"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

const conformanceDir = "../../testdata/conformance/hurl"

// literal is a constant query or filter argument of a conformance file.
type literal struct {
	kind, text, where string
}

// conformanceLiterals collects the placeholder-free JSONPath, XPath, regex
// and date format arguments of every parsable conformance file.
func conformanceLiterals(t *testing.T) []literal {
	t.Helper()
	var lits []literal
	root := os.DirFS(conformanceDir)
	err := fs.WalkDir(root, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(rel, ".hurl") {
			return err
		}
		src, err := fs.ReadFile(root, rel)
		if err != nil {
			return err
		}
		f, perr := syntax.Parse(rel, src, syntax.DialectHurl)
		if perr != nil {
			return nil // parser error suites
		}
		add := func(kind string, n syntax.Node, span syntax.Span) {
			if s, ok := constant(n); ok {
				lits = append(lits, literal{kind, s, rel + ":" + span.Start.String()})
			}
		}
		visit := func(q *syntax.Query, filters []*syntax.FilterItem) {
			switch q.Kind {
			case syntax.QueryJSONPath:
				add("jsonpath", q.Arg, q.Span)
			case syntax.QueryXPath:
				add("xpath", q.Arg, q.Span)
			case syntax.QueryRegex:
				add("regex", q.Arg, q.Span)
			}
			for _, it := range filters {
				switch it.Filter.Kind {
				case syntax.FilterJSONPath:
					add("jsonpath", it.Filter.Arg, it.Filter.Span)
				case syntax.FilterXPath:
					add("xpath", it.Filter.Arg, it.Filter.Span)
				case syntax.FilterRegex, syntax.FilterReplaceRegex:
					add("regex", it.Filter.Arg, it.Filter.Span)
				case syntax.FilterToDate, syntax.FilterFormat, syntax.FilterDateFormat:
					add("date", it.Filter.Arg, it.Filter.Span)
				}
			}
		}
		for _, e := range f.Entries {
			if e.Response == nil {
				continue
			}
			for _, s := range e.Response.Sections {
				for _, c := range s.Captures {
					visit(c.Query, c.Filters)
				}
				for _, a := range s.Asserts {
					visit(a.Query, a.Filters)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return lits
}

// constant returns the text of a template without placeholders or of a
// regex literal.
func constant(n syntax.Node) (string, bool) {
	switch n := n.(type) {
	case *syntax.Regex:
		return n.Pattern, true
	case *syntax.Template:
		var b strings.Builder
		for _, el := range n.Elements {
			s, ok := el.(*syntax.TemplateString)
			if !ok {
				return "", false
			}
			b.WriteString(s.Value)
		}
		return b.String(), true
	}
	return "", false
}

// invalidLiterals are conformance literals that are invalid on purpose (the
// tests expect an error).
var invalidLiterals = map[string]bool{
	"date:%👻":                    true,
	"regex:[x":                   true,
	"xpath://":                   true,
	"xpath:strong(//head/title)": true,
	"jsonpath:":                  true,
	"jsonpath:xxx":               true,
	"jsonpath:$.":                true,
	"jsonpath:$.tags[0]x":        true,
	"jsonpath:$.tags[0,A]":       true,
	"jsonpath:$.tags[0:A]":       true,
	"jsonpath:$.tags[]":          true,
	"jsonpath:$.items[?match(@.name, '[]')].name": true,
	"jsonpath:$.items[?search(@.name, '[]')]":     true,
	"jsonpath:$.items[?match(@.name, 1)].name":    true,
	"jsonpath:$.items[?search(@.name, 1)]":        true,
}

func TestConformanceLiterals(t *testing.T) {
	lits := conformanceLiterals(t)
	counts := map[string]int{}
	for _, l := range lits {
		counts[l.kind]++
		var err error
		switch l.kind {
		case "jsonpath":
			_, err = jsonpath.Parse(l.text)
		case "xpath":
			_, err = xpath.CompileWithNS(l.text, map[string]string{"_": "urn:x", "ns": "urn:y", "a": "urn:a", "b": "urn:b", "c": "urn:c", "soap": "urn:s", "ns1": "urn:1", "isbn": "urn:i", "bk": "urn:bk"})
		case "regex":
			_, err = value.NewRegex(l.text)
		case "date":
			_, err = datefmt.Format(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), l.text)
		}
		invalid := invalidLiterals[l.kind+":"+l.text]
		if (err != nil) != invalid {
			t.Errorf("%s %s %q: error %v", l.where, l.kind, l.text, err)
		}
	}
	for _, kind := range []string{"jsonpath", "xpath", "regex", "date"} {
		if counts[kind] == 0 {
			t.Errorf("no %s literal found", kind)
		}
	}
	t.Logf("checked %v", counts)
}
