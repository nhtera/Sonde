// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/nhtera/sonde/internal/value"
)

// VariableSource says where one of an environment's variables comes from.
type VariableSource struct {
	// File is the variables_files/secrets_files path (as written in
	// sonde.yaml, relative to the project), or "" for an inline
	// "variables:" entry.
	File string
	// Secret is set for a secrets_files entry: only its name is known,
	// its value is never returned or kept in the result. The value is
	// still decoded from the file (ParseProperties needs to, to find the
	// name and to detect a reassigned secret), but is discarded as soon as
	// that check is done and never appears in VariableNames' return.
	Secret bool
}

// VariableNames returns the names env defines and where each comes from,
// for callers (the LSP) that only need names, never values. Precedence
// matches Resolve: variables, then variables_files in order, then
// secrets_files in order, each later source's names winning on a clash,
// via the same walkEnvironment both share.
//
// Unlike Resolve, a file that can't be read or parsed does not discard
// everything collected so far: the walk continues through every remaining
// variables_files/secrets_files entry, and the first error encountered
// (if any) is returned alongside every name that could still be
// determined. A secrets_files name already used by an earlier secrets
// source is reported as its own error (AddSecret's message, the same one
// a run would fail with), and that one name is skipped.
//
// An empty env returns an empty, non-nil map and no error, like Resolve; a
// non-empty env absent from p.Environments is an error listing the
// environments that do exist (with no partial result, since no
// environment means nothing to collect from either).
func (p *Project) VariableNames(env string) (map[string]VariableSource, error) {
	names := map[string]VariableSource{}
	var firstErr error
	recordErr := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	secretsSeen := map[string]string{} // names only: values are stored as ""

	err := p.walkEnvironment(env, false,
		func(name string, _ value.Value) {
			names[name] = VariableSource{}
		},
		func(rel string, data []byte, readErr error) error {
			if readErr != nil {
				recordErr(fmt.Errorf("%s: variables_files: %w", p.Path, readErr))
				return nil
			}
			assigns, perr := ParseProperties(data, Inferred)
			if perr != nil {
				recordErr(fmt.Errorf("%s: variables_files %s: %w", p.Path, rel, perr))
				return nil
			}
			for _, a := range assigns {
				names[a.Name] = VariableSource{File: rel}
			}
			return nil
		},
		func(rel string, data []byte, readErr error) error {
			if p.optionalSecretsFile(env, readErr) {
				return nil
			}
			if readErr != nil {
				recordErr(fmt.Errorf("%s: secrets_files: %w", p.Path, readErr))
				return nil
			}
			assigns, perr := ParseProperties(data, Forced)
			if perr != nil {
				recordErr(fmt.Errorf("%s: secrets_files %s: %w", p.Path, rel, perr))
				return nil
			}
			for _, a := range assigns {
				// Forced parsing makes every value a string, so only the
				// reassignment check can fail; the value itself is dropped.
				if err := AddSecret(secretsSeen, a.Name, value.String("")); err != nil {
					recordErr(fmt.Errorf("%s: secrets_files %s: %w", p.Path, rel, err))
					continue
				}
				names[a.Name] = VariableSource{File: rel, Secret: true}
			}
			return nil
		},
	)
	if err != nil {
		// env is unknown, or the project's sandbox couldn't be opened:
		// nothing was visited, so there is no partial result to add to
		// names beyond what the caller already has (empty).
		return names, err
	}
	return names, firstErr
}
