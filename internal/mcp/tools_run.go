// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/openapi"
)

// Bounds of the variables of one call.
const (
	maxVariables     = 64
	maxVariableBytes = 4 << 10
)

// maxResponseBody caps each response of a run (--max-filesize), so a run
// cannot hold unbounded bodies in memory.
const maxResponseBody = 16 << 20

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

type runInput struct {
	Path      string         `json:"path" jsonschema:"the request file (.hurl, .sonde), relative to the server root"`
	Env       string         `json:"env,omitempty" jsonschema:"a sonde.yaml environment (default: the server's --env, then SONDE_ENV, then the sonde.yaml defaults.env)"`
	Variables map[string]any `json:"variables,omitempty" jsonschema:"variables for this run: names to strings, numbers or booleans"`
}

func (s *server) run(ctx context.Context, _ *sdk.CallToolRequest, in runInput) (*sdk.CallToolResult, runOutput, error) {
	start := time.Now()
	fail := func(err error) (*sdk.CallToolResult, runOutput, error) {
		s.audit("sonde_run", start, "path=%q refused: %s", in.Path, s.redact(err.Error()))
		return nil, runOutput{}, err
	}
	vars, err := callVariables(in.Variables)
	if err != nil {
		return fail(err)
	}
	rf, err := s.readRequestFile(in.Path)
	if err != nil {
		return fail(err)
	}
	job, validator, envName, err := s.prepare(ctx, rf, in.Env)
	if err != nil {
		return fail(err)
	}
	for name := range vars {
		_, server := s.cfg.Secrets[name]
		_, project := job.Secrets[name]
		if server || project {
			return fail(fmt.Errorf("variables: %s is a secret", name))
		}
	}

	// One run at a time: a run holds connections and memory, and the
	// audit log stays readable.
	select {
	case s.runs <- struct{}{}:
		defer func() { <-s.runs }()
	case <-ctx.Done():
		return fail(ctx.Err())
	}
	// Both cases of the select may have been ready.
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	runCtx, cancel := context.WithTimeout(ctx, s.cfg.RunTimeout)
	defer cancel()

	variables := make(map[string]any, len(s.cfg.Variables)+len(vars))
	for name, v := range s.cfg.Variables {
		variables[name] = v
	}
	for name, v := range vars {
		variables[name] = v
	}
	runner := engine.NewRunner(engine.Options{
		Variables: variables,
		Secrets:   s.cfg.Secrets,
		FileRoot:  s.cfg.Root,
		HTTP:      engine.HTTPOptions{MaxFilesize: maxResponseBody},
		Validator: validator,
	})
	enginex.SetHosts(runner, s.cfg.Hosts)

	var res *engine.UnitResult
	var jobErr error
	runner.RunAll(runCtx, func(yield func(engine.Job) bool) { yield(job) }, engine.RunAllOptions{
		Finished: func(_ int, _ engine.Job, r *engine.UnitResult, err error) bool {
			res, jobErr = r, err
			return false
		},
	})
	if jobErr == nil && res == nil {
		// The run context ended before the job started.
		jobErr = fmt.Errorf("the run did not start: %w", runCtx.Err())
	}
	if jobErr != nil {
		err := errors.New(runner.Redact(s.relativeText(strings.ReplaceAll(jobErr.Error(), rf.abs, rf.rel))))
		s.audit("sonde_run", start, "path=%q env=%q error: %v", rf.rel, envName, err)
		return nil, runOutput{}, err
	}
	// A run that ended on its own right at the deadline did not time out.
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil && (res.Interrupted || !res.Success)
	out, err := buildRunOutput(res, rf, s.cfg.Root, envName, timedOut, s.cfg.RunTimeout)
	if err != nil {
		return nil, runOutput{}, err
	}
	outcome := fmt.Sprintf("success=%t", out.Success)
	switch {
	case timedOut:
		outcome = "timed out"
	case out.firstError != "":
		// Already redacted: the reason, such as a host not allowed.
		outcome += " failed at " + out.firstError
	}
	s.audit("sonde_run", start, "path=%q env=%q %s entries=%d requests=%d", rf.rel, envName, outcome, len(res.Entries), requests(res))
	return out.toolResult(), out, nil
}

