// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
)

// exportCurlOptions are `sonde export curl`'s own flags. Variables and
// secrets follow the same sources and precedence as a run
// (config.BuildVariables/BuildSecrets, then a matching sonde.yaml
// environment per input file), scoped down to what an export — which
// never sends anything and has no data rows, retries or run-level
// transport overrides of its own — actually needs; an entry's own
// [Options] (insecure, proxy, ...) still render, since those come from
// the file itself, not from a flag here.
type exportCurlOptions struct {
	entry          int
	variables      []string
	variablesFiles []string
	secrets        []string
	secretsFiles   []string
	env            string
	config         string
	fileRoot       string
}

// newExportCmd builds the `sonde export` command tree: one subcommand per
// supported target format (only "curl" for now).
func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export request files to another format",
	}
	cmd.AddCommand(newExportCurlCmd())
	return cmd
}

func newExportCurlCmd() *cobra.Command {
	o := &exportCurlOptions{}
	cmd := &cobra.Command{
		Use:   "curl FILE...",
		Short: "Print each entry's equivalent curl command line, without sending it",
		Args:  cobra.MinimumNArgs(1),
		RunE:  typed(func(cmd *cobra.Command, args []string) error { return runExportCurl(cmd, o, args) }),
	}
	f := cmd.Flags()
	f.IntVar(&o.entry, "entry", 0, "exports only the given entry number (1-based); 0 exports every entry")
	f.StringArrayVar(&o.variables, "variable", nil, "defines a variable")
	f.StringArrayVar(&o.variablesFiles, "variables-file", nil, "defines variables from a properties file")
	f.StringArrayVar(&o.secrets, "secret", nil, "defines a variable whose value is treated as a secret")
	f.StringArrayVar(&o.secretsFiles, "secrets-file", nil, "defines secrets from a file")
	f.StringVar(&o.env, "env", "", "selects a sonde.yaml environment by name")
	f.StringVar(&o.config, "config", "", "uses this sonde.yaml for every input file, skipping discovery")
	f.StringVar(&o.fileRoot, "file-root", "", "sets the root directory used to resolve file paths")
	return cmd
}

