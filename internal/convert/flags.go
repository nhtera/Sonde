// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Options are the flags every importer command shares.
type Options struct {
	// Output is the target directory (-o/--output); required.
	Output string
	// Ext is the extension generated files get, without the dot: "hurl"
	// (default) or "sonde".
	Ext string
	// Force allows overwriting files that already exist in Output. It
	// never applies to an existing sonde.yaml, which Write always leaves
	// untouched.
	Force bool
	// DryRun prints the plan and writes nothing.
	DryRun bool
}

// extension validates and returns Ext, defaulting to "hurl".
func (o Options) extension() (string, error) {
	switch o.Ext {
	case "", "hurl":
		return "hurl", nil
	case "sonde":
		return "sonde", nil
	default:
		return "", fmt.Errorf("convert: --ext must be \"hurl\" or \"sonde\", got %q", o.Ext)
	}
}

// RegisterFlags adds the shared import flags to cmd, bound to o.
func RegisterFlags(cmd *cobra.Command, o *Options) {
	flags := cmd.Flags()
	flags.StringVarP(&o.Output, "output", "o", "", "output directory (required)")
	flags.StringVar(&o.Ext, "ext", "hurl", `generated file extension: "hurl" or "sonde"`)
	flags.BoolVar(&o.Force, "force", false, "overwrite existing files in the output directory")
	flags.BoolVar(&o.DryRun, "dry-run", false, "print the planned files and warnings; write nothing")
	_ = cmd.MarkFlagRequired("output")
}
