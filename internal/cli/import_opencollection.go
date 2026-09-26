// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"os"

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

// importOpenCollection converts INPUT — a single OpenCollection YAML file,
// or a collection directory (docs/decisions/0002-opencollection-mapping.md)
// — to Sonde request files. A directory is read directly (os.Root, inside
// the package); a file goes through the shared readImportInput.
func importOpenCollection(cmd *cobra.Command, input string, opts convert.Options) (convert.Output, error) {
	st, err := os.Stat(input)
	if err != nil {
		return convert.Output{}, err
	}
	if st.IsDir() {
		return opencollection.ImportDir(input, opts.Dialect())
	}
	if !st.Mode().IsRegular() {
		return convert.Output{}, fmt.Errorf("%s: not a regular file or directory", input)
	}
	data, err := readImportInput(cmd, input)
	if err != nil {
		return convert.Output{}, err
	}
	return opencollection.ImportFile(data, opts.Dialect())
}
