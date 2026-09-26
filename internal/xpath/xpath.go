// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package xpath evaluates XPath 1.0 expressions on HTML and XML documents.
package xpath

import (
	"bytes"
	"errors"
	"strings"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"golang.org/x/net/html"

	"github.com/nhtera/sonde/internal/value"
)

// Format selects the parser.
type Format int

// Document formats.
const (
	HTML Format = iota
	XML
)

// ErrInvalidDocument reports input with no root element.
var ErrInvalidDocument = errors.New("no root element")

// ErrEval reports an expression that cannot be compiled or evaluated.
var ErrEval = errors.New("XPath expression is not valid")

// Document is a parsed HTML or XML document.
type Document struct {
	nav        xpath.NodeNavigator
	namespaces map[string]string
}

// Parse parses text as HTML (lenient, namespaces ignored) or XML. For XML,
// every namespace declared in the document is available by its prefix, and
// the first default namespace by the prefix `_`.
func Parse(text string, f Format) (*Document, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrInvalidDocument
	}
	if f == HTML {
		doc, err := htmlquery.Parse(strings.NewReader(trimAfterBody(text)))
		if err != nil || !hasContent(doc) && !hasStartTag(text) {
			return nil, ErrInvalidDocument
		}
		return &Document{nav: htmlquery.CreateXPathNavigator(doc)}, nil
	}
	doc, err := xmlquery.Parse(strings.NewReader(text))
	if err != nil || doc.SelectElement("*") == nil {
		return nil, ErrInvalidDocument
	}
	return &Document{nav: xmlquery.CreateXPathNavigator(doc), namespaces: namespaces(doc)}, nil
}

// hasContent reports whether an HTML document has an element other than
// the html, head and body elements an HTML5 parser always creates, or some
// text: input made only of comments or processing instructions has no
// root element.
func hasContent(n *html.Node) bool {
	switch n.Type {
	case html.ElementNode:
		if n.Data != "html" && n.Data != "head" && n.Data != "body" {
			return true
		}
	case html.TextNode:
		if strings.TrimSpace(n.Data) != "" {
			return true
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasContent(c) {
			return true
		}
	}
	return false
}

// hasStartTag reports whether text contains an element start tag.
func hasStartTag(text string) bool {
	for i := strings.IndexByte(text, '<'); i >= 0 && i+1 < len(text); {
		c := text[i+1]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			return true
		}
		j := strings.IndexByte(text[i+1:], '<')
		if j < 0 {
			return false
		}
		i += 1 + j
	}
	return false
}

// trimAfterBody drops the white space that follows the last </body> tag:
// an HTML5 parser moves it into the body, which would change its text.
func trimAfterBody(text string) string {
	lower := []byte(text) // ASCII only, byte by byte: keeps byte offsets
	for k, c := range lower {
		if c >= 'A' && c <= 'Z' {
			lower[k] = c + 'a' - 'A'
		}
	}
	i := bytes.LastIndex(lower, []byte("</body>"))
	if i < 0 {
		return text
	}
	i += len("</body>")
	rest := strings.TrimSpace(text[i:])
	if rest != "" && !strings.EqualFold(rest, "</html>") {
		return text
	}
	return text[:i] + rest
}

// namespaces collects the prefixes declared in doc, in document order.
func namespaces(doc *xmlquery.Node) map[string]string {
	ns := map[string]string{}
	var walk func(n *xmlquery.Node)
	walk = func(n *xmlquery.Node) {
		if n.Type == xmlquery.ElementNode {
			for _, a := range n.Attr {
				switch {
				case a.Name.Space == "xmlns":
					if _, ok := ns[a.Name.Local]; !ok {
						ns[a.Name.Local] = a.Value
					}
				case a.Name.Space == "" && a.Name.Local == "xmlns":
					if _, ok := ns["_"]; !ok {
						ns["_"] = a.Value
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return ns
}

// Eval evaluates expr: numbers give a Float, booleans a Bool, strings a
// String and node-sets a Nodeset with their size.
func (d *Document) Eval(expr string) (v value.Value, err error) {
	defer func() {
		if recover() != nil {
			v, err = nil, ErrEval
		}
	}()
	e, err := xpath.CompileWithNS(expr, d.namespaces)
	if err != nil {
		return nil, ErrEval
	}
	switch r := e.Evaluate(d.nav.Copy()).(type) {
	case float64:
		return value.Float(r), nil
	case bool:
		return value.Bool(r), nil
	case string:
		return value.String(r), nil
	case *xpath.NodeIterator:
		n := 0
		for r.MoveNext() {
			n++
		}
		return value.Nodeset(n), nil
	}
	return nil, ErrEval
}
