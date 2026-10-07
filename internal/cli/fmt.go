// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/syntax"
)

type fmtOptions struct {
	write bool
	check bool
}

func newFmtCmd() *cobra.Command {
	opts := &fmtOptions{}
	cmd := &cobra.Command{
		Use:   "fmt [FILE...]",
		Short: "Format request files canonically",
		Long: "Fmt prints files in canonical layout: whitespace normalized, sections in\n" +
			"canonical order ([Options] first), and a unit (ms) added to unitless\n" +
			"durations; bodies are untouched. With no FILE it reads standard input.\n" +
			"--write rewrites files in place. --check lists files that are not\n" +
			"formatted and exits with 1 (2 if a file cannot be read or parsed).",
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
	color := resolveColor(cmd, config.FromOSEnviron(), isTerminal(stdout), nil)
	if len(args) == 0 {
		args = []string{stdinName}
	}
	if opts.write && slices.Contains(args, stdinName) {
		writeErrorMessage(stderr, "Standard input can not be formatted in place", color)
		return silentExit(ExitParse)
	}

	var out bytes.Buffer
	invalid, writeFailed, unformatted := false, false, 0
	for _, name := range args {
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		in, c := readInputColor(stderr, name, color, true)
		if c != ExitOK {
			invalid = true
			continue
		}
		linted := syntax.Lint(in.file)
		switch {
		case opts.check:
			if !bytes.Equal(linted, bytes.TrimPrefix(in.src, []byte(utf8BOM))) {
				fmt.Fprintf(&out, "would reformat: %s\n", name)
				unformatted++
			}
		case opts.write:
			if bytes.Equal(linted, in.src) {
				continue
			}
			if err := writeAtomic(name, linted); err != nil {
				_, _ = fmt.Fprintf(stderr, "error: Issue writing to %s: %v\n", name, err)
				writeFailed = true
			}
		case color:
			// The linted text parses: Lint is checked to round-trip.
			if f, err := syntax.Parse(name, linted, syntax.DialectFor(name)); err == nil {
				out.Write(syntax.HighlightANSI(f))
			} else {
				out.Write(linted)
			}
		default:
			out.Write(linted)
		}
	}

	switch {
	case opts.write:
		if writeFailed {
			return silentExit(ExitUndefined)
		}
		if invalid {
			return silentExit(ExitParse)
		}
		return nil
	case opts.check:
		if !invalid && unformatted == 0 {
			return nil
		}
		if unformatted > 0 {
			fmt.Fprintf(&out, "%d file%s would be reformatted", unformatted, plural(unformatted))
		}
	}
	// Like the reference formatter, the output always ends with a newline.
	if !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
		out.WriteByte('\n')
	}
	if _, err := stdout.Write(out.Bytes()); err != nil {
		return NewExitError(ExitUndefined, err)
	}
	switch {
	case invalid:
		return silentExit(ExitParse)
	case opts.check:
		// The reference formatter exits 3 here; sonde keeps its documented
		// exit code (docs/stability.md).
		return silentExit(ExitUsage)
	}
	return nil
}

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
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