// callVariables checks and converts the variables of a call. JSON
// numbers arrive as float64: an integral one becomes an integer, so that
// 42 renders as 42, not 42.0.
func callVariables(in map[string]any) (map[string]any, error) {
	if len(in) > maxVariables {
		return nil, fmt.Errorf("variables: at most %d", maxVariables)
	}
	out := make(map[string]any, len(in))
	for name, v := range in {
		if !variableName.MatchString(name) {
			return nil, fmt.Errorf("variables: invalid name %q", name)
		}
		switch x := v.(type) {
		case string:
			if len(x) > maxVariableBytes {
				return nil, fmt.Errorf("variables: %s is longer than %d bytes", name, maxVariableBytes)
			}
			out[name] = x
		case bool:
			out[name] = x
		case float64:
			if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
				out[name] = int64(x)
			} else {
				out[name] = x
			}
		default:
			return nil, fmt.Errorf("variables: %s must be a string, a number or a boolean", name)
		}
	}
	return out, nil
}

// prepare finds the sonde.yaml of a file (never one above the root),
// selects its environment and returns the job to run, with the file's
// contract when its sonde.yaml has one.
func (s *server) prepare(ctx context.Context, rf *requestFile, callEnv string) (engine.Job, engine.ResponseValidator, string, error) {
	job := engine.Job{Name: rf.abs, Source: rf.src}
	wantEnv := callEnv
	if wantEnv == "" {
		wantEnv = s.cfg.Env
	}
	cache := config.NewProjectCache()
	path, ok, err := cache.FindProject(filepath.Dir(rf.abs))
	for _, w := range cache.TakeWarnings() {
		_, _ = fmt.Fprintf(s.cfg.Log, "sonde mcp: warning: %s\n", w)
	}
	if err != nil {
		return job, nil, "", err
	}
	if !ok || !s.inRoot(path) {
		if wantEnv != "" {
			return job, nil, "", fmt.Errorf("%s: no sonde.yaml under the server root for environment %q", rf.rel, wantEnv)
		}
		return job, nil, "", nil
	}
	// The project file is read by path: it must not be a symbolic link,
	// which could lead out of the root (the files it names are read
	// through its own directory's sandbox).
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return job, nil, "", s.relative(fmt.Errorf("%s: not a regular file", path))
	}
	proj, err := config.LoadProject(path)
	if err != nil {
		return job, nil, "", s.relative(err)
	}
	envName := config.SelectEnv(wantEnv, s.cfg.EnvVar, proj.Defaults.Env)
	vars, secrets, err := proj.Resolve(envName)
	if err != nil {
		return job, nil, "", s.relative(err)
	}
	// The secrets the environment lists, set by none of its files (a
	// fresh clone) nor the server's --secret / SONDE_SECRET_*.
	if missing := proj.MissingSecrets(envName, secrets, func(n string) bool { _, ok := s.cfg.Secrets[n]; return ok }); len(missing) > 0 {
		return job, nil, "", s.relative(proj.MissingSecretsError(envName, missing))
	}
	job.Secrets = secrets
	if len(vars) > 0 {
		job.Variables = make(map[string]any, len(vars))
		for name, v := range vars {
			job.Variables[name] = v
		}
	}
	validator, err := s.contract(ctx, proj, rf.abs)
	if err != nil {
		return job, nil, "", err
	}
	return job, validator, envName, nil
}

// contract returns the validator of a file whose sonde.yaml has an
// "openapi:" block (nil: none).
func (s *server) contract(ctx context.Context, proj *config.Project, file string) (engine.ResponseValidator, error) {
	o := proj.OpenAPI
	if o == nil {
		return nil, nil
	}
	if o.ExcludesFile(proj.Dir, file) {
		return engine.NoContract(), nil
	}
	spec, err := openapi.Load(ctx, o.Spec, openapi.LoadOptions{})
	if err != nil {
		return nil, s.relative(fmt.Errorf("openapi.spec: %w", err))
	}
	return spec.Validator(openapi.Options{Server: o.Server, Strict: o.Strict, ExcludeOperations: o.ExcludeOperations}), nil
}

// relative rewrites the absolute paths under the root in an error as
// root-relative ones.
func (s *server) relative(err error) error {
	return errors.New(s.relativeText(err.Error()))
}

func (s *server) relativeText(text string) string {
	return relativeTo(s.cfg.Root, text)
}

func relativeTo(root, text string) string {
	return strings.ReplaceAll(text, root+string(filepath.Separator), "")
}

func requests(res *engine.UnitResult) int {
	n := 0
	for _, e := range res.Entries {
		n += len(e.Calls)
	}
	return n
}
