// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"path/filepath"
	"strings"

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
// for standard input) to one entry of a single generated file, named
// after input's stem ("curl" for standard input).
func importCurl(cmd *cobra.Command, input string, opts convert.Options) (convert.Output, error) {
	data, err := readImportInput(cmd, input)
	if err != nil {
		return convert.Output{}, err
	}
	res, err := curl.Import(data, opts.Dialect())
	if err != nil {
		return convert.Output{}, err
	}
	return convert.Output{
		Files:    []convert.GeneratedFile{{Path: curlOutputStem(input), File: res.File}},
		Warnings: res.Warnings,
		Skipped:  res.Skipped,
	}, nil
}

// curlOutputStem names the single generated file: input's base name
// without its extension, or "curl" for standard input or an input whose
// name sanitizes to nothing (the writer would fall back to "request"
// otherwise, which is a fine but less informative default).
func curlOutputStem(input string) string {
	if input == "-" {
		return "curl"
	}
	base := filepath.Base(input)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		return "curl"
	}
	return stem
}
