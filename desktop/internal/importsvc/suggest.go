// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
	// After is the file with every change applied.
	After string `json:"after"`
	// Changes are the file's suggestions one by one, to accept or reject
	// each on its own.
	Changes []Change `json:"changes"`
	// Error is why the suggestions can not be applied (After is empty).
	Error string `json:"error,omitempty"`
}

// Change is one suggestion of a file: what one script or setting
// translates to.
type Change struct {
	Index int    `json:"index"`
	Label string `json:"label"`
	// Line is the first line (1-based) the change adds or changes, in After.
	Line int `json:"line"`
	// After is the file with this change alone.
	After string `json:"after"`
	// Error is why this change can not be applied.
	Error string `json:"error,omitempty"`
}

// fileSuggestions are a file's suggestions in the order they apply.
type fileSuggestions struct {
	rel string // the path in the import
	all []postman.FileSuggestion
}

// Suggestions returns the suggestions of a Postman import, against the
// files as they are now.
func (s *Service) Suggestions(ctx context.Context, req Request) ([]Suggestion, error) {
	out, _, err := s.suggestions(ctx, req)
	return out, err
}

func (s *Service) suggestions(ctx context.Context, req Request) ([]Suggestion, []fileSuggestions, error) {
	if req.Kind != Postman {
		return []Suggestion{}, nil, nil
	}
	p, err := s.plan(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	all, err := postman.Suggest(p.data, p.postOpts)
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.Invalid, err)
	}
	out := []Suggestion{}
	files := []fileSuggestions{}
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
			out = append(out, Suggestion{Path: p.project(rel), Before: string(src), After: string(src), Changes: []Change{}})
			files = append(files, fileSuggestions{rel: rel})
			i = len(out) - 1
			index[rel] = i
		}
		sg := &out[i]
		files[i].all = append(files[i].all, fs)
		sg.Labels = append(sg.Labels, fs.Label)
		ch := Change{Index: len(sg.Changes), Label: fs.Label}
		if alone, err := fs.Apply(rel, []byte(sg.Before)); err != nil {
			ch.Error = err.Error()
		} else {
			ch.After, ch.Line = string(alone), firstChangedLine(sg.Before, string(alone))
		}
		sg.Changes = append(sg.Changes, ch)
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
	return out, files, nil
}

// firstChangedLine is the first line (1-based) where after differs from
// before.
func firstChangedLine(before, after string) int {
	b, a := strings.Split(before, "\n"), strings.Split(after, "\n")
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return i + 1
		}
	}
	return len(a)
}

// Accept applies the suggestions of file (a project path) to it, as they
// were reviewed: the changes picked (all of them when picked is nil), in
// the order they apply. The file must still be as the import wrote it
// (once applied, it has no suggestions any more).
func (s *Service) Accept(ctx context.Context, req Request, file string, picked []int) error {
	all, files, err := s.suggestions(ctx, req)
	if err != nil {
		return err
	}
	for i, sg := range all {
		if sg.Path != file {
			continue
		}
		after := []byte(sg.After)
		if picked == nil {
			if sg.Error != "" {
				return apperr.New(apperr.Invalid, sg.Error)
			}
		} else {
			if len(picked) == 0 {
				return nil // nothing accepted: the file stays as imported
			}
			for _, j := range picked {
				if j < 0 || j >= len(files[i].all) {
					return apperr.New(apperr.Invalid, fmt.Sprintf("%s has no change %d", file, j))
				}
			}
			after = []byte(sg.Before)
			for j, fs := range files[i].all {
				if !slices.Contains(picked, j) {
					continue
				}
				if after, err = fs.Apply(files[i].rel, after); err != nil {
					return apperr.Wrap(apperr.Invalid, err)
				}
			}
		}
		root := s.project()
		if root == nil {
			return apperr.New(apperr.NotFound, "no project is open")
		}
		return writeProject(root, file, after)
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
