// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/convert/opencollection"
)

func init() {
	registerImportKind(importKind{
		Name:  "opencollection",
		Short: "Import a Bruno OpenCollection YAML collection (a file or a directory)",
		Run:   importOpenCollection,
	})
}

// importOpenCollection converts INPUT, a single OpenCollection YAML file
// or a collection directory (opencollection.ImportPath).
func importOpenCollection(_ *cobra.Command, input string, opts convert.Options) (convert.Output, error) {
	return opencollection.ImportPath(input, opts.Dialect())
}
