// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/openapi"
)

// openAPIOptions are the --openapi* flags.
type openAPIOptions struct {
	spec        string
	server      string
	strict      bool
	allowRemote bool
}

func addOpenAPIFlags(f *pflag.FlagSet, o *openAPIOptions) {
	f.StringVar(&o.spec, "openapi", "", "validates every response against an OpenAPI spec (file, or URL with --openapi-allow-remote)")
	f.StringVar(&o.server, "openapi-server", "", "base URL replacing the spec's servers when matching requests to operations")
	f.BoolVar(&o.strict, "openapi-strict", false, "fails a request that no operation of the spec matches")
	f.BoolVar(&o.allowRemote, "openapi-allow-remote", false, "allows a remote spec and remote $ref targets")
}

// contracts builds the validators of a run: one per distinct spec and
// settings, every spec loaded once.
type contracts struct {
	ctx         context.Context
	flags       openAPIOptions
	strictSet   bool
	specs       map[string]*openapi.Spec
	validators  map[string]engine.ResponseValidator
	loadOptions openapi.LoadOptions
}

func newContracts(cmd *cobra.Command, o openAPIOptions) *contracts {
	return &contracts{
		ctx:         cmd.Context(),
		flags:       o,
		strictSet:   changed(cmd, "openapi-strict"),
		specs:       map[string]*openapi.Spec{},
		validators:  map[string]engine.ResponseValidator{},
		loadOptions: openapi.LoadOptions{AllowRemote: o.allowRemote},
	}
}

// forRun returns the validator of --openapi for files without a
// sonde.yaml contract (nil without --openapi).
func (c *contracts) forRun() (engine.ResponseValidator, error) {
	if c.flags.spec == "" {
		return nil, nil
	}
	return c.validator(c.flags.spec, "--openapi", openapi.Options{Server: c.flags.server, Strict: c.flags.strict})
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
		return engine.NoContract, nil
	}
	spec, origin := o.Spec, proj.Path+": openapi.spec"
	if c.flags.spec != "" {
		spec, origin = c.flags.spec, "--openapi"
	}
	opt := openapi.Options{Server: o.Server, Strict: o.Strict, ExcludeOperations: o.ExcludeOperations}
	if c.flags.server != "" {
		opt.Server = c.flags.server
	}
	if c.strictSet {
		opt.Strict = c.flags.strict
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
			return nil, NewExitError(ExitUsage, fmt.Errorf("%s: %w", origin, err))
		}
		c.specs[spec] = s
	}
	v := s.Validator(opt)
	c.validators[key] = v
	return v, nil
}
