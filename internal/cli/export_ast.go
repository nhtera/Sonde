// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/syntaxexport"
)

// exportASTOptions are the flags of `sonde export json` and `export html`.
type exportASTOptions struct {
	output     string
	standalone bool
}

func newExportJSONCmd() *cobra.Command {
	o := &exportASTOptions{}
	cmd := &cobra.Command{
		Use:   "json [FILE...]",
		Short: "Print the syntax tree of .hurl files as JSON",
		Long: "Json prints the syntax tree of each .hurl file as one JSON document, in the\n" +
			"reference formatter's shape. With no FILE it reads standard input.",
		RunE: typed(func(cmd *cobra.Command, args []string) error {
			return runExportAST(cmd, o, args, func(f *syntax.File) string { return syntaxexport.JSON(f) })
		}),
	}
	cmd.Flags().StringVarP(&o.output, "output", "o", "", "write to FILE instead of stdout")
	return cmd
}

func newExportHTMLCmd() *cobra.Command {
	o := &exportASTOptions{}
	cmd := &cobra.Command{
		Use:   "html [FILE...]",
		Short: "Print .hurl files as syntax-highlighted HTML",
		Long: "Html prints each .hurl file as a highlighted <pre> block, or with\n" +
			"--standalone as a complete HTML document with its stylesheet. With no\n" +
			"FILE it reads standard input.",
		RunE: typed(func(cmd *cobra.Command, args []string) error {
			return runExportAST(cmd, o, args, func(f *syntax.File) string { return syntaxexport.HTML(f, o.standalone) })
		}),
	}
	cmd.Flags().StringVarP(&o.output, "output", "o", "", "write to FILE instead of stdout")
	cmd.Flags().BoolVar(&o.standalone, "standalone", false, "write a complete HTML document with its stylesheet")
	return cmd
}

// runExportAST renders every input with render and writes the outputs,
// joined, ending with a newline. A file that cannot be read or parsed is
// reported and skipped; the exit code is then 2.
func runExportAST(cmd *cobra.Command, o *exportASTOptions, args []string, render func(*syntax.File) string) error {
	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	color := resolveColor(cmd, config.FromOSEnviron(), isTerminal(stdout))
	if len(args) == 0 {
		args = []string{stdinName}
	}
	for _, name := range args {
		if syntax.DialectFor(name) == syntax.DialectSonde {
			return NewExitError(ExitUsage, errors.New("export "+cmd.Name()+" supports .hurl files only"))
		}
	}

	var out strings.Builder
	invalid := false
	for _, name := range args {
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		in, c := readInputColor(stderr, name, color, true)
		if c != ExitOK {
			invalid = true
			continue
		}
		out.WriteString(render(in.file))
	}
	data := []byte(out.String())
	if !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	if o.output != "" {
		if err := os.WriteFile(o.output, data, 0o644); err != nil { //nolint:gosec // G306: a user-requested output file.
			return NewExitError(ExitUndefined, fmt.Errorf("Issue writing to %s: %w", o.output, err)) //nolint:staticcheck,revive // kept for CLI message-format compatibility
		}
	} else if _, err := stdout.Write(data); err != nil {
		return NewExitError(ExitUndefined, err)
	}
	if invalid {
		return silentExit(ExitParse)
	}
	return nil
}
