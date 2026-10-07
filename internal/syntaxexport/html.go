// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package syntaxexport renders a parsed .hurl file in the reference formatter's
// export formats: syntax-highlighted HTML and the AST as JSON.
package syntaxexport

import (
	_ "embed"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// css is the reference formatter's stylesheet for standalone HTML,
// unchanged (Apache-2.0, see NOTICE).
//
//go:embed hurl.css
var css string

// htmlClasses maps a token kind to its span class; kinds not listed are
// written without a span.
var htmlClasses = map[syntax.TokenKind]string{
	syntax.TokenMethod:      "method",
	syntax.TokenURL:         "url",
	syntax.TokenString:      "string",
	syntax.TokenFilename:    "filename",
	syntax.TokenSection:     "section-header",
	syntax.TokenQuery:       "query-type",
	syntax.TokenFilter:      "filter-type",
	syntax.TokenPredicate:   "predicate-type",
	syntax.TokenNot:         "not",
	syntax.TokenNumber:      "number",
	syntax.TokenBoolean:     "boolean",
	syntax.TokenNull:        "null",
	syntax.TokenComment:     "comment",
	syntax.TokenVersion:     "version",
	syntax.TokenStatus:      "number",
	syntax.TokenJSON:        "json",
	syntax.TokenXML:         "xml",
	syntax.TokenMultiline:   "multiline",
	syntax.TokenBase64:      "base64",
	syntax.TokenHex:         "hex",
	syntax.TokenRegex:       "regex",
	syntax.TokenPlaceholder: "expr",
	syntax.TokenUnit:        "unit",
}

// htmlEscaper escapes every text the HTML holds; the class names are
// fixed strings.
var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// HTML renders f as a highlighted `<pre>` block; standalone wraps it in a
// complete document with the reference stylesheet inline.
func HTML(f *syntax.File, standalone bool) string {
	noBOM := *f // the reference drops a byte order mark on reading
	noBOM.BOM = false
	var b strings.Builder
	b.WriteString(`<pre><code class="language-hurl">`)
	for _, t := range syntax.Tokens(&noBOM) {
		switch t.Kind {
		case syntax.TokenOpen:
			b.WriteString(`<span class="` + t.Text + `">`)
		case syntax.TokenClose:
			b.WriteString("</span>")
		default:
			class, ok := htmlClasses[t.Kind]
			if ok {
				b.WriteString(`<span class="` + class + `">`)
			}
			b.WriteString(htmlEscaper.Replace(t.Text))
			if ok {
				b.WriteString("</span>")
			}
		}
	}
	b.WriteString("</code></pre>")
	if !standalone {
		return b.String()
	}
	return `<!DOCTYPE html>
<html>
    <head>
        <meta charset="utf-8">
        <title>Hurl File</title>
        <style>
` + css + `
        </style>
    </head>
    <body>
` + b.String() + `
    </body>
</html>
`
}
