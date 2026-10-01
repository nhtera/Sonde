// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"context"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/internal/convert/postman"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Suggestions: after a Postman import, what its test scripts and OAuth2
// settings translate to (asserts, captures, a login request), offered per
// file as a diff. Nothing is run; a script that is not one of the known
// forms stays a comment. Each edit goes through the syntax edit layer,
// which refuses one that would add anything but its own row or entry.

// Suggestion is the suggestions for one imported file.
type Suggestion struct {
	Path   string   `json:"path"` // project path
	Labels []string `json:"labels"`
	Before string   `json:"before"`
	After  string   `json:"after"`
	// Error is why the suggestions can not be applied (After is empty).
	Error string `json:"error,omitempty"`
}

// Suggestions returns the suggestions of a Postman import, against the
// files as they are now.
func (s *Service) Suggestions(ctx context.Context, req Request) ([]Suggestion, error) {
	if req.Kind != Postman {
		return []Suggestion{}, nil
	}
	p, err := s.plan(ctx, req)
	if err != nil {
		return nil, err
	}
	all, err := postman.Suggest(p.data, p.postOpts)
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	out := []Suggestion{}
	index := map[string]int{}
	for _, fs := range all {
		if fs.File >= len(p.out.Files) {
			continue
		}
		rel := p.files[fs.File].Path
		i, ok := index[rel]
		if !ok {
			// Only a file as this import wrote it: not one the user kept,
			// edited, or already applied suggestions to.
			src, err := p.root.ReadFile(p.project(rel))
			if err != nil || string(src) != string(p.files[fs.File].Data) {
				continue
			}
			out = append(out, Suggestion{Path: p.project(rel), Before: string(src), After: string(src)})
			i = len(out) - 1
			index[rel] = i
		}
		sg := &out[i]
		sg.Labels = append(sg.Labels, fs.Label)
		if sg.Error != "" {
			continue
		}
		after, err := fs.Apply(rel, []byte(sg.After))
		if err != nil {
			sg.Error, sg.After = err.Error(), ""
			continue
		}
		sg.After = string(after)
	}
	return out, nil
}

// Accept applies the suggestions of file (a project path) to it, as they
// were reviewed: the file must still be as the import wrote it (once
// applied, it has no suggestions any more).
func (s *Service) Accept(ctx context.Context, req Request, file string) error {
	all, err := s.Suggestions(ctx, req)
	if err != nil {
		return err
	}
	for _, sg := range all {
		if sg.Path != file {
			continue
		}
		if sg.Error != "" {
			return apperr.New(apperr.Invalid, sg.Error)
		}
		root := s.project()
		if root == nil {
			return apperr.New(apperr.NotFound, "no project is open")
		}
		return writeProject(root, file, []byte(sg.After))
	}
	return apperr.New(apperr.Stale, file+" changed since the import: its suggestions no longer apply")
}

// writeProject replaces the project file rel (an imported request file,
// 0644 as the import wrote it).
func writeProject(root *sandbox.Root, rel string, data []byte) error {
	if err := root.WriteFileAtomic(rel, data, 0o644); err != nil {
		return apperr.Wrap(apperr.Invalid, err)
	}
	return nil
}
