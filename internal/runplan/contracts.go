// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"context"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/openapi"
)

// contracts builds the validators of a run: one per distinct spec and
// settings, every spec loaded once.
type contracts struct {
	ctx         context.Context
	flags       OpenAPI
	strictSet   bool
	specs       map[string]*openapi.Spec
	validators  map[string]engine.ResponseValidator
	loadOptions openapi.LoadOptions
}

func newContracts(ctx context.Context, inv *Invocation) *contracts {
	return &contracts{
		ctx:         ctx,
		flags:       inv.OpenAPI,
		strictSet:   inv.Changed("openapi-strict"),
		specs:       map[string]*openapi.Spec{},
		validators:  map[string]engine.ResponseValidator{},
		loadOptions: openapi.LoadOptions{AllowRemote: inv.OpenAPI.AllowRemote},
	}
}

// forRun returns the validator of --openapi for files without a
// sonde.yaml contract (nil without --openapi).
func (c *contracts) forRun() (engine.ResponseValidator, error) {
	if c.flags.Spec == "" {
		return nil, nil
	}
	return c.validator(c.flags.Spec, "--openapi", openapi.Options{Server: c.flags.Server, Strict: c.flags.Strict})
}

// forFile returns the validator of a file whose sonde.yaml has an
// "openapi:" block: command line flags win key by key, the exclusions
// always apply. nil: the run's validator applies.
func (c *contracts) forFile(file string, proj *config.Project) (engine.ResponseValidator, error) {
	if proj == nil || proj.OpenAPI == nil {
		return nil, nil
	}
	o := proj.OpenAPI
	if o.ExcludesFile(proj.Dir, file) {
		return engine.NoContract(), nil
	}
	spec, origin := o.Spec, proj.Path+": openapi.spec"
	if c.flags.Spec != "" {
		spec, origin = c.flags.Spec, "--openapi"
	}
	opt := openapi.Options{Server: o.Server, Strict: o.Strict, ExcludeOperations: o.ExcludeOperations}
	if c.flags.Server != "" {
		opt.Server = c.flags.Server
	}
	if c.strictSet {
		opt.Strict = c.flags.Strict
	}
	return c.validator(spec, origin, opt)
}

func (c *contracts) validator(spec, origin string, opt openapi.Options) (engine.ResponseValidator, error) {
	key := strings.Join(append([]string{spec, opt.Server, fmt.Sprint(opt.Strict)}, opt.ExcludeOperations...), "\x00")
	if v, ok := c.validators[key]; ok {
		return v, nil
	}
	s, ok := c.specs[spec]
	if !ok {
		var err error
		if s, err = openapi.Load(c.ctx, spec, c.loadOptions); err != nil {
			return nil, fmt.Errorf("%s: %w", origin, err)
		}
		c.specs[spec] = s
	}
	v := s.Validator(opt)
	c.validators[key] = v
	return v, nil
}
