// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// EnvironmentSkeleton is one "environments:<name>:" entry for EmitProject.
type EnvironmentSkeleton struct {
	// Variables is the "variables:" map; every value is written as a YAML
	// string scalar (quoted where needed), matching this skeleton's plain
	// string values.
	Variables map[string]string
	// SecretsFiles is the "secrets_files:" list.
	SecretsFiles []string
}

// ProjectSkeleton is the data EmitProject renders to a sonde.yaml skeleton.
type ProjectSkeleton struct {
	// Environments is every "environments:" entry, by name.
	Environments map[string]EnvironmentSkeleton
	// DefaultEnv is "defaults.env"; "" omits the "defaults:" block.
	DefaultEnv string
	// OpenAPISpec is "openapi.spec"; "" omits the "openapi:" block.
	OpenAPISpec string
}

// skeletonYAML mirrors projectFileYAML's shape (schema in
// docs/sonde-yaml.md) but only the keys an importer skeleton ever sets;
// field order fixes the emitted top-level key order.
type skeletonYAML struct {
	Version      int                        `yaml:"version"`
	Environments map[string]envSkeletonYAML `yaml:"environments,omitempty"`
	Defaults     *defaultsSkeletonYAML      `yaml:"defaults,omitempty"`
	OpenAPI      *openAPISkeletonYAML       `yaml:"openapi,omitempty"`
}

type envSkeletonYAML struct {
	Variables    map[string]string `yaml:"variables,omitempty"`
	SecretsFiles []string          `yaml:"secrets_files,omitempty"`
}

type defaultsSkeletonYAML struct {
	Env string `yaml:"env"`
}

type openAPISkeletonYAML struct {
	Spec string `yaml:"spec"`
}

// EmitProject renders p as a "version: 1" sonde.yaml skeleton. Key order is
// deterministic (map keys are sorted by the YAML encoder) and the result is
// parseable by LoadProject.
func EmitProject(p ProjectSkeleton) ([]byte, error) {
	doc := skeletonYAML{Version: 1}
	if len(p.Environments) > 0 {
		doc.Environments = make(map[string]envSkeletonYAML, len(p.Environments))
		for name, env := range p.Environments {
			doc.Environments[name] = envSkeletonYAML(env)
		}
	}
	if p.DefaultEnv != "" {
		doc.Defaults = &defaultsSkeletonYAML{Env: p.DefaultEnv}
	}
	if p.OpenAPISpec != "" {
		doc.OpenAPI = &openAPISkeletonYAML{Spec: p.OpenAPISpec}
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("config: emit sonde.yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("config: emit sonde.yaml: %w", err)
	}
	return b.Bytes(), nil
}

// EmitVariables renders vars as a variables/secrets file: one "name=value"
// line per entry (ParseProperties's format, internal/config/properties.go),
// sorted by name for determinism. A value must not contain a newline; the
// format has no way to escape one.
func EmitVariables(vars map[string]string) []byte {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(vars[name])
		b.WriteByte('\n')
	}
	return []byte(b.String())
}
