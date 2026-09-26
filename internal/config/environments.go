// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/value"
)

// SelectEnv resolves the environment to run with, highest precedence
// first: an explicit --env flag, then the SONDE_ENV process environment
// variable, then sonde.yaml's defaults.env. An empty result means no
// environment was selected by any of the three (not an error by itself:
// Resolve("") returns no sonde.yaml variables or secrets).
func SelectEnv(flagEnv, sondeEnvVar, defaultEnv string) string {
	if flagEnv != "" {
		return flagEnv
	}
	if sondeEnvVar != "" {
		return sondeEnvVar
	}
	return defaultEnv
}

// walkEnvironment reads env's variable sources in Resolve's precedence
// order (variables, then variables_files in order, then secrets_files in
// order), calling onVariable for each inline "variables:" entry and
// onVariablesFile/onSecretsFile once per file with its raw bytes (nil,
// readErr when it could not be read; parsing is each callback's own job,
// since Resolve and VariableNames report a read failure and a parse
// failure with different wording).
//
// When stopOnError, a non-nil error from a callback ends the walk
// immediately and is returned, matching Resolve's abort-on-first-file-
// error contract. Otherwise every entry is visited regardless of earlier
// callback errors, letting a caller such as VariableNames collect a
// partial result across every file that did work, alongside whatever
// errors it wants to report.
//
// An empty env or an unknown one behaves exactly as Resolve documents: an
// empty env visits nothing and returns nil, an unknown one returns an
// error listing the environments that do exist before visiting anything.
func (p *Project) walkEnvironment(env string, stopOnError bool,
	onVariable func(name string, v value.Value),
	onVariablesFile func(rel string, data []byte, readErr error) error,
	onSecretsFile func(rel string, data []byte, readErr error) error,
) error {
	if env == "" {
		return nil
	}
	e, ok := p.Environments[env]
	if !ok {
		return fmt.Errorf("%s: unknown environment %q (available: %s)", p.Path, env, strings.Join(p.envNames(), ", "))
	}

	root, err := newProjectSandbox(p.Dir)
	if err != nil {
		return fmt.Errorf("%s: %w", p.Path, err)
	}
	defer root.Close()

	for name, v := range e.Variables {
		onVariable(name, v)
	}
	for _, rel := range e.VariablesFiles {
		data, rerr := root.ReadFile(rel)
		if cbErr := onVariablesFile(rel, data, rerr); cbErr != nil && stopOnError {
			return cbErr
		}
	}
	for _, rel := range e.SecretsFiles {
		data, rerr := root.ReadFile(rel)
		if cbErr := onSecretsFile(rel, data, rerr); cbErr != nil && stopOnError {
			return cbErr
		}
	}
	return nil
}

// Resolve returns the typed variables and secrets of env: its "variables"
// map, then its "variables_files" applied in order on top (a name in a
// later file wins), giving the sonde.yaml precedence tier described by
// docs/sonde-yaml.md. "secrets_files" is loaded the same way --secrets-file
// is: a name already defined by an earlier secrets source is an error.
//
// An empty env returns empty, non-nil maps and no error: a run with no
// environment selected has no sonde.yaml variables or secrets. A non-empty
// env not present in p.Environments is an error listing the environments
// that do exist.
func (p *Project) Resolve(env string) (variables map[string]value.Value, secrets map[string]string, err error) {
	variables = map[string]value.Value{}
	secrets = map[string]string{}

	werr := p.walkEnvironment(env, true,
		func(name string, v value.Value) { variables[name] = v },
		func(rel string, data []byte, readErr error) error {
			if readErr != nil {
				return fmt.Errorf("%s: variables_files: %w", p.Path, readErr)
			}
			assigns, perr := ParseProperties(data, Inferred)
			if perr != nil {
				return fmt.Errorf("%s: variables_files %s: %w", p.Path, rel, perr)
			}
			for _, a := range assigns {
				variables[a.Name] = a.Value
			}
			return nil
		},
		func(rel string, data []byte, readErr error) error {
			if readErr != nil {
				return fmt.Errorf("%s: secrets_files: %w", p.Path, readErr)
			}
			assigns, perr := ParseProperties(data, Forced)
			if perr != nil {
				return fmt.Errorf("%s: secrets_files %s: %w", p.Path, rel, perr)
			}
			for _, a := range assigns {
				if err := AddSecret(secrets, a.Name, a.Value); err != nil {
					return fmt.Errorf("%s: secrets_files %s: %w", p.Path, rel, err)
				}
			}
			return nil
		},
	)
	if werr != nil {
		return nil, nil, werr
	}
	return variables, secrets, nil
}

