// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/syntax"
)

type fmtOptions struct {
	write bool
	check bool
}

func newFmtCmd() *cobra.Command {
	opts := &fmtOptions{}
	cmd := &cobra.Command{
		Use:   "fmt FILE...",
		Short: "Format request files canonically",
		Long: "Fmt prints files in canonical layout (whitespace only; nothing is reordered\n" +
			"and bodies are untouched). --write rewrites files in place, --check lists\n" +
			"files that are not formatted and exits with 1.",
		Args: cobra.MinimumNArgs(1),
		RunE: typed(func(cmd *cobra.Command, args []string) error {
			return runFmt(cmd, opts, args)
		}),
	}
	cmd.Flags().BoolVarP(&opts.write, "write", "w", false, "write the result to the files instead of stdout")
	cmd.Flags().BoolVar(&opts.check, "check", false, "list files that are not formatted; exit 1 if any")
	cmd.MarkFlagsMutuallyExclusive("write", "check")
	return cmd
}

func runFmt(cmd *cobra.Command, opts *fmtOptions, args []string) error {
	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	code := ExitOK
	for _, name := range args {
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		in, c := readInput(stderr, name)
		if c != ExitOK {
			code = max(code, c)
			continue
		}
		out := syntax.Format(in.file)
		switch {
		case opts.check:
			if !bytes.Equal(out, in.src) {
				_, _ = fmt.Fprintln(stdout, name)
				code = max(code, ExitUsage)
			}
		case opts.write:
			if bytes.Equal(out, in.src) {
				continue
			}
			if err := writeAtomic(name, out); err != nil {
				_, _ = fmt.Fprintf(stderr, "error: Issue writing to %s: %v\n", name, err)
				code = max(code, ExitUndefined)
			}
		default:
			if _, err := stdout.Write(out); err != nil {
				return NewExitError(ExitUndefined, err)
			}
		}
	}
	if code != ExitOK {
		return silentExit(code)
	}
	return nil
}

// writeAtomic replaces name with data via a synced temporary file in the
// same directory, keeping the file mode, so readers never see a partial
// file. A symlink is followed and its target rewritten. Hard links, owner
// and extended attributes are not preserved.
func writeAtomic(name string, data []byte) (err error) {
	if name, err = filepath.EvalSymlinks(name); err != nil {
		return err
	}
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
