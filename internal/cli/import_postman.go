// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/convert/postman"
)

func init() {
	registerImportKind(importKind{
		Name:  "postman",
		Short: "Convert a Postman v2.1 collection to request files",
		RegisterFlags: func(cmd *cobra.Command) {
			f := cmd.Flags()
			f.String("group", postman.GroupRequest, `lays files out one per request ("request") or one per folder, chaining its requests ("folder")`)
			f.StringArray("environment", nil, "a Postman environment JSON file (repeatable); each becomes a sonde.yaml environment")
		},
		Run: importPostman,
	})
}

// importPostman converts a Postman Collection v2.1 (or v2.0) document at
// input to Sonde request files, folding its variables, and every given
// --environment file's, into a sonde.yaml skeleton.
func importPostman(cmd *cobra.Command, input string, opts convert.Options) (convert.Output, error) {
	f := cmd.Flags()
	group, _ := f.GetString("group")
	envPaths, _ := f.GetStringArray("environment")

	data, err := readImportInput(cmd, input)
	if err != nil {
		return convert.Output{}, err
	}
	envs := make([]postman.EnvironmentFile, 0, len(envPaths))
	for _, path := range envPaths {
		raw, err := convert.ReadInput(path, convert.MaxInput)
		if err != nil {
			return convert.Output{}, fmt.Errorf("--environment: %w", err)
		}
		envs = append(envs, postman.EnvironmentFile{FileName: path, Data: raw})
	}
	return postman.Import(data, postman.Options{Group: group, Environments: envs, Dialect: opts.Dialect()})
}
