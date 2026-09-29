// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/openapi"
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

// importOpenAPI generates one file per operation of the spec at input
// (openapi.Import).
func importOpenAPI(cmd *cobra.Command, input string, opts convert.Options) (convert.Output, error) {
	f := cmd.Flags()
	group, _ := f.GetString("group")
	baseVar, _ := f.GetString("base-url-var")
	remote, _ := f.GetBool("openapi-allow-remote")
	return openapi.Import(cmd.Context(), input, openapi.ImportOptions{
		Group: group, BaseURLVar: baseVar, AllowRemote: remote, Dialect: opts.Dialect(), Output: opts.Output,
	})
}
