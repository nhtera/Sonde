// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check FILE...",
		Short: "Check request files for syntax errors",
		Long: "Check parses every file and prints the first syntax error of each invalid one.\n" +
			"It exits with 2 if any file is invalid.",
		Args: cobra.MinimumNArgs(1),
		RunE: typed(func(cmd *cobra.Command, args []string) error {
			code := ExitOK
			for _, name := range args {
				if err := cmd.Context().Err(); err != nil {
					return err
				}
				if _, c := readInput(cmd.ErrOrStderr(), name); c != ExitOK {
					code = c
				}
			}
			if code != ExitOK {
				return silentExit(code)
			}
			return nil
		}),
	}
}
