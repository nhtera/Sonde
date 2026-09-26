// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package curl imports curl command lines into Sonde request files, and
// (see engine/curl_export.go, internal/cli/export.go) supports exporting
// entries back to curl. It never runs anything a command names (no
// network access, no reading a file a curl command references) and never
// panics on malformed input: unsupported flags become warnings, and a
// command with no URL is skipped rather than failing the whole import.
package curl

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// ErrNoCommand is returned by Import when data holds no `curl` command at
// all.
var ErrNoCommand = errors.New("curl: no curl command found in input")

// maxCommands bounds how many curl commands one input can import in a
// single call, so a script built out of an enormous number of trivial
// commands cannot make one Import call do unbounded work (see C1 in the
// phase 8 review). Real scripts are nowhere near this size; it exists only
// as a hard ceiling.
const maxCommands = 10000

// Result is one input's import: every command becomes one entry of File,
// in order.
type Result struct {
	File     *syntax.File
	Warnings []convert.Warning
	Skipped  []convert.Skipped
}

// Import parses data as a shell script (or a single command line) and
// converts every `curl` command it finds, in order, to one entry of the
// returned file. dialect controls the dialect of the generated files;
// see internal/convert.Options.Dialect.
func Import(data []byte, dialect syntax.Dialect) (Result, error) {
	data = stripPromptLines(data)
	toks, names, unevaluated, unevaluatedExtra := tokenize(data)
	cmds := commands(toks)
	if len(cmds) == 0 {
		return Result{}, ErrNoCommand
	}
	var truncatedCommands int
	if len(cmds) > maxCommands {
		truncatedCommands = len(cmds) - maxCommands
		cmds = cmds[:maxCommands]
	}

	var res Result
	for _, name := range names {
		res.Warnings = append(res.Warnings, convert.Warning{
			Kind:    convert.WarnUnsupported,
			Message: "shell variable $" + name + " became {{" + convert.VariableName(name) + "}}; set it with --variable or sonde.yaml",
		})
	}
	for _, snippet := range unevaluated {
		res.Warnings = append(res.Warnings, convert.Warning{
			Kind:    convert.WarnUnsupported,
			Message: "curl: shell expansion " + snippet + " is not evaluated; kept as literal text",
		})
	}
	if unevaluatedExtra > 0 {
		res.Warnings = append(res.Warnings, convert.Warning{
			Kind:    convert.WarnUnsupported,
			Message: fmt.Sprintf("curl: %d more shell expansions are not evaluated; kept as literal text", unevaluatedExtra),
		})
	}
	if truncatedCommands > 0 {
		res.Warnings = append(res.Warnings, convert.Warning{
			Kind:    convert.WarnUnsupported,
			Message: fmt.Sprintf("curl: input has more than %d curl commands; the remaining %d were not imported", maxCommands, truncatedCommands),
		})
	}

	marks := windowsCmdMarkers(data)
	var specs []syntax.EntrySpec
	for i, cmd := range cmds {
		label := "command " + strconv.Itoa(i+1)
		if cmd.isWindowsCmd(marks) {
			res.Skipped = append(res.Skipped, convert.Skipped{Name: label, Reason: windowsCmdReason})
			continue
		}
		spec, warnings, skipReason := buildEntry(parseArgv(cmd.argv))
		if skipReason != "" {
			res.Skipped = append(res.Skipped, convert.Skipped{Name: label, Reason: skipReason})
			continue
		}
		// Building this one entry on its own first isolates a malformed
		// command (one whose flags render to source BuildFile's own
		// parser rejects) from every other command in the input, instead
		// of failing the whole import.
		if _, err := syntax.BuildFile([]syntax.EntrySpec{spec}, dialect); err != nil {
			res.Skipped = append(res.Skipped, convert.Skipped{Name: label, Reason: err.Error()})
			continue
		}
		specs = append(specs, spec)
		res.Warnings = append(res.Warnings, warnings...)
	}
	if len(specs) == 0 {
		reasons := make([]string, len(res.Skipped))
		for i, s := range res.Skipped {
			reasons[i] = s.Name + ": " + s.Reason
		}
		return Result{}, fmt.Errorf("curl: no usable curl command in input (%s)", strings.Join(reasons, "; "))
	}

	f, err := syntax.BuildFile(specs, dialect)
	if err != nil {
		return Result{}, err
	}
	res.File = f
	return res, nil
}