// envNames returns p.Environments' keys, sorted, for an "available: ..."
// error message.
func (p *Project) envNames() []string {
	names := make([]string, 0, len(p.Environments))
	for name := range p.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// projectSandbox confines variables_files/secrets_files reads to a
// sonde.yaml's own directory. Unlike a request file's paths (trusted
// enough that an absolute path landing inside the root is allowed, see
// internal/sandbox), sonde.yaml is untrusted input: an absolute path is
// always rejected outright, on top of the sandbox.Root checks for a ".."
// escape or a symbolic link leaving the directory.
type projectSandbox struct {
	root *sandbox.Root
}

func newProjectSandbox(dir string) (*projectSandbox, error) {
	root, err := sandbox.Open(dir)
	if err != nil {
		return nil, err
	}
	return &projectSandbox{root: root}, nil
}

func (s *projectSandbox) Close() error { return s.root.Close() }

// checkPath validates rel without requiring the file to exist: only an
// absolute path or an escape (via ".." or, for any existing ancestor
// component, a symbolic link) is rejected.
func (s *projectSandbox) checkPath(rel string) error {
	if filepath.IsAbs(rel) {
		return fmt.Errorf("%q must be a relative path", rel)
	}
	if _, err := s.root.Path(rel); err != nil {
		return err
	}
	return nil
}

// ReadFile reads rel, rejecting an absolute path outright and any other
// escape the same way checkPath does, plus two things a bare
// sandbox.Root.ReadFile does not check: rel must name a regular file
// (opening a FIFO with no writer would hang the run forever), and its
// content must not exceed maxProjectFileBytes.
func (s *projectSandbox) ReadFile(rel string) ([]byte, error) {
	if filepath.IsAbs(rel) {
		return nil, fmt.Errorf("%q must be a relative path", rel)
	}
	// s.root.Path already confines abs inside the sandbox, following the
	// same ".." and symbolic-link escape rules sandbox.Root.ReadFile
	// applies again below; this Lstat only decides whether reading it is
	// safe (regular file, within the size cap) before doing so.
	abs, err := s.root.Path(rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", rel)
	}
	if info.Size() > maxProjectFileBytes {
		return nil, fmt.Errorf("%q is larger than %d bytes", rel, maxProjectFileBytes)
	}
	data, err := s.root.ReadFile(rel)
	if err != nil {
		return nil, err
	}
	if len(data) > maxProjectFileBytes {
		return nil, fmt.Errorf("%q is larger than %d bytes", rel, maxProjectFileBytes)
	}
	return data, nil
}

// variableFromNode converts one "variables:" entry to a typed value.Value.
// Only a plain scalar is accepted: string, boolean, number or null: the
// same set a --variable or variables-file entry can hold
// (internal/config/value.go). A list or map value is rejected with its
// line, as is any other YAML type (dates, binary, ...).
func variableFromNode(node *yaml.Node) (value.Value, error) {
	if node.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("line %d: must be a string, boolean, number or null", node.Line)
	}
	switch node.ShortTag() {
	case "!!null":
		return value.Null{}, nil
	case "!!bool":
		var b bool
		if err := node.Decode(&b); err != nil {
			return nil, fmt.Errorf("line %d: %w", node.Line, err)
		}
		return value.Bool(b), nil
	case "!!int":
		if i, err := strconv.ParseInt(node.Value, 10, 64); err == nil {
			return value.Int(i), nil
		}
		if isAllDigits(strings.TrimPrefix(node.Value, "+")) {
			return value.BigInt(node.Value), nil
		}
		return nil, fmt.Errorf("line %d: invalid integer %q", node.Line, node.Value)
	case "!!float":
		var f float64
		if err := node.Decode(&f); err != nil {
			return nil, fmt.Errorf("line %d: %w", node.Line, err)
		}
		return value.Float(f), nil
	case "!!str":
		return value.String(node.Value), nil
	default:
		return nil, fmt.Errorf("line %d: unsupported value type %q", node.Line, node.ShortTag())
	}
}
