// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package cli implements the sonde command line: cobra commands, flag
// mapping and output wiring. It returns exit codes and never exits itself.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

// Execute runs sonde with the process arguments and returns the exit code.
// Ctrl-C cancels the command context, which maps to ExitInterrupted.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// run executes the root command with explicit arguments and writers.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if ctx.Err() != nil {
		// Interrupted: exit 130 whatever the command returned, even if a
		// library flattened the cancellation error.
		return ExitInterrupted
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
	}
	return exitCode(err)
}

// rootOptions holds persistent flags shared by all commands.
type rootOptions struct {
	// color and noColor are reserved for the output renderers (Phase 4).
	color   bool
	noColor bool
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	opts := &rootOptions{}
	root := &cobra.Command{
		Use:           "sonde",
		Short:         "Run and test HTTP requests written in plain text",
		Long:          "Sonde runs and tests HTTP requests written in plain text (.hurl / .sonde files).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	flags := root.PersistentFlags()
	flags.BoolVar(&opts.color, "color", false, "colorize output")
	flags.BoolVar(&opts.noColor, "no-color", false, "do not colorize output")
	root.MarkFlagsMutuallyExclusive("color", "no-color")

	root.AddCommand(newVersionCmd())
	return root
}
