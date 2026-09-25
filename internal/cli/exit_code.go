// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// Process exit codes (docs/architecture.md §5), including the documented
// ExitInterrupted.
const (
	ExitOK          = 0   // success
	ExitUsage       = 1   // CLI usage / option error
	ExitParse       = 2   // input file parse error (also unreadable input file)
	ExitRuntime     = 3   // runtime error: connect, TLS, timeout, sandbox denial, unsupported option
	ExitAssert      = 4   // assert / contract failure
	ExitUndefined   = 127 // undefined error, e.g. a report cannot be written
	ExitInterrupted = 130 // interrupted (Ctrl-C)
)

// ExitError attaches a process exit code to an error. Commands wrap every
// error that must not map to ExitUsage.
type ExitError struct {
	Code int
	Err  error
}

// NewExitError wraps err with the given exit code.
func NewExitError(code int, err error) *ExitError {
	return &ExitError{Code: code, Err: err}
}

// silentExit ends a command with code after it already reported its own
// diagnostics; nothing more is printed.
func silentExit(code int) *ExitError {
	return &ExitError{Code: code}
}

func isSilent(err error) bool {
	exitErr, ok := errors.AsType[*ExitError](err)
	return ok && exitErr.Err == nil
}

// typed wraps a command body so any error it returns carries an exit code:
// untyped errors become ExitUndefined. Only cobra's own errors (unknown
// command, bad arguments) stay untyped and map to ExitUsage.
func typed(f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := f(cmd, args)
		if err == nil || errors.Is(err, context.Canceled) {
			return err
		}
		if _, ok := errors.AsType[*ExitError](err); ok {
			return err
		}
		return NewExitError(ExitUndefined, err)
	}
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// exitCode maps an error returned by a command to a process exit code.
// Cancellation wins over any wrapped code; untyped errors come from cobra
// itself (unknown command, bad flag, wrong arguments) and are usage errors.
func exitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, context.Canceled) {
		return ExitInterrupted
	}
	if exitErr, ok := errors.AsType[*ExitError](err); ok {
		return exitErr.Code
	}
	return ExitUsage
}
