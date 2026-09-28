// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bytes"
	"context"
	"errors"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nhtera/sonde/internal/syntax"
)

// maxCheckPaths bounds the files of one sonde_check call.
const maxCheckPaths = 100

type checkInput struct {
	Paths []string `json:"paths" jsonschema:"request files (.hurl, .sonde), relative to the server root"`
}

type checkOutput struct {
	Files []checkedFile `json:"files"`
}

type checkedFile struct {
	Path string `json:"path"`
	// OK is set when the file parses.
	OK bool `json:"ok"`
	// Problem is why the file could not be read (outside the root,
	// missing, not a request file).
	Problem string       `json:"problem,omitempty"`
	Error   *syntaxError `json:"error,omitempty" jsonschema:"the first syntax error"`
	Entries []entry      `json:"entries,omitempty"`
}

type syntaxError struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Description string `json:"description"`
	Message     string `json:"message"`
}

type entry struct {
	Index    int      `json:"index" jsonschema:"1-based"`
	Line     int      `json:"line"`
	Method   string   `json:"method"`
	URL      string   `json:"url" jsonschema:"as written, templates included"`
	Sections []string `json:"sections,omitempty" jsonschema:"request and response sections, as written"`
	Options  []string `json:"options,omitempty" jsonschema:"names of the [Options] of the entry"`
}

func (s *server) check(_ context.Context, _ *sdk.CallToolRequest, in checkInput) (*sdk.CallToolResult, checkOutput, error) {
	start := time.Now()
	out := checkOutput{Files: []checkedFile{}}
	if len(in.Paths) == 0 || len(in.Paths) > maxCheckPaths {
		return nil, out, errors.New("paths: give 1 to 100 request files")
	}
	failed := 0
	for _, name := range in.Paths {
		cf := s.checkFile(name)
		if !cf.OK {
			failed++
		}
		out.Files = append(out.Files, cf)
	}
	s.audit("sonde_check", start, "files=%d invalid=%d", len(in.Paths), failed)
	return nil, out, nil
}

func (s *server) checkFile(name string) checkedFile {
	rf, err := s.readRequestFile(name)
	if err != nil {
		return checkedFile{Path: name, Problem: err.Error()}
	}
	cf := checkedFile{Path: rf.rel}
	f, err := syntax.Parse(rf.rel, rf.src, syntax.DialectFor(rf.rel))
	if err != nil {
		var perr *syntax.Error
		if !errors.As(err, &perr) {
			cf.Problem = err.Error()
			return cf
		}
		cf.Error = &syntaxError{Line: perr.Pos.Line, Column: perr.Pos.Col, Description: perr.Description(), Message: perr.Message()}
		return cf
	}
	cf.OK = true
	// Spans skip a byte order mark.
	src := bytes.TrimPrefix(rf.src, []byte("\uFEFF"))
	for i, e := range f.Entries {
		req := e.Request
		en := entry{Index: i + 1, Line: req.Method.Span.Start.Line, Method: req.Method.Value}
		if req.URL != nil {
			en.URL = spanText(src, req.URL.Span)
		}
		sections := req.Sections
		if e.Response != nil {
			sections = append(sections[:len(sections):len(sections)], e.Response.Sections...)
		}
		for _, sec := range sections {
			en.Sections = append(en.Sections, sec.Name)
			if sec.Kind == syntax.SectionOptions {
				for _, o := range sec.Options {
					en.Options = append(en.Options, o.Name)
				}
			}
		}
		cf.Entries = append(cf.Entries, en)
	}
	return cf
}

func spanText(src []byte, sp syntax.Span) string {
	if sp.Start.Offset < 0 || sp.End.Offset > len(src) || sp.Start.Offset > sp.End.Offset {
		return ""
	}
	return string(src[sp.Start.Offset:sp.End.Offset])
}
