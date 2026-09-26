// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// OpenAPI is a sonde.yaml "openapi:" block: the contract every response
// of the project's files is validated against.
type OpenAPI struct {
	// Spec is the contract file, an absolute path inside the project
	// directory. A remote spec can only be given on the command line.
	Spec string
	// Server replaces the spec's servers for matching request URLs.
	Server string
	// Strict makes a request no operation matches a failure.
	Strict bool
	// ExcludeOperations are operations never validated, "METHOD /path"
	// with the path as written in the spec ("GET /health").
	ExcludeOperations []string
	// ExcludeFiles are globs of request files never validated, relative to
	// the project directory ("**" matches any number of directories).
	ExcludeFiles []string
}

type openAPIYAML struct {
	Spec              string   `yaml:"spec"`
	Server            string   `yaml:"server"`
	Strict            bool     `yaml:"strict"`
	ExcludeOperations []string `yaml:"exclude_operations"`
	ExcludeFiles      []string `yaml:"exclude_files"`
}

// operationRe matches an excluded operation: an HTTP method and a path.
var operationRe = regexp.MustCompile(`^[A-Z]+ /\S*$`)

// buildOpenAPI checks an "openapi:" block; spec must stay inside root.
func buildOpenAPI(root *projectSandbox, y *openAPIYAML) (*OpenAPI, error) {
	if y == nil {
		return nil, nil
	}
	if y.Spec == "" {
		return nil, fmt.Errorf("openapi.spec is required")
	}
	if strings.Contains(y.Spec, "://") {
		return nil, fmt.Errorf("openapi.spec: %q: a remote spec can only be given on the command line (--openapi with --openapi-allow-remote)", y.Spec)
	}
	if err := root.checkPath(y.Spec); err != nil {
		return nil, fmt.Errorf("openapi.spec: %w", err)
	}
	spec, err := root.root.Path(y.Spec)
	if err != nil {
		return nil, fmt.Errorf("openapi.spec: %w", err)
	}
	for _, op := range y.ExcludeOperations {
		if !operationRe.MatchString(op) {
			return nil, fmt.Errorf("openapi.exclude_operations: %q must be a method and a path, such as \"GET /health\"", op)
		}
	}
	for _, g := range y.ExcludeFiles {
		if g == "" || path.IsAbs(g) {
			return nil, fmt.Errorf("openapi.exclude_files: %q is not a relative glob", g)
		}
		for _, seg := range strings.Split(g, "/") {
			if _, err := path.Match(seg, ""); err != nil {
				return nil, fmt.Errorf("openapi.exclude_files: %q: %w", g, err)
			}
		}
	}
	return &OpenAPI{Spec: spec, Server: y.Server, Strict: y.Strict,
		ExcludeOperations: y.ExcludeOperations, ExcludeFiles: y.ExcludeFiles}, nil
}

// ExcludesFile reports whether file (a request file path) matches one of
// ExcludeFiles, taken relative to dir (the project directory).
func (o *OpenAPI) ExcludesFile(dir, file string) bool {
	abs, err := filepath.Abs(file)
	if err != nil {
		return false
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return false
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for _, g := range o.ExcludeFiles {
		if matchGlob(strings.Split(g, "/"), segs) {
			return true
		}
	}
	return false
}

// matchGlob matches path segments against glob segments, where "**"
// matches zero or more segments.
func matchGlob(glob, segs []string) bool {
	if len(glob) == 0 {
		return len(segs) == 0
	}
	if glob[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchGlob(glob[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	ok, err := path.Match(glob[0], segs[0])
	return err == nil && ok && matchGlob(glob[1:], segs[1:])
}
