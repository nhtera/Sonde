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
	"time"

	"github.com/spf13/cobra"
)

// ctrlCGrace is how long a run keeps going after the first Ctrl-C before a
// second one is required: entry boundaries are respected during this
// window (RunAll's stop channel), then in-flight requests are aborted
// (context cancellation) whether or not a second signal arrived.
const ctrlCGrace = 5 * time.Second

// Execute runs sonde with the process arguments and returns the exit code.
// Ctrl-C is two-stage, matching the upstream CLI: the first SIGINT stops
// scheduling new files and lets running ones finish their current entry;
// if a second SIGINT arrives, or ctrlCGrace elapses first, in-flight
// requests are aborted too. Either way the run then exits 130.
func Execute() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan struct{})
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	done := make(chan struct{})
	defer close(done)
	go twoStageCtrlC(sig, done, ctrlCGrace, stop, cancel, func() { signal.Reset(os.Interrupt) })
	return run(withStop(ctx, stop), os.Args[1:], os.Stdout, os.Stderr)
}

// twoStageCtrlC implements Execute's Ctrl-C policy against sig, a channel
// of raw OS signals (only os.Interrupt is ever sent to it, by
// signal.Notify): the first signal closes stop, the second — or grace
// elapsing first, whichever comes first — calls resetSignals then cancel.
// resetSignals restores the default SIGINT disposition (Execute passes
// signal.Reset), so once a run is being forcefully aborted a further
// Ctrl-C kills the process immediately instead of silently doing nothing
// while something hangs (unwinding, report writing, ...). It returns
// early, doing nothing further, once done closes (the run finished on its
// own before either stage). Split out from Execute so a test can drive it
// with a fake signal channel, a short grace and a spied resetSignals
// instead of a real process signal, a multi-second wait and a real,
// process-wide signal disposition change.
func twoStageCtrlC(sig <-chan os.Signal, done <-chan struct{}, grace time.Duration, stop chan<- struct{}, cancel context.CancelFunc, resetSignals func()) {
	select {
	case <-sig:
	case <-done:
		return
	}
	close(stop)
	select {
	case <-sig:
	case <-time.After(grace):
	case <-done:
	}
	resetSignals()
	cancel()
}

// stopKey is the context key withStop/stopFromContext use.
type stopKey struct{}

// withStop attaches stop to ctx, retrievable with stopFromContext.
func withStop(ctx context.Context, stop <-chan struct{}) context.Context {
	return context.WithValue(ctx, stopKey{}, stop)
}

// stopFromContext returns the channel Execute's first Ctrl-C closes, or
// ctx.Done() itself when ctx carries none (every other caller, including
// tests): canceling ctx directly then plays both roles at once, which is
// the single-stage behavior those callers expect.
func stopFromContext(ctx context.Context) <-chan struct{} {
	if s, ok := ctx.Value(stopKey{}).(<-chan struct{}); ok {
		return s
	}
	return ctx.Done()
}

// run executes the root command with explicit arguments and writers.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if interrupted(ctx) {
		// Interrupted: exit 130 whatever the command returned, even if a
		// library flattened the cancellation error, and even if the run
		// went on to spend an arbitrary amount of time writing reports
		// after the first Ctrl-C — the exit code must not depend on that
		// timing (only ctx.Err() would: a report write finishing inside
		// the grace period, before ctx is ever canceled, must not look
		// like a clean exit).
		return ExitInterrupted
	}
	if err != nil && !isSilent(err) {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
	}
	return exitCode(err)
}

// interrupted reports whether ctx was ever asked to stop: either canceled
// outright, or (the first Ctrl-C stage) its stop channel closed. A
// non-blocking receive is enough — both channels are only ever closed,
// never sent to.
func interrupted(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	select {
	case <-stopFromContext(ctx):
		return true
	default:
		return false
	}
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
			return runMain(cmd, runOpts, args, false)
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

	root.AddCommand(newVersionCmd(), newCheckCmd(), newFmtCmd(), newImportCmd(), newRunCmd(), newTestCmd())
	return root
}
