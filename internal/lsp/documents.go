// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// maxDiagnostics caps parse errors per document so one bad edit cannot
// flood the editor with cascading errors.
const maxDiagnostics = 50

// document is an open text document and its parse.
type document struct {
	uri     string
	path    string // local file path, "" when the URI is not a file URI
	version int32
	text    string
	dialect syntax.Dialect
	lines   *lineIndex

	// file holds the entries that parsed; errs the parse errors (at most
	// maxDiagnostics), in source order.
	file *syntax.File
	errs []*syntax.Error
}

func newDocument(uri string, version int32, text string, utf16 bool) *document {
	d := &document{uri: uri, path: uriToPath(uri), version: version}
	d.dialect = syntax.DialectFor(uriBase(uri))
	d.setText(text, utf16)
	return d
}

// setText replaces the text and reparses it.
func (d *document) setText(text string, utf16 bool) {
	d.text = text
	d.lines = newLineIndex(text, utf16)
	d.file, d.errs = syntax.ParseAll(d.uri, []byte(text), d.dialect, maxDiagnostics)
}

// uriToPath returns the local path of a file URI, or "" for other schemes.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return ""
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		// file:///c:/x → c:/x
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
	}
	return filepath.Clean(filepath.FromSlash(p))
}

// uriBase returns the last path segment of uri, for dialect detection.
func uriBase(uri string) string {
	if u, err := url.Parse(uri); err == nil && u.Path != "" {
		return u.Path[strings.LastIndexByte(u.Path, '/')+1:]
	}
	return uri
}
