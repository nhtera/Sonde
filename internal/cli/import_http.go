// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/convert/httpfile"
)

func init() {
	registerImportKind(importKind{
		Name:  "http",
		Short: "Convert a .http file (JetBrains HTTP Client or VS Code REST Client) to Sonde request files",
		RegisterFlags: func(cmd *cobra.Command) {
			cmd.Flags().StringArray("env-file", nil,
				"a JetBrains http-client.env.json file to import as sonde.yaml environments (repeatable); "+
					"a sibling *.private.env.json is read too, as a secrets stub")
		},
		Run: importHTTPFile,
	})
}

// importHTTPFile converts one .http file (or stdin) into one Sonde request
// file, one entry per request, plus a sonde.yaml whose default environment
// holds the file's own "@var = value" variables.
func importHTTPFile(cmd *cobra.Command, input string, opts convert.Options) (convert.Output, error) {
	data, err := readImportInput(cmd, input)
	if err != nil {
		return convert.Output{}, err
	}
	envFiles, _ := cmd.Flags().GetStringArray("env-file")
	return httpfile.Import(httpfile.StemFor(input), data, opts.Dialect(), httpfile.Options{EnvFiles: envFiles})
}
