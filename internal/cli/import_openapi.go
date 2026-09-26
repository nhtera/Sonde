// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/openapi"
	"github.com/nhtera/sonde/internal/syntax"
)

func init() {
	registerImportKind(importKind{
		Name:  "openapi",
		Short: "Generate request files from an OpenAPI spec, one per operation",
		RegisterFlags: func(cmd *cobra.Command) {
			f := cmd.Flags()
			f.String("group", openapi.GroupTag, "lays files out by first tag (tag), first path segment (path) or in one directory (flat)")
			f.String("base-url-var", "base_url", "variable prefixing every URL")
			f.Bool("openapi-allow-remote", false, "allows a remote spec and remote $ref targets")
		},
		Run: importOpenAPI,
	})
}

// importOpenAPI generates one file per operation of the spec at input, and
// a sonde.yaml whose default environment sets the base URL and the path
// parameters; it names the spec as the project's contract when the spec
// lies inside the output directory.
func importOpenAPI(cmd *cobra.Command, input string) (convert.Output, error) {
	f := cmd.Flags()
	group, _ := f.GetString("group")
	baseVar, _ := f.GetString("base-url-var")
	remote, _ := f.GetBool("openapi-allow-remote")
	ext, _ := f.GetString("ext")
	dir, _ := f.GetString("output")

	spec, err := openapi.Load(cmd.Context(), input, openapi.LoadOptions{AllowRemote: remote})
	if err != nil {
		return convert.Output{}, err
	}
	gen, err := spec.Generate(openapi.GenerateOptions{Group: group, BaseURLVar: baseVar, Dialect: syntax.DialectFor("x." + ext)})
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
		OpenAPISpec:  specInside(dir, input),
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
