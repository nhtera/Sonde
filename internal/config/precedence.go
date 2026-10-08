// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"maps"
	"os"

	"github.com/nhtera/sonde/internal/value"
)

// BuildVariables merges variable sources in the upstream precedence order
// (lowest to highest): the config file's --variable lines, then
// HURL_VARIABLE_*/SONDE_VARIABLE_* env vars, then --variables-file files
// (in the order given, each read in full), then --variable "name=value"
// assignments (in the order given). A name defined by more than one source
// keeps the value of the last one.
func BuildVariables(file FileOptions, env Env, variablesFiles, variables []string) (map[string]value.Value, error) {
	vars := map[string]value.Value{}
	for _, a := range file.Variables {
		vars[a.Name] = a.Value
	}
	if err := env.ApplyVariableEnvVars(vars); err != nil {
		return nil, err
	}
	for _, path := range variablesFiles {
		data, err := readPropertiesFile(path)
		if err != nil {
			return nil, err
		}
		assigns, err := ParseProperties(data, Inferred)
		if err != nil {
			return nil, err
		}
		for _, a := range assigns {
			vars[a.Name] = a.Value
		}
	}
	for _, s := range variables {
		a, err := ParseAssignment(s, Inferred)
		if err != nil {
			return nil, err
		}
		vars[a.Name] = a.Value
	}
	return vars, nil
}

// BuildSecrets merges secret sources in the same order as BuildVariables:
// the config file's --secret lines, then HURL_SECRET_*/SONDE_SECRET_* env
// vars, then --secrets-file files, then --secret assignments. Unlike a
// variable, a name defined by more than one source is an error: a secret
// can never be reassigned, not even by the command line.
func BuildSecrets(file FileOptions, env Env, secretsFiles, secrets []string) (map[string]string, error) {
	out := maps.Clone(file.Secrets)
	if out == nil {
		out = map[string]string{}
	}
	if err := env.ApplySecretEnvVars(out); err != nil {
		return nil, err
	}
	for _, path := range secretsFiles {
		data, err := readPropertiesFile(path)
		if err != nil {
			return nil, err
		}
		assigns, err := ParseProperties(data, Forced)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, a := range assigns {
			if err := AddSecret(out, a.Name, a.Value); err != nil {
				return nil, err
			}
		}
	}
	for _, s := range secrets {
		a, err := ParseAssignment(s, Forced)
		if err != nil {
			return nil, err
		}
		if err := AddSecret(out, a.Name, a.Value); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// readPropertiesFile reads a variables/secrets file, reporting the
// upstream tool's own messages for a missing or unreadable file.
func readPropertiesFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the command line names the file
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("Variables file %s does not exist", path)
	}
	if err != nil {
		return nil, fmt.Errorf("Error opening %s", path)
	}
	return data, nil
}
