// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// referencePage is one generated docs/cli/*.md file.
type referencePage struct {
	name string // file name relative to docs/cli, e.g. "sonde_import_openapi.md"
	body string
}

// cliReferenceHeader marks every generated page as machine-written, like
// docs/compat.md.
const cliReferenceHeader = "<!-- Generated from the command tree by `make docs`. Do not edit. -->\n\n"

// renderCLIReference walks root's command tree and renders one markdown
// page per visible command plus an index (README.md), entirely from
// Use/Short/Long/Example and flags (pflag.FlagUsages): no dates, no
// machine-specific defaults (paths, environment) and a stable order, so the
// output is identical on every machine and every run.
func renderCLIReference(root *cobra.Command) []referencePage {
	var cmds []*cobra.Command
	collectCommands(root, &cmds)

	pages := make([]referencePage, 0, len(cmds)+1)
	for _, c := range cmds {
		pages = append(pages, referencePage{name: pageName(c), body: cliReferenceHeader + renderCommandPage(c)})
	}
	pages = append(pages, referencePage{name: "README.md", body: cliReferenceHeader + renderCLIIndex(root)})
	sort.Slice(pages, func(i, j int) bool { return pages[i].name < pages[j].name })
	return pages
}

// collectCommands appends c and every visible descendant, depth-first,
// children sorted by name at each level: the result order never depends on
// AddCommand call order, and hidden commands (none exist today) and
// cobra's own "help"/"completion" commands (not registered here, but
// excluded defensively) never appear.
func collectCommands(c *cobra.Command, out *[]*cobra.Command) {
	*out = append(*out, c)
	children := visibleChildren(c)
	for _, ch := range children {
		collectCommands(ch, out)
	}
}

// visibleChildren returns c's subcommands, excluding hidden and
// deprecated ones and cobra's automatic help/completion commands, sorted
// by name.
func visibleChildren(c *cobra.Command) []*cobra.Command {
	all := c.Commands()
	out := make([]*cobra.Command, 0, len(all))
	for _, ch := range all {
		if ch.Hidden || ch.Deprecated != "" || ch.Name() == "help" || ch.Name() == "completion" {
			continue
		}
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// pageName returns c's generated file name: its command path with spaces
// replaced by underscores, e.g. "sonde import openapi" ->
// "sonde_import_openapi.md".
func pageName(c *cobra.Command) string {
	return strings.ReplaceAll(c.CommandPath(), " ", "_") + ".md"
}

// renderCommandPage renders one command's page: title, usage line, help
// text, flags (its own, then those inherited from parents) and a list of
// subcommands, in that fixed order.
func renderCommandPage(c *cobra.Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", c.CommandPath())
	if c.Short != "" {
		fmt.Fprintf(&b, "%s.\n\n", strings.TrimSuffix(c.Short, "."))
	}
	fmt.Fprintf(&b, "```\n%s\n```\n\n", c.UseLine())
	if long := strings.TrimSpace(c.Long); long != "" && long != strings.TrimSpace(c.Short) {
		fmt.Fprintf(&b, "%s\n\n", long)
	}
	if ex := strings.TrimSpace(c.Example); ex != "" {
		fmt.Fprintf(&b, "## Examples\n\n```\n%s\n```\n\n", ex)
	}
	if flags := c.NonInheritedFlags(); flags.HasAvailableFlags() {
		fmt.Fprintf(&b, "## Flags\n\n```\n%s```\n\n", flags.FlagUsages())
	}
	if flags := c.InheritedFlags(); flags.HasAvailableFlags() {
		fmt.Fprintf(&b, "## Global flags\n\n```\n%s```\n\n", flags.FlagUsages())
	}
	if children := visibleChildren(c); len(children) > 0 {
		b.WriteString("## Subcommands\n\n")
		for _, ch := range children {
			fmt.Fprintf(&b, "- [%s](%s) — %s\n", ch.CommandPath(), pageName(ch), ch.Short)
		}
		b.WriteString("\n")
	}
	if c.HasParent() {
		fmt.Fprintf(&b, "Parent command: [%s](%s)\n", c.Parent().CommandPath(), pageName(c.Parent()))
	}
	return b.String()
}

// renderCLIIndex renders docs/cli/README.md: every command, indented by
// depth, linking to its page.
func renderCLIIndex(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("# CLI reference\n\n")
	b.WriteString("Every `sonde` command, generated from its cobra definition " +
		"(`internal/cli/cli_reference.go`; `go test ./internal/cli -run TestCLIReferenceUpToDate -update`, " +
		"wired into `make docs`).\n\n")
	writeIndexEntry(&b, root, 0)
	return b.String()
}

func writeIndexEntry(b *strings.Builder, c *cobra.Command, depth int) {
	fmt.Fprintf(b, "%s- [%s](%s) — %s\n", strings.Repeat("  ", depth), c.CommandPath(), pageName(c), c.Short)
	for _, ch := range visibleChildren(c) {
		writeIndexEntry(b, ch, depth+1)
	}
}

// newReferenceRootCmd builds the full command tree exactly as Execute
// does, but with discarded output: the generator only inspects Use, Short,
// Long, Example and flags, never runs a command.
func newReferenceRootCmd() *cobra.Command {
	return newRootCmd(io.Discard, io.Discard)
}
