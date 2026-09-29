// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"context"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/syntax"
)

// jobExtras holds one input file's sonde.yaml-resolved variables and
// secrets. They are set on that file's engine.Job below the runner's
// Options: an Options value of the same name wins (docs/sonde-yaml.md
// precedence), so this is a pure fallback layer.
type jobExtras struct {
	vars    map[string]any
	secrets map[string]string
	// project is the file's sonde.yaml.
	project *config.Project
	// validator checks the file's responses against its contract (nil:
	// the run's, from --openapi).
	validator engine.ResponseValidator
}

// Resolve finds each input file's sonde.yaml and resolves its environment
// and contract: the second half of building a run, once the input files
// are known. ctx bounds the loading of OpenAPI specs. Every error is a
// usage error.
func (p *Plan) Resolve(ctx context.Context, files []Input) error {
	p.files = files
	extras, defaultsJobs, warnings, err := resolveJobExtras(files, p.inv, p.env)
	if err != nil {
		return err
	}
	p.extras, p.Warnings = extras, warnings
	if p.Data != nil {
		for name, e := range extras {
			if err := p.Data.CheckSecrets(e.secrets); err != nil {
				return fmt.Errorf("%s: %w (sonde.yaml)", name, err)
			}
		}
	}
	if err := p.resolveContracts(ctx); err != nil {
		return err
	}
	if p.Workers > 1 && defaultsJobs > 0 && !p.inv.Changed("jobs") {
		if _, _, ok := p.env.Lookup("JOBS"); !ok {
			p.Workers = defaultsJobs
		}
	}
	return nil
}

// resolveContracts loads the OpenAPI specs of the run, once each: the
// run's (--openapi) and those of the files' sonde.yaml projects.
func (p *Plan) resolveContracts(ctx context.Context) error {
	c := newContracts(ctx, p.inv)
	v, err := c.forRun()
	if err != nil {
		return err
	}
	p.Options.Validator = v
	for _, f := range p.files {
		e, ok := p.extras[f.Name]
		if !ok || f.Stdin {
			continue
		}
		if e.validator, err = c.forFile(f.Name, e.project); err != nil {
			return err
		}
		p.extras[f.Name] = e
	}
	return nil
}

// resolveJobExtras discovers each real (non-stdin) file's sonde.yaml,
// selects its environment (--env, then SONDE_ENV, then that project's own
// defaults.env) and resolves its variables/secrets, returning them keyed
// by file name. defaultsJobs is the first project found's defaults.jobs
// (0 if none set anywhere), used as a --jobs fallback. warnings are every
// discovery warning the search collected (a candidate sonde.yaml skipped
// for failing the ownership/permission check — see
// config.ProjectCache.TakeWarnings); the caller is responsible for
// printing them, redacted like any other stderr text.
//
// inv.Config, when set, names the sonde.yaml used for every file,
// skipping discovery entirely. A file with no sonde.yaml above it (and no
// --config) is simply left out of the result: it runs with no sonde.yaml
// variables or secrets, not an error.
func resolveJobExtras(files []Input, inv *Invocation, env config.Env) (extras map[string]jobExtras, defaultsJobs int, warnings []string, err error) {
	extras = make(map[string]jobExtras)
	loaded := map[string]*config.Project{}
	var cache *config.ProjectCache
	if inv.Config == "" {
		cache = config.NewProjectCache()
	}
	haveDefaultsJobs := false

	for _, f := range files {
		if f.Stdin {
			continue
		}

		path := inv.Config
		if path == "" {
			found, ok, ferr := cache.FindProject(filepath.Dir(f.Name))
			if ferr != nil {
				return nil, 0, nil, ferr
			}
			if !ok {
				// No sonde.yaml applies to this file at all. An explicit
				// --env has nothing to select an environment from, which
				// is an error; SONDE_ENV alone (inv.Env stays "" here,
				// since SelectEnv's flag argument is exactly inv.Env) is
				// silently ignored instead, matching a plain run with no
				// sonde.yaml anywhere.
				if inv.Env != "" {
					return nil, 0, nil, fmt.Errorf("%s: no sonde.yaml found for environment %q", f.Name, inv.Env)
				}
				continue
			}
			path = found
		}

		proj, ok := loaded[path]
		if !ok {
			var lerr error
			proj, lerr = config.LoadProject(path)
			if lerr != nil {
				return nil, 0, nil, lerr
			}
			loaded[path] = proj
		}
		if !haveDefaultsJobs && proj.Defaults.Jobs > 0 {
			defaultsJobs, haveDefaultsJobs = proj.Defaults.Jobs, true
		}

		envName := config.SelectEnv(inv.Env, env["SONDE_ENV"], proj.Defaults.Env)
		vars, secrets, rerr := proj.Resolve(envName)
		if rerr != nil {
			return nil, 0, nil, rerr
		}
		e := jobExtras{secrets: secrets, project: proj}
		if len(vars) > 0 {
			e.vars = make(map[string]any, len(vars))
			for name, v := range vars {
				e.vars[name] = v
			}
		}
		extras[f.Name] = e
	}
	if cache != nil {
		warnings = cache.TakeWarnings()
	}
	return extras, defaultsJobs, warnings, nil
}

// Jobs returns the sequence of jobs of the run: the input files, repeated
// Repeat times (-1: forever). A real file's Source is left nil so RunAll
// reads it (lazily, once per attempt); stdin's Source is stdinSrc, the
// bytes read once, reused for every repeat. A file's sonde.yaml values
// and contract come from Resolve.
//
// With a data file, each file runs once per row; a data file that fails
// to read midway ends the sequence and sets *dataErr.
func (p *Plan) Jobs(stdinSrc []byte, dataErr *error) iter.Seq[engine.Job] {
	files, repeat, extras, data := p.files, p.Repeat, p.extras, p.Data
	return func(yield func(engine.Job) bool) {
		for pass := 0; repeat < 0 || pass < repeat; pass++ {
			yielded := false
			for _, f := range files {
				job := engine.Job{Name: f.Name}
				if f.Stdin {
					job = engine.Job{Name: "-", Source: stdinSrc}
				} else if e, ok := extras[f.Name]; ok {
					job.Variables = e.vars
					job.Secrets = e.secrets
					job.Validator = e.validator
				}
				if data == nil {
					if !yield(job) {
						return
					}
					yielded = true
					continue
				}
				if job.Source == nil {
					// Read once for all the rows; on failure the jobs
					// read it again and report the error.
					job.Source = readSourceOnce(f.Name)
				}
				stopped, err := data.Each(func(row *engine.Row) bool {
					job.Row = row
					yielded = true
					return yield(job)
				})
				if err != nil {
					*dataErr = err
					return
				}
				if stopped {
					return
				}
			}
			if !yielded {
				return // no rows: another pass would yield nothing either
			}
		}
	}
}

// readSourceOnce reads a request file shared by many jobs; nil when it
// cannot be read or is too large (each job then reports it).
func readSourceOnce(name string) []byte {
	f, err := os.Open(name) //nolint:gosec // G304: an input file named on the command line
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only
	src, err := ReadLimited(f)
	if err != nil {
		return nil
	}
	return src
}

// ReadLimited reads all of r but stops past syntax.MaxFileSize.
func ReadLimited(r io.Reader) ([]byte, error) {
	src, err := io.ReadAll(io.LimitReader(r, syntax.MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(src) > syntax.MaxFileSize {
		return nil, fmt.Errorf("file is larger than %d MiB", syntax.MaxFileSize>>20)
	}
	return src, nil
}
