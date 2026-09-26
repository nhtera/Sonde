// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/lsp"
)

func newLSPCmd() *cobra.Command {
	var stdio bool
	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Run the language server over stdio",
		Long: "Lsp runs a Language Server Protocol server for .hurl and .sonde files on\n" +
			"stdin/stdout: diagnostics, completion, hover and formatting. Editors start\n" +
			"it themselves; see docs/guides/editors.md. It never sends HTTP requests.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := lsp.NewServer(lsp.Options{Version: currentBuildInfo().Version, Environ: config.FromOSEnviron()})
			if err != nil {
				return err
			}
			return s.Run(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	// Language clients pass --stdio; stdio is the only transport.
	cmd.Flags().BoolVar(&stdio, "stdio", true, "use stdin/stdout (the only transport)")
	return cmd
}