// runExportCurl exports every FILE in turn: each entry's curl command
// line goes to stdout, one per line; an unresolved variable warns on
// stderr instead of failing that entry (see engine.Runner.RenderCurl). A
// problem with one FILE (parse error, out-of-range --entry, a render
// error) is reported and that file contributes nothing further, but
// every other FILE is still attempted; the command's own exit code
// reflects the worst problem across all of them.
func runExportCurl(cmd *cobra.Command, o *exportCurlOptions, files []string) error {
	if o.entry < 0 {
		return NewExitError(ExitUsage, fmt.Errorf("--entry must not be negative, got %d", o.entry))
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
	cliVars := make(map[string]any, len(variables))
	for name, v := range variables {
		cliVars[name] = v
	}

	base := engine.Options{DefaultUserAgent: defaultUserAgent(env)}
	if o.entry > 0 {
		base.FromEntry, base.ToEntry = o.entry, o.entry
	}

	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	cache := config.NewProjectCache()
	loaded := map[string]*config.Project{}

	worst := ExitOK
	for _, file := range files {
		if err := exportCurlFile(cmd.Context(), stdout, stderr, file, o, env, base, cliVars, secrets, cache, loaded); err != nil {
			if !isSilent(err) {
				fmt.Fprintf(stderr, "error: %v\n", err)
			}
			if code := exitCode(err); code > worst {
				worst = code
			}
		}
	}
	if worst != ExitOK {
		return silentExit(worst)
	}
	return nil
}

// exportCurlFile exports one FILE: it is read and parsed like a run
// would, its sonde.yaml environment (if any) resolved the same way and
// merged under the command line's own variables/secrets (which win on a
// name collision, exactly like a run's job tier loses to its runner
// tier), then every selected entry's curl command is written to stdout.
func exportCurlFile(ctx context.Context, stdout, stderr io.Writer, file string, o *exportCurlOptions, env config.Env, base engine.Options, cliVars map[string]any, cliSecrets map[string]string, cache *config.ProjectCache, loaded map[string]*config.Project) error {
	pf, code := readInput(stderr, file)
	if code != ExitOK {
		return silentExit(code)
	}
	if o.entry > len(pf.file.Entries) {
		return NewExitError(ExitUsage, fmt.Errorf("%s: entry %d not found (file has %d)", file, o.entry, len(pf.file.Entries)))
	}
	projectVars, projectSecrets, err := resolveProjectEnv(cache, loaded, o, env, file)
	if err != nil {
		return NewExitError(ExitUsage, err)
	}

	opt := base
	opt.Variables = mergeVariables(projectVars, cliVars)
	opt.Secrets = mergeSecrets(projectSecrets, cliSecrets)
	opt.FileRoot = o.fileRoot
	if opt.FileRoot == "" {
		opt.FileRoot = filepath.Dir(file)
	}

	runner := engine.NewRunner(opt)
	defer func() { _ = runner.Close() }()
	entries, err := runner.RenderCurl(ctx, file, pf.src)
	if err != nil {
		return NewExitError(ExitRuntime, fmt.Errorf("%s: %w", file, err))
	}
	failed := false
	for _, e := range entries {
		for _, name := range e.Undefined {
			fmt.Fprintf(stderr, "warning: %s: entry %d: {{%s}} is undefined; kept as a literal placeholder\n", file, e.Index, name)
		}
		if e.Err != nil {
			fmt.Fprintf(stderr, "error: %s: entry %d: %v\n", file, e.Index, e.Err)
			failed = true
			continue
		}
		fmt.Fprintln(stdout, e.Command)
	}
	if failed {
		return silentExit(ExitRuntime)
	}
	return nil
}

// mergeVariables merges project under cli, cli winning on a name
// collision (a run's own job/runner tiers apply the same precedence).
func mergeVariables(project, cli map[string]any) map[string]any {
	if len(project) == 0 {
		return cli
	}
	merged := make(map[string]any, len(project)+len(cli))
	for name, v := range project {
		merged[name] = v
	}
	for name, v := range cli {
		merged[name] = v
	}
	return merged
}

// mergeSecrets is mergeVariables for secrets.
func mergeSecrets(project, cli map[string]string) map[string]string {
	if len(project) == 0 {
		return cli
	}
	merged := make(map[string]string, len(project)+len(cli))
	for name, v := range project {
		merged[name] = v
	}
	for name, v := range cli {
		merged[name] = v
	}
	return merged
}

// resolveProjectEnv resolves file's sonde.yaml environment (--config,
// else discovery from file's directory), exactly as a run's own
// resolveJobExtras (internal/cli/run.go) does for one file, but without
// the run-only concerns (defaults.jobs, a data row, a contract
// validator) or its stdin ("-") handling, which export's own FILE
// arguments never need. Kept as its own function, deliberately close to
// resolveJobExtras's per-file body, rather than factored to share code
// with it: run.go belongs to a different phase of this same effort, and
// duplicating this one small piece is safer than reaching into a file
// this package doesn't own.
func resolveProjectEnv(cache *config.ProjectCache, loaded map[string]*config.Project, o *exportCurlOptions, env config.Env, file string) (map[string]any, map[string]string, error) {
	path := o.config
	if path == "" {
		found, ok, err := cache.FindProject(filepath.Dir(file))
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			if o.env != "" {
				return nil, nil, fmt.Errorf("%s: no sonde.yaml found for environment %q", file, o.env)
			}
			return nil, nil, nil
		}
		path = found
	}
	proj, ok := loaded[path]
	if !ok {
		var err error
		if proj, err = config.LoadProject(path); err != nil {
			return nil, nil, err
		}
		loaded[path] = proj
	}
	envName := config.SelectEnv(o.env, env["SONDE_ENV"], proj.Defaults.Env)
	vars, secrets, err := proj.Resolve(envName)
	if err != nil {
		return nil, nil, err
	}
	if len(vars) == 0 {
		return nil, secrets, nil
	}
	out := make(map[string]any, len(vars))
	for name, v := range vars {
		out[name] = v
	}
	return out, secrets, nil
}
