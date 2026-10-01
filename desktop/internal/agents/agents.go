// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package agents writes the configuration that lets an AI agent (Claude
// Code, Cursor, VS Code) start `sonde mcp` on the open project: the
// snippet to paste, where it goes, and the tools the agent then has.
// Hosts are checked as `sonde mcp --allow-host` checks them; `*` is
// refused in a configuration shared through the repository; --root is
// always the project's own folder.
package agents

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/internal/mcp"
	"github.com/nhtera/sonde/internal/netpolicy"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Clients and scopes.
const (
	Claude = "claude"
	Cursor = "cursor"
	VSCode = "vscode"

	Project = "project" // shared through the repository
	User    = "user"    // this machine only
)

// Request asks for a snippet.
type Request struct {
	Client   string   `json:"client"`
	Scope    string   `json:"scope"`
	AllowRun bool     `json:"allowRun"`
	Hosts    []string `json:"hosts"`
}

// Tool is a tool the agent gets.
type Tool struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Snippet is a configuration to copy.
type Snippet struct {
	// Text is the snippet: a command (Claude Code, user scope) or JSON.
	Text string `json:"text"`
	// Where says where it goes ("run in the project folder", ".mcp.json").
	Where string `json:"where"`
	// Args are `sonde`'s arguments, for display.
	Args  []string `json:"args"`
	Tools []Tool   `json:"tools"`
}

// Agents builds snippets for the open project.
type Agents struct {
	project func() *sandbox.Root
}

// New returns the agents service's core.
func New(project func() *sandbox.Root) *Agents { return &Agents{project: project} }

// Snippet builds the configuration of req.
func (a *Agents) Snippet(req Request) (*Snippet, error) {
	root := a.project()
	if root == nil {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	if req.Scope != Project && req.Scope != User {
		return nil, apperr.New(apperr.Invalid, "unknown scope "+req.Scope)
	}
	args := []string{"mcp", "--root", rootArg(req, root.Dir())}
	if req.AllowRun {
		var hosts []string
		for _, h := range req.Hosts {
			if h = strings.TrimSpace(h); h != "" {
				hosts = append(hosts, h)
			}
		}
		if len(hosts) == 0 {
			return nil, apperr.New(apperr.Invalid, "sending requests needs at least one allowed host")
		}
		for _, h := range hosts {
			if h == "*" && req.Scope == Project {
				return nil, apperr.New(apperr.Invalid, "every host (*) is refused in a configuration shared through the repository: list the hosts")
			}
		}
		if _, err := netpolicy.Parse(hosts); err != nil {
			return nil, apperr.New(apperr.Invalid, "allowed hosts: "+err.Error())
		}
		args = append(args, "--allow-run")
		for _, h := range hosts {
			args = append(args, "--allow-host", h)
		}
	}
	s := &Snippet{Args: args, Tools: tools(req.AllowRun)}
	server := map[string]any{"command": "sonde", "args": args}
	switch req.Client {
	case Claude:
		if req.Scope == User {
			// User scope: every project of this machine, not only the
			// folder the command runs in (the default local scope).
			s.Text, s.Where = "claude mcp add --scope user sonde -- sonde "+shellJoin(args), "Run it in a terminal"
			return s, nil
		}
		server["type"] = "stdio"
		s.Text, s.Where = jsonText(map[string]any{"mcpServers": map[string]any{"sonde": server}}), ".mcp.json in the project folder"
	case Cursor:
		s.Text = jsonText(map[string]any{"mcpServers": map[string]any{"sonde": server}})
		s.Where = ".cursor/mcp.json in the project folder"
		if req.Scope == User {
			s.Where = "~/.cursor/mcp.json"
		}
	case VSCode:
		server["type"] = "stdio"
		s.Text = jsonText(map[string]any{"servers": map[string]any{"sonde": server}})
		s.Where = ".vscode/mcp.json in the project folder"
		if req.Scope == User {
			s.Where = "the user configuration (MCP: Open User Configuration)"
		}
	default:
		return nil, apperr.New(apperr.Invalid, "unknown client "+req.Client)
	}
	return s, nil
}

// rootArg is the --root of req's configuration, always set: the project
// folder's path on this machine for a user's own configuration; for one
// shared through the repository, the folder as each client names its
// workspace (a path on disk would show this machine's and fail on
// another's): Claude starts a project's servers in its folder.
func rootArg(req Request, dir string) string {
	switch {
	case req.Scope == User:
		return dir
	case req.Client == Claude:
		return "."
	default:
		return "${workspaceFolder}"
	}
}

// tools are the tools `sonde mcp` gives, as it describes them.
func tools(allowRun bool) []Tool {
	var out []Tool
	for _, t := range mcp.Tools(allowRun) {
		out = append(out, Tool{Name: t.Name, Title: t.Title, Description: t.Description})
	}
	return out
}

// jsonText is v as indented JSON, without HTML escaping.
func jsonText(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return b.String()
}

// unsafeInShell reports whether r needs quoting in a POSIX shell.
func unsafeInShell(r rune) bool {
	safe := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=@,+", r)
	return !safe
}

// shellJoin quotes args for a POSIX shell.
func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a != "" && strings.IndexFunc(a, unsafeInShell) < 0 {
			out[i] = a
			continue
		}
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}

// Service is the agents bindings.
type Service struct{ a *Agents }

// NewService returns the bindings over a.
func NewService(a *Agents) *Service { return &Service{a: a} }

// Snippet builds a configuration for an agent.
func (s *Service) Snippet(req Request) (*Snippet, error) { return s.a.Snippet(req) }
