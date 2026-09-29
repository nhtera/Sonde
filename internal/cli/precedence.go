// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import "github.com/spf13/cobra"

// changed reports whether flag was explicitly given on the command line.
func changed(cmd *cobra.Command, flag string) bool {
	return cmd.Flags().Changed(flag)
}
