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
	if err != nil && !isSilent(err) {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
	}
	return exitCode(err)
}

// rootOptions holds persistent flags shared by all commands.
type rootOptions struct {
	color   bool
	noColor bool
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	opts := &rootOptions{}
	runOpts := &runOptions{}
	root := &cobra.Command{
		Use:   "sonde [options] FILE...",
		Short: "Run and test HTTP requests written in plain text",
		Long: "Sonde runs and tests HTTP requests written in plain text (.hurl / .sonde files).\n" +
			"Any argument that is not one of the commands below is treated as\n" +
			"`sonde run [options] FILE...`: `sonde a.hurl b.hurl` runs both files, and\n" +
			"`sonde` with no FILE reads a single input from standard input.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMain(cmd, runOpts, args)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return NewExitError(ExitUsage, err)
	})
	// No generated completion command: `sonde NAME` must stay free for files.
	root.CompletionOptions.DisableDefaultCmd = true

	flags := root.PersistentFlags()
	flags.BoolVar(&opts.color, "color", false, "colorize output")
	flags.BoolVar(&opts.noColor, "no-color", false, "do not colorize output")
	// --color and --no-color are not mutually exclusive here: like the
	// upstream CLI, giving both is not a usage error; --no-color simply
	// wins (see buildRunContext).
	addRunFlags(root, runOpts)

	root.AddCommand(newVersionCmd(), newCheckCmd(), newFmtCmd(), newRunCmd())
	return root
}
