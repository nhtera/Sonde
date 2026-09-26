// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"fmt"
	"io"
	"sort"
)

// Warning is a non-fatal note about the conversion, grouped by Kind in the
// summary (e.g. "unsupported-auth", "renamed-file").
type Warning struct {
	Kind    string
	Message string
}

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
			fmt.Fprintf(w, "  %s: %s\n", s.Name, s.Reason)
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
				fmt.Fprintf(w, "    %s\n", m)
			}
		}
	}
}
