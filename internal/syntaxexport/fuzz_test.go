// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// markupRE matches the only markup HTML writes itself.
var markupRE = regexp.MustCompile(`<pre><code class="language-hurl">|</code></pre>|<span class="[a-z0-9-]+">|</span>`)

var htmlUnescaper = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")

// FuzzExport checks, for any file that parses, that the HTML export holds
// exactly the file's text, escaped: removing the exporter's own markup and
// unescaping gives the source back (so no input text can pass for markup,
// and none is lost); and that the JSON export is valid JSON.
func FuzzExport(f *testing.F) {
	seeds, _ := filepath.Glob("../../testdata/conformance/hurlfmt/tests_export/*.hurl")
	for _, path := range seeds {
		if b, err := os.ReadFile(path); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte("GET http://x/<script>\nX-A: </span><b>&amp;\n# <!-- c -->\n```\n<html>\n```\n"))
	f.Add([]byte("GET http://x\nHTTP 200\n[Asserts]\nbody == 1" + strings.Repeat("0", 310) + ".0\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		file, err := syntax.Parse("f.hurl", src, syntax.DialectHurl)
		if err != nil {
			return
		}
		text := markupRE.ReplaceAllString(HTML(file, false), "")
		if strings.ContainsAny(text, "<>") {
			t.Fatalf("unescaped markup in the HTML of %q:\n%s", src, text)
		}
		noBOM := *file
		noBOM.BOM = false
		if got, want := htmlUnescaper.Replace(text), string(syntax.Print(&noBOM)); got != want {
			t.Fatalf("HTML text of %q is not the source:\n got %q\nwant %q", src, got, want)
		}
		if out := JSON(file); !json.Valid([]byte(out)) {
			t.Fatalf("invalid JSON for %q:\n%s", src, out)
		}
	})
}
