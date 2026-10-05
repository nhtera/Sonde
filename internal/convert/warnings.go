// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Warning is a non-fatal note about the conversion, grouped by Kind in the
// summary. Importers use the Warn* kinds below where one fits, so users can
// grep the same kind across formats.
type Warning struct {
	Kind    string
	Message string
}

// Warning kinds shared by the importers.
const (
	// WarnUnsupportedAuth: an auth scheme with no Sonde equivalent.
	WarnUnsupportedAuth = "unsupported-auth"
	// WarnUnsupportedBody: a body left out or reduced.
	WarnUnsupportedBody = "unsupported-body"
	// WarnUnsupportedOption: an option or flag with no Sonde equivalent.
	WarnUnsupportedOption = "unsupported-option"
	// WarnScript: a script kept as a comment, never run.
	WarnScript = "script"
	// WarnDynamicVariable: a dynamic variable with no Sonde function.
	WarnDynamicVariable = "dynamic-variable"
	// WarnSecret: a secret value left out of the output.
	WarnSecret = "secret"
	// WarnUnsupported: any other item that could not be converted
	// faithfully.
	WarnUnsupported = "unsupported"
)

// Skipped is an input item an importer chose not to convert, and why.
type Skipped struct {
	Name   string
	Reason string
}

// Summarize writes a deterministic, human-readable summary of a Write to w:
// files written (or planned, for a dry run), the number of requests,
// skipped items with their reasons, and warnings grouped by kind. Callers
// typically pass cmd.ErrOrStderr().
func Summarize(w io.Writer, out Output, res *Result) {
	verb := "wrote"
	if res.DryRun {
		verb = "would write"
	}
	fmt.Fprintf(w, "%s %d file(s), %d request(s)\n", verb, len(res.Files), res.RequestCount)
	for _, f := range res.Files {
		fmt.Fprintf(w, "  %s\n", f)
	}
	for _, f := range res.Extra {
		fmt.Fprintf(w, "%s %s\n", verb, f)
	}
	for _, f := range res.ExtraKept {
		fmt.Fprintf(w, "%s: already exists, left untouched\n", f)
	}
	for _, f := range res.Extra {
		if strings.HasSuffix(f, ".secrets") {
			fmt.Fprintf(w, "keep secret values out of git: add *.secrets to .gitignore\n")
			break
		}
	}
	switch {
	case res.Project != "":
		fmt.Fprintf(w, "%s %s\n", verb, res.Project)
	case res.ProjectSkipped:
		fmt.Fprintf(w, "%s: already exists, left untouched\n", ProjectFileName)
	}

	if len(out.Skipped) > 0 {
		fmt.Fprintf(w, "skipped %d item(s):\n", len(out.Skipped))
		skipped := append([]Skipped(nil), out.Skipped...)
		sort.Slice(skipped, func(i, j int) bool { return skipped[i].Name < skipped[j].Name })
		for _, s := range skipped {
			fmt.Fprintf(w, "  %s: %s\n", printable(s.Name), printable(s.Reason))
		}
	}

	if len(out.Warnings) > 0 {
		byKind := map[string][]string{}
		for _, wn := range out.Warnings {
			byKind[wn.Kind] = append(byKind[wn.Kind], wn.Message)
		}
		kinds := make([]string, 0, len(byKind))
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		fmt.Fprintf(w, "%d warning(s):\n", len(out.Warnings))
		for _, k := range kinds {
			msgs := byKind[k]
			sort.Strings(msgs)
			fmt.Fprintf(w, "  %s:\n", k)
			for _, m := range msgs {
				fmt.Fprintf(w, "    %s\n", printable(m))
			}
		}
	}
}

// printable quotes the control characters of s, which may come from the
// input, so they can't drive the terminal.
func printable(s string) string {
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	q := strconv.Quote(s)
	return q[1 : len(q)-1]
}
