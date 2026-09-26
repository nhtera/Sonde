// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
)

// importKind is one `sonde import <kind> INPUT` implementation. This file
// defines no real kind and only the dispatcher; each importer (openapi,
// curl, Postman collection, OpenCollection, .http) registers its own with
// registerImportKind, typically from an init function in its own file.
type importKind struct {
	// Name is the kind as typed on the command line, e.g. "openapi".
	Name string
	// Short is the one-line help shown for this kind's subcommand.
	Short string
	// RegisterFlags adds this kind's own flags to cmd, in addition to the
	// shared convert.Options flags newImportCmd already registers.
	RegisterFlags func(cmd *cobra.Command)
	// Run converts input (the command's INPUT argument) to a
	// convert.Output. Any error it returns is a usage error (unreadable
	// input, or a bad flag value only this kind understands).
	Run func(cmd *cobra.Command, input string) (convert.Output, error)
}

// importKinds is the registry registerImportKind fills; newImportCmd reads
// it once, when the root command is built.
var importKinds = map[string]importKind{}

// registerImportKind adds k to the kinds `sonde import` accepts. It panics
// on a duplicate name, which is a programming error (two importers sharing
// a name), never user input.
func registerImportKind(k importKind) {
	if _, dup := importKinds[k.Name]; dup {
		panic("cli: import kind " + k.Name + " already registered")
	}
	importKinds[k.Name] = k
}

// importKindNames returns every registered kind name, sorted.
func importKindNames() []string {
	names := make([]string, 0, len(importKinds))
	for name := range importKinds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// newImportCmd builds the `sonde import` command tree: one subcommand per
// registered kind (sorted by name), plus a fallback that turns an
// unregistered or missing kind into a usage error listing the kinds that do
// exist.
func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import KIND INPUT",
		Short: "Import request files from another format",
		Long: "Import converts INPUT, in KIND's format, to Sonde request files under\n" +
			"--output (see docs/guides/import-export.md).\n\n" +
			"Available kinds: " + kindList() + ".",
		Args: cobra.ArbitraryArgs,
		RunE: typed(func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return NewExitError(ExitUsage, fmt.Errorf("missing import kind; available kinds: %s", kindList()))
			}
			return NewExitError(ExitUsage, fmt.Errorf("unknown import kind %q; available kinds: %s", args[0], kindList()))
		}),
	}
	for _, name := range importKindNames() {
		cmd.AddCommand(newImportKindCmd(importKinds[name]))
	}
	return cmd
}

func kindList() string {
	names := importKindNames()
	if len(names) == 0 {
		return "(none registered)"
	}
	return strings.Join(names, ", ")
}

// newImportKindCmd builds the INPUT-taking subcommand for one registered
// kind: the shared convert.Options flags, then the kind's own.
func newImportKindCmd(k importKind) *cobra.Command {
	opts := &convert.Options{}
	cmd := &cobra.Command{
		Use:   k.Name + " INPUT",
		Short: k.Short,
		Args:  cobra.ExactArgs(1),
		RunE: typed(func(cmd *cobra.Command, args []string) error {
			return runImportKind(cmd, k, args[0], opts)
		}),
	}
	convert.RegisterFlags(cmd, opts)
	if k.RegisterFlags != nil {
		k.RegisterFlags(cmd)
	}
	return cmd
}

// runImportKind runs one import: k.Run builds the output, convert.Write
// places it under opts.Output, and a summary goes to stderr. Exit codes
// (docs/guides/import-export.md): 0 on success, including with warnings; 1
// (ExitUsage) for an unreadable input or a bad flag, or for a write
// conflict without --force.
func runImportKind(cmd *cobra.Command, k importKind, input string, opts *convert.Options) error {
	out, err := k.Run(cmd, input)
	if err != nil {
		return NewExitError(ExitUsage, err)
	}
	res, err := convert.Write(opts.Output, out, *opts)
	if err != nil {
		var conflicts *convert.ErrConflicts
		if errors.As(err, &conflicts) {
			return NewExitError(ExitUsage, err)
		}
		return NewExitError(ExitUndefined, err)
	}
	convert.Summarize(cmd.ErrOrStderr(), out, res)
	return nil
}
