// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/mcp"
	"github.com/nhtera/sonde/internal/netpolicy"
)

type mcpOptions struct {
	root           string
	allowRun       bool
	allowHosts     []string
	env            string
	variables      []string
	variablesFiles []string
	secrets        []string
	secretsFiles   []string
	runTimeout     time.Duration
}

func newMCPCmd() *cobra.Command {
	o := &mcpOptions{}
	cmd := &cobra.Command{
		Use:   "mcp [options]",
		Short: "Serve request files to AI agents over MCP",
		Long: "Mcp runs a Model Context Protocol server on stdin/stdout for AI agents\n" +
			"(Claude Code, Cursor, VS Code...). Its tools list the request files under\n" +
			"--root (sonde_list) and check them (sonde_check). With --allow-run, a\n" +
			"third tool runs one file (sonde_run), contacting only the hosts given\n" +
			"with --allow-host; request files cannot read outside the root, and\n" +
			"secrets are redacted from every result. It logs one line per tool call\n" +
			"to stderr. See docs/guides/mcp.md.",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCP(cmd, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.root, "root", ".", "the directory the tools can see (request files cannot read outside it)")
	f.BoolVar(&o.allowRun, "allow-run", false, "add the sonde_run tool (requires --allow-host)")
	f.StringArrayVar(&o.allowHosts, "allow-host", nil, "a host sonde_run may contact: `HOST`, HOST:PORT, *.DOMAIN, an IP, a CIDR range, or * for any (repeatable)")
	f.StringVar(&o.env, "env", "", "the sonde.yaml environment of runs that name none")
	f.StringArrayVar(&o.variables, "variable", nil, "defines a variable for every run (`NAME=VALUE`, repeatable)")
	f.StringArrayVar(&o.variablesFiles, "variables-file", nil, "sets variables for every run from a properties `FILE` (repeatable)")
	f.StringArrayVar(&o.secrets, "secret", nil, "defines a secret for every run (`NAME=VALUE`, repeatable)")
	f.StringArrayVar(&o.secretsFiles, "secrets-file", nil, "sets secrets for every run from a properties `FILE` (repeatable)")
	f.DurationVar(&o.runTimeout, "run-timeout", mcp.DefaultRunTimeout, "the longest a run may take (1s to 10m)")
	return cmd
}

// runMCP serves until the client closes stdin or Ctrl-C. Every flag
// problem is a usage error, reported before the server starts.
func runMCP(cmd *cobra.Command, o *mcpOptions) error {
	stderr := cmd.ErrOrStderr()
	if o.allowRun && len(o.allowHosts) == 0 {
		return NewExitError(ExitUsage, errors.New("--allow-run requires at least one --allow-host (--allow-host '*' allows every host)"))
	}
	if !o.allowRun && len(o.allowHosts) > 0 {
		return NewExitError(ExitUsage, errors.New("--allow-host has no effect without --allow-run"))
	}
	if o.runTimeout < time.Second || o.runTimeout > mcp.MaxRunTimeout {
		return invalidFlagErr("run-timeout", "DURATION", o.runTimeout.String(), fmt.Errorf("must be between 1s and %s", mcp.MaxRunTimeout))
	}
	var hosts *netpolicy.Policy
	if o.allowRun {
		var err error
		if hosts, err = netpolicy.Parse(o.allowHosts); err != nil {
			return NewExitError(ExitUsage, fmt.Errorf("--allow-host: %w", err))
		}
	}
	root, err := mcpRoot(o.root)
	if err != nil {
		return NewExitError(ExitUsage, fmt.Errorf("--root: %w", err))
	}

	env := config.FromOSEnviron()
	variables, err := config.BuildVariables(env, o.variablesFiles, o.variables)
	if err != nil {
		return NewExitError(ExitUsage, err)
	}
	secrets, err := config.BuildSecrets(env, o.secretsFiles, o.secrets)
	if err != nil {
		return NewExitError(ExitUsage, err)
	}
	if err := config.CheckNoClash(variables, secrets); err != nil {
		return NewExitError(ExitUsage, err)
	}
	// The files of the flags above are read from the directory the server
	// started in. From here on the root is the working directory: relative
	// paths of request-file options (cacert, cert, key) are read from it,
	// as curl reads them.
	if err := os.Chdir(root); err != nil {
		return NewExitError(ExitUsage, fmt.Errorf("--root: %w", err))
	}
	vars := make(map[string]any, len(variables))
	for name, v := range variables {
		vars[name] = v
	}

	cfg := mcp.Config{
		Root:       root,
		AllowRun:   o.allowRun,
		Hosts:      hosts,
		Env:        o.env,
		EnvVar:     env["SONDE_ENV"],
		Variables:  vars,
		Secrets:    secrets,
		RunTimeout: o.runTimeout,
		Version:    currentBuildInfo().Version,
		Log:        stderr,
	}
	run := "off"
	if o.allowRun {
		run = "hosts " + strings.Join(o.allowHosts, ", ")
		if hosts.AllowsAll() {
			_, _ = fmt.Fprintln(stderr, "sonde mcp: warning: --allow-host '*' lets sonde_run contact any host")
		}
	}
	_, _ = fmt.Fprintf(stderr, "sonde mcp: serving %s over stdio (run: %s)\n", root, run)

	// The first Ctrl-C stops the server (exit 130), as it stops `sonde
	// mock`: there is no entry boundary to wait for.
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	go func() {
		select {
		case <-stopFromContext(cmd.Context()):
			cancel()
		case <-ctx.Done():
		}
	}()
	// Standard output carries the protocol and nothing else.
	t := &sdk.IOTransport{Reader: io.NopCloser(cmd.InOrStdin()), Writer: nopWriteCloser{cmd.OutOrStdout()}}
	if err := mcp.Serve(ctx, cfg, t); err != nil && ctx.Err() == nil && !errors.Is(err, io.EOF) {
		return NewExitError(ExitRuntime, err)
	}
	return nil
}

// mcpRoot returns the absolute root directory, symbolic links resolved.
func mcpRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s: not a directory", dir)
	}
	return abs, nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
