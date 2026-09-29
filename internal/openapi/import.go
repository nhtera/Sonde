// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// ImportOptions configure Import.
type ImportOptions struct {
	// Group lays files out by first tag (GroupTag), first path segment or
	// in one directory.
	Group string
	// BaseURLVar is the variable prefixing every URL.
	BaseURLVar string
	// AllowRemote allows a remote spec and remote $ref targets.
	AllowRemote bool
	Dialect     syntax.Dialect
	// Output is the directory the files go to: the spec is named as the
	// project's contract when it lies inside it.
	Output string
}

// Import generates one file per operation of the spec at input (a path or
// a URL), and a sonde.yaml whose default environment sets the base URL and
// the path parameters.
func Import(ctx context.Context, input string, opts ImportOptions) (convert.Output, error) {
	spec, err := Load(ctx, input, LoadOptions{AllowRemote: opts.AllowRemote})
	if err != nil {
		return convert.Output{}, err
	}
	gen, err := spec.Generate(GenerateOptions{Group: opts.Group, BaseURLVar: opts.BaseURLVar, Dialect: opts.Dialect})
	if err != nil {
		return convert.Output{}, err
	}
	var out convert.Output
	for _, g := range gen.Files {
		out.Files = append(out.Files, convert.GeneratedFile{Path: g.Path, File: g.File})
	}
	for _, w := range gen.Warnings {
		out.Warnings = append(out.Warnings, convert.Warning{Kind: w.Kind, Message: w.Message})
	}
	for _, s := range gen.Skipped {
		out.Skipped = append(out.Skipped, convert.Skipped{Name: s.Kind, Reason: s.Message})
	}
	skeleton := config.ProjectSkeleton{
		Environments: map[string]config.EnvironmentSkeleton{"default": {Variables: gen.Variables}},
		DefaultEnv:   "default",
		OpenAPISpec:  specInside(opts.Output, input),
	}
	if out.ProjectYAML, err = config.EmitProject(skeleton); err != nil {
		return convert.Output{}, err
	}
	return out, nil
}

// specInside returns the spec path relative to dir, or "" when the spec is
// remote or outside dir (sonde.yaml can only name a spec inside its own
// directory).
func specInside(dir, spec string) string {
	if strings.Contains(spec, "://") {
		return ""
	}
	d, err1 := filepath.Abs(dir)
	s, err2 := filepath.Abs(spec)
	if err1 != nil || err2 != nil {
		return ""
	}
	rel, err := filepath.Rel(d, s)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}
