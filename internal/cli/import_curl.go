// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/convert/curl"
)

func init() {
	registerImportKind(importKind{
		Name:  "curl",
		Short: "Convert curl command lines to a request file, one entry per command",
		Run:   importCurl,
	})
}

// importCurl converts every `curl` command found in input (a file, or "-"
// for standard input) to one entry of a single generated file.
func importCurl(cmd *cobra.Command, input string, opts convert.Options) (convert.Output, error) {
	data, err := readImportInput(cmd, input)
	if err != nil {
		return convert.Output{}, err
	}
	return curl.ImportInput(input, data, opts.Dialect())
}
